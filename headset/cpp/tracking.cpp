// PICO Trackers (шлем): читает скелет тела XR_BD_body_tracking и шлёт его на ПК по UDP.
//
// Особенности рантайма PICO 4 Pro (Pico(XRT), форк Monado) — подробнее в CLAUDE.md:
//  * сессия headless (XR_MND_headless): без графики, экран остаётся у Steam Link;
//  * из фонового приложения сессия навсегда остаётся в IDLE, но xrLocateBodyJointsBD в IDLE работает;
//  * xrLocateSpace в IDLE роняет рантайм — не вызываем, поза головы берётся из сустава head.

#include <jni.h>
#include <android/log.h>
#include <netdb.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

#include <algorithm>
#include <atomic>
#include <cerrno>
#include <chrono>
#include <cstdarg>
#include <cstdio>
#include <cstring>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

#define XR_USE_PLATFORM_ANDROID
#define XR_USE_TIMESPEC
#include <openxr/openxr.h>
#include <openxr/openxr_platform.h>

#include "protocol.h"

#define TAG "PicoTrackers"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, TAG, __VA_ARGS__)
#define LOGW(...) __android_log_print(ANDROID_LOG_WARN, TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, TAG, __VA_ARGS__)

namespace {

constexpr int kDefaultRate = 100, kMinRate = 10, kMaxRate = 250;  // снимков/с; задаёт драйвер (PTC1 rate_hz)
constexpr auto kRetryDelay = std::chrono::seconds(2);
constexpr int kThreadNice = -10;

// Вариант PICO (так вызывает ALVR): (instance, status, error). У Khronos сигнатура другая,
// поэтому даём с запасом большие буферы, чтобы не испортить память при несовпадении.
typedef XrResult(XRAPI_PTR *PFN_xrGetBodyTrackingStatePico)(XrInstance instance, int32_t *status, int32_t *error);

JavaVM *g_vm = nullptr;
jobject g_context = nullptr;
std::thread g_thread;
std::atomic<bool> g_running{false};
std::atomic<int> g_rate{kDefaultRate};

std::mutex g_status_mutex;
std::string g_status = "остановлено";

void set_status(const std::string &s) {
    std::lock_guard<std::mutex> lock(g_status_mutex);
    g_status = s;
}

std::string format(const char *fmt, ...) __attribute__((format(printf, 1, 2)));
std::string format(const char *fmt, ...) {
    char buf[512];
    va_list args;
    va_start(args, fmt);
    vsnprintf(buf, sizeof(buf), fmt, args);
    va_end(args);
    return buf;
}

std::string result_str(XrInstance instance, XrResult r) {
    char buf[XR_MAX_RESULT_STRING_SIZE];
    if (instance != XR_NULL_HANDLE && xrResultToString(instance, r, buf) == XR_SUCCESS) return buf;
    return std::to_string(r);
}

const char *state_name(XrSessionState s) {
    switch (s) {
    case XR_SESSION_STATE_IDLE: return "IDLE";
    case XR_SESSION_STATE_READY: return "READY";
    case XR_SESSION_STATE_SYNCHRONIZED: return "SYNCHRONIZED";
    case XR_SESSION_STATE_VISIBLE: return "VISIBLE";
    case XR_SESSION_STATE_FOCUSED: return "FOCUSED";
    case XR_SESSION_STATE_STOPPING: return "STOPPING";
    case XR_SESSION_STATE_LOSS_PENDING: return "LOSS_PENDING";
    case XR_SESSION_STATE_EXITING: return "EXITING";
    default: return "NONE";
    }
}

template <typename T>
T get_proc(XrInstance instance, const char *name) {
    PFN_xrVoidFunction fn = nullptr;
    xrGetInstanceProcAddr(instance, name, &fn);
    return reinterpret_cast<T>(fn);
}

// Ждёт, пока не истечёт время или не попросят остановиться.
void sleep_while_running(std::chrono::milliseconds duration) {
    auto until = std::chrono::steady_clock::now() + duration;
    while (g_running.load() && std::chrono::steady_clock::now() < until)
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
}

class UdpSender {
public:
    bool open(const std::string &host, int port) {
        addrinfo hints{};
        hints.ai_family = AF_INET;
        hints.ai_socktype = SOCK_DGRAM;
        addrinfo *res = nullptr;
        if (getaddrinfo(host.c_str(), std::to_string(port).c_str(), &hints, &res) != 0 || !res) return false;
        memcpy(&addr_, res->ai_addr, res->ai_addrlen);
        addr_len_ = res->ai_addrlen;
        freeaddrinfo(res);

        fd_ = socket(AF_INET, SOCK_DGRAM, 0);
        if (fd_ < 0) return false;
        int on = 1;
        setsockopt(fd_, SOL_SOCKET, SO_BROADCAST, &on, sizeof(on));
        return true;
    }

    bool send(const void *data, size_t size) {
        return sendto(fd_, data, size, 0, reinterpret_cast<sockaddr *>(&addr_), addr_len_) >= 0;
    }

    ~UdpSender() {
        if (fd_ >= 0) close(fd_);
    }

private:
    int fd_ = -1;
    sockaddr_storage addr_{};
    socklen_t addr_len_ = 0;
};

// Одна «жизнь» трекинга: instance, headless-сессия, трекер тела.
// При потере сессии объект пересоздаётся целиком.
class BodyTracking {
public:
    ~BodyTracking() {
        if (tracker_) destroy_tracker_(tracker_);
        if (session_) xrDestroySession(session_);
        if (instance_) xrDestroyInstance(instance_);
    }

    // Пустая строка — успех, иначе описание ошибки.
    std::string init() {
        static bool loader_ready = false;
        if (!loader_ready) {
            auto initialize_loader = get_proc<PFN_xrInitializeLoaderKHR>(XR_NULL_HANDLE, "xrInitializeLoaderKHR");
            if (initialize_loader) {
                XrLoaderInitInfoAndroidKHR loader_info{XR_TYPE_LOADER_INIT_INFO_ANDROID_KHR};
                loader_info.applicationVM = g_vm;
                loader_info.applicationContext = g_context;
                initialize_loader(reinterpret_cast<const XrLoaderInitInfoBaseHeaderKHR *>(&loader_info));
            }
            loader_ready = true;
        }

        uint32_t count = 0;
        xrEnumerateInstanceExtensionProperties(nullptr, 0, &count, nullptr);
        std::vector<XrExtensionProperties> exts(count, {XR_TYPE_EXTENSION_PROPERTIES});
        xrEnumerateInstanceExtensionProperties(nullptr, count, &count, exts.data());
        auto has = [&](const char *name) {
            return std::any_of(exts.begin(), exts.end(), [&](const auto &e) { return strcmp(e.extensionName, name) == 0; });
        };
        for (const char *required : {XR_BD_BODY_TRACKING_EXTENSION_NAME, XR_MND_HEADLESS_EXTENSION_NAME,
                                     XR_KHR_CONVERT_TIMESPEC_TIME_EXTENSION_NAME})
            if (!has(required)) return format("рантайм не поддерживает %s", required);

        std::vector<const char *> enabled = {XR_BD_BODY_TRACKING_EXTENSION_NAME, XR_MND_HEADLESS_EXTENSION_NAME,
                                             XR_KHR_CONVERT_TIMESPEC_TIME_EXTENSION_NAME};
        XrInstanceCreateInfoAndroidKHR android_info{XR_TYPE_INSTANCE_CREATE_INFO_ANDROID_KHR};
        android_info.applicationVM = g_vm;
        android_info.applicationActivity = g_context;
        XrInstanceCreateInfo create_info{XR_TYPE_INSTANCE_CREATE_INFO};
        if (has(XR_KHR_ANDROID_CREATE_INSTANCE_EXTENSION_NAME)) {
            enabled.push_back(XR_KHR_ANDROID_CREATE_INSTANCE_EXTENSION_NAME);
            create_info.next = &android_info;
        }
        create_info.enabledExtensionCount = enabled.size();
        create_info.enabledExtensionNames = enabled.data();
        strncpy(create_info.applicationInfo.applicationName, "PICO Trackers", XR_MAX_APPLICATION_NAME_SIZE - 1);
        strncpy(create_info.applicationInfo.engineName, "none", XR_MAX_ENGINE_NAME_SIZE - 1);
        create_info.applicationInfo.apiVersion = XR_API_VERSION_1_0;
        XrResult r = xrCreateInstance(&create_info, &instance_);
        if (XR_FAILED(r)) return format("xrCreateInstance: %d", r);

        XrSystemGetInfo system_info{XR_TYPE_SYSTEM_GET_INFO};
        system_info.formFactor = XR_FORM_FACTOR_HEAD_MOUNTED_DISPLAY;
        XrSystemId system = XR_NULL_SYSTEM_ID;
        r = xrGetSystem(instance_, &system_info, &system);
        if (XR_FAILED(r)) return "xrGetSystem: " + result_str(instance_, r);

        XrSystemBodyTrackingPropertiesBD body_props{XR_TYPE_SYSTEM_BODY_TRACKING_PROPERTIES_BD};
        XrSystemProperties props{XR_TYPE_SYSTEM_PROPERTIES, &body_props};
        xrGetSystemProperties(instance_, system, &props);
        if (!body_props.supportsBodyTracking) return "шлем не поддерживает body tracking";

        // Без graphics binding и с XR_MND_headless сессия создаётся без графики.
        XrSessionCreateInfo session_info{XR_TYPE_SESSION_CREATE_INFO};
        session_info.systemId = system;
        r = xrCreateSession(instance_, &session_info, &session_);
        if (XR_FAILED(r)) return "xrCreateSession: " + result_str(instance_, r);

        XrReferenceSpaceCreateInfo space_info{XR_TYPE_REFERENCE_SPACE_CREATE_INFO};
        space_info.referenceSpaceType = XR_REFERENCE_SPACE_TYPE_STAGE;
        space_info.poseInReferenceSpace.orientation.w = 1;
        r = xrCreateReferenceSpace(session_, &space_info, &stage_);
        if (XR_FAILED(r)) return "STAGE space: " + result_str(instance_, r);

        auto create_tracker = get_proc<PFN_xrCreateBodyTrackerBD>(instance_, "xrCreateBodyTrackerBD");
        destroy_tracker_ = get_proc<PFN_xrDestroyBodyTrackerBD>(instance_, "xrDestroyBodyTrackerBD");
        locate_joints_ = get_proc<PFN_xrLocateBodyJointsBD>(instance_, "xrLocateBodyJointsBD");
        get_state_ = get_proc<PFN_xrGetBodyTrackingStatePico>(instance_, "xrGetBodyTrackingStateBD");
        timespec_to_xr_ = get_proc<PFN_xrConvertTimespecTimeToTimeKHR>(instance_, "xrConvertTimespecTimeToTimeKHR");
        if (!create_tracker || !destroy_tracker_ || !locate_joints_ || !timespec_to_xr_)
            return "рантайм не отдал функции body tracking";

        for (auto [set, joints] : {std::pair{XR_BODY_JOINT_SET_FULL_BODY_JOINTS_BD, XR_BODY_JOINT_COUNT_BD},
                                   std::pair{XR_BODY_JOINT_SET_BODY_WITHOUT_ARM_BD, XR_BODY_JOINT_WITHOUT_ARM_COUNT_BD}}) {
            XrBodyTrackerCreateInfoBD tracker_info{XR_TYPE_BODY_TRACKER_CREATE_INFO_BD};
            tracker_info.jointSet = set;
            r = create_tracker(session_, &tracker_info, &tracker_);
            if (XR_SUCCEEDED(r)) {
                joint_count_ = joints;
                break;
            }
        }
        if (XR_FAILED(r)) return "xrCreateBodyTrackerBD: " + result_str(instance_, r);

        LOGI("body tracking ready, %u joints", joint_count_);
        return {};
    }

    // Заполняет пакет свежим скелетом. false — сессию потеряли, нужен перезапуск.
    bool poll(picotrackers::Packet &packet, bool &located) {
        located = false;
        if (!handle_events()) return false;

        timespec ts{};
        clock_gettime(CLOCK_MONOTONIC, &ts);
        XrTime now = 0;
        timespec_to_xr_(instance_, &ts, &now);

        XrBodyJointLocationBD locations[XR_BODY_JOINT_COUNT_BD]{};
        XrBodyJointsLocateInfoBD info{XR_TYPE_BODY_JOINTS_LOCATE_INFO_BD};
        info.baseSpace = stage_;
        info.time = now;
        XrBodyJointLocationsBD result{XR_TYPE_BODY_JOINT_LOCATIONS_BD};
        result.jointLocationCount = joint_count_;
        result.jointLocations = locations;
        XrResult r = locate_joints_(tracker_, &info, &result);
        if (r == XR_ERROR_SESSION_LOST || r == XR_ERROR_INSTANCE_LOST) return false;
        if (XR_FAILED(r)) {
            last_error_ = r;
            return true;
        }

        packet = {};
        memcpy(packet.magic, picotrackers::kMagic, sizeof(packet.magic));
        packet.xr_time_ns = now;
        packet.joint_count = joint_count_;
        if (result.allJointPosesTracked) packet.flags |= picotrackers::kFlagAllJointsTracked;
        if (trackers_ready_) packet.flags |= picotrackers::kFlagTrackersReady;
        for (uint32_t i = 0; i < joint_count_; i++) {
            const XrPosef &p = locations[i].pose;
            auto &joint = packet.joints[i];
            joint.position[0] = p.position.x;
            joint.position[1] = p.position.y;
            joint.position[2] = p.position.z;
            joint.orientation[0] = p.orientation.x;
            joint.orientation[1] = p.orientation.y;
            joint.orientation[2] = p.orientation.z;
            joint.orientation[3] = p.orientation.w;
            joint.flags = static_cast<uint32_t>(locations[i].locationFlags);
        }
        located = true;
        return true;
    }

    // Раз в секунду: подключены ли трекеры (status != 0 — подключены и откалиброваны).
    void refresh_tracker_state() {
        if (!get_state_) return;
        int32_t status[16]{}, error[16]{};
        if (XR_SUCCEEDED(get_state_(instance_, status, error))) trackers_ready_ = status[0] != 0;
    }

    bool trackers_ready() const { return trackers_ready_; }
    XrSessionState state() const { return state_; }

    XrResult take_last_error() {
        XrResult r = last_error_;
        last_error_ = XR_SUCCESS;
        return r;
    }

    std::string result_string(XrResult r) const { return result_str(instance_, r); }

private:
    bool handle_events() {
        XrEventDataBuffer event{XR_TYPE_EVENT_DATA_BUFFER};
        while (xrPollEvent(instance_, &event) == XR_SUCCESS) {
            if (event.type == XR_TYPE_EVENT_DATA_INSTANCE_LOSS_PENDING) return false;
            if (event.type == XR_TYPE_EVENT_DATA_SESSION_STATE_CHANGED) {
                state_ = reinterpret_cast<XrEventDataSessionStateChanged &>(event).state;
                LOGI("session state -> %s", state_name(state_));
                // Фоновое приложение READY не получает; ветки на случай, если PICO это изменит.
                if (state_ == XR_SESSION_STATE_READY) {
                    XrSessionBeginInfo begin{XR_TYPE_SESSION_BEGIN_INFO};
                    begin.primaryViewConfigurationType = XR_VIEW_CONFIGURATION_TYPE_PRIMARY_STEREO;
                    xrBeginSession(session_, &begin);
                } else if (state_ == XR_SESSION_STATE_STOPPING) {
                    xrEndSession(session_);
                } else if (state_ == XR_SESSION_STATE_EXITING || state_ == XR_SESSION_STATE_LOSS_PENDING) {
                    return false;
                }
            }
            event = {XR_TYPE_EVENT_DATA_BUFFER};
        }
        return true;
    }

    XrInstance instance_ = XR_NULL_HANDLE;
    XrSession session_ = XR_NULL_HANDLE;
    XrSpace stage_ = XR_NULL_HANDLE;
    XrBodyTrackerBD tracker_ = XR_NULL_HANDLE;
    uint32_t joint_count_ = 0;
    XrSessionState state_ = XR_SESSION_STATE_UNKNOWN;
    bool trackers_ready_ = false;
    XrResult last_error_ = XR_SUCCESS;

    PFN_xrDestroyBodyTrackerBD destroy_tracker_ = nullptr;
    PFN_xrLocateBodyJointsBD locate_joints_ = nullptr;
    PFN_xrGetBodyTrackingStatePico get_state_ = nullptr;
    PFN_xrConvertTimespecTimeToTimeKHR timespec_to_xr_ = nullptr;
};

void run(const std::string &host, int port) {
    if (setpriority(PRIO_PROCESS, gettid(), kThreadNice) != 0) LOGW("setpriority failed: %s", strerror(errno));

    UdpSender udp;
    if (!udp.open(host, port)) {
        set_status(format("ОШИБКА: не удалось открыть UDP на %s:%d", host.c_str(), port));
        LOGE("udp open failed: %s:%d", host.c_str(), port);
        return;
    }
    std::string target = format("%s:%d", host.c_str(), port);

    while (g_running.load()) {
        BodyTracking body;
        std::string error = body.init();
        if (!error.empty()) {
            set_status("Не удалось запустить трекинг: " + error + "\nПовтор через 2 с…");
            LOGW("init failed: %s", error.c_str());
            sleep_while_running(kRetryDelay);
            continue;
        }

        uint32_t seq = 0;
        int sent = 0, tracked = 0;
        auto next_tick = std::chrono::steady_clock::now();
        auto next_report = next_tick;
        picotrackers::Packet packet{};
        bool lost = false;

        while (g_running.load()) {
            auto t = std::chrono::steady_clock::now();
            if (t >= next_report) {
                next_report = t + std::chrono::seconds(1);
                body.refresh_tracker_state();
                XrResult err = body.take_last_error();
                std::string err_text = err == XR_SUCCESS ? "" : "\nОшибка опроса: " + body.result_string(err);
                set_status(format("Отправка на %s: %d пакетов/с\nТрекеры: %s\nСкелет отслеживается: %d/%d\nСессия: %s",
                                  target.c_str(), sent,
                                  body.trackers_ready() ? "подключены" : "НЕ подключены — включите трекеры",
                                  tracked, sent, state_name(body.state())) +
                           err_text);
                // Строку «stats» разбирает pc/picotrackers.py.
                LOGI("stats sent=%d tracked=%d trackers=%d session=%s err=%d rate=%d target=%s", sent, tracked,
                     body.trackers_ready(), state_name(body.state()), err, g_rate.load(), target.c_str());
                sent = tracked = 0;
            }

            bool located = false;
            if (!body.poll(packet, located)) {
                lost = true;
                break;
            }
            if (located) {
                packet.seq = seq++;
                if (udp.send(&packet, sizeof(packet))) sent++;
                if (packet.flags & picotrackers::kFlagAllJointsTracked) tracked++;
            }

            next_tick += std::chrono::microseconds(1000000 / g_rate.load());
            if (next_tick < t) next_tick = t;  // не догоняем пропущенные тики
            std::this_thread::sleep_until(next_tick);
        }
        if (lost) {
            LOGW("session lost, restarting");
            set_status("Сессия OpenXR потеряна, перезапуск…");
            sleep_while_running(kRetryDelay);
        }
    }
    LOGI("tracking stopped");
}

void tracking_thread(std::string host, int port) {
    JNIEnv *env = nullptr;
    g_vm->AttachCurrentThread(&env, nullptr);
    set_status("запуск…");
    run(host, port);
    g_running = false;  // после ошибки можно нажать «Старт» ещё раз
    g_vm->DetachCurrentThread();
}

}  // namespace

extern "C" JNIEXPORT jint JNI_OnLoad(JavaVM *vm, void *) {
    g_vm = vm;
    return JNI_VERSION_1_6;
}

extern "C" JNIEXPORT void JNICALL Java_dev_picotrackers_headset_Native_start(JNIEnv *env, jclass, jobject context,
                                                                           jstring host, jint port) {
    if (g_running.exchange(true)) return;
    if (g_thread.joinable()) g_thread.join();
    // Ссылку не пересоздаём: loader запоминает контекст при первом xrInitializeLoaderKHR
    // и обращается к нему при каждом xrCreateInstance (иначе — «accessed deleted Global»).
    if (!g_context) g_context = env->NewGlobalRef(context);

    const char *chars = env->GetStringUTFChars(host, nullptr);
    std::string host_str(chars);
    env->ReleaseStringUTFChars(host, chars);

    LOGI("start, target %s:%d", host_str.c_str(), port);
    g_thread = std::thread(tracking_thread, host_str, port);
}

extern "C" JNIEXPORT void JNICALL Java_dev_picotrackers_headset_Native_stop(JNIEnv *, jclass) {
    g_running = false;
    if (g_thread.joinable()) g_thread.join();
    set_status("трекинг выключен");
}

extern "C" JNIEXPORT void JNICALL Java_dev_picotrackers_headset_Native_setRate(JNIEnv *, jclass, jint rate) {
    int clamped = std::clamp(static_cast<int>(rate), kMinRate, kMaxRate);
    if (g_rate.exchange(clamped) != clamped) LOGI("rate -> %d/s", clamped);
}

extern "C" JNIEXPORT jboolean JNICALL Java_dev_picotrackers_headset_Native_running(JNIEnv *, jclass) {
    return g_running.load();
}

extern "C" JNIEXPORT jstring JNICALL Java_dev_picotrackers_headset_Native_status(JNIEnv *env, jclass) {
    std::lock_guard<std::mutex> lock(g_status_mutex);
    return env->NewStringUTF(g_status.c_str());
}
