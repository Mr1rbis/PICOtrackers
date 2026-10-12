// Трекеры PICO (Motion Tracker).
//
// Подключение, калибровка и заряд — из системной libtrackingclient.pxr.so (она в public.libraries-pxr.txt, приложениям
// можно): C-обёртки над клиентом сервиса трекинга PICO — CreateClient() (синглтон), GetSwiftConnectState (Swift —
// внутреннее имя Motion Tracker; {u8 количество, u8 номера[12]}), GetSwiftCalibState, GetSwiftTrackerBattery(номер),
// getSwiftTrackerMaxBattery. Проверено на PICO 4 Pro (PICO OS 5.13) из adb shell: 2 трекера с номерами 1 и 2, заряд 8
// из 10, calibrated 0 у неоткалиброванных. (libpxr_api.so из PICO SDK 2.x — обёртка над тем же, но на 5.13 её нет.)
// В OpenXR-рантайме ничего такого нет, а xrGetBodyTrackingStateBD у неоткалиброванных трекеров отвечает
// status 0 error 0 — как будто трекеров нет. TrackingService.java страхует первый опрос: если процесс на нём упал,
// при следующем запуске опрос отключается.
//
// Калибровку открывает рантайм: xrStartBodyTrackingCalibAppBD (так делает ALVR) или её вариант из PICO SDK 3
// xrStartBodyTrackingCalibrationAppPICO(instance). Если не вышло — TrackingService запускает приложение
// калибровки (com.pvr.swift) сам.

#include "pico_trackers.h"

#include <android/log.h>
#include <dlfcn.h>

#include <algorithm>
#include <cstring>
#include <mutex>

#define TAG "PicoTrackers"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, TAG, __VA_ARGS__)
#define LOGW(...) __android_log_print(ANDROID_LOG_WARN, TAG, __VA_ARGS__)

namespace pico {
namespace {

using picotrackers::kMaxPicoTrackers;
using picotrackers::kTrackerCountUnknown;
using picotrackers::PicoTracker;

#pragma pack(push, 1)
struct ConnectState {  // PxrFitnessBandConnectState
    uint8_t num;
    uint8_t ids[12];
};
#pragma pack(pop)

typedef void *(*PFN_CreateClient)();
typedef int (*PFN_GetConnectState)(void *client, ConnectState *state);
typedef int (*PFN_GetBattery)(void *client, int tracker_id, int *battery);
typedef int (*PFN_GetInt)(void *client, int *value);
typedef XrResult(XRAPI_PTR *PFN_StartCalibAppBD)(XrInstance instance, const char *package, int32_t flag);
typedef XrResult(XRAPI_PTR *PFN_StartCalibAppPico)(XrInstance instance);

constexpr int32_t kCalibAppMotionTracker2 = 1;  // CalibAppFlagBD::MOTION_TRACKER_2

// Рантайм: под своим мьютексом — калибровку просят из потока Java, а instance уничтожает поток трекинга.
std::mutex g_runtime_mutex;
XrInstance g_instance = XR_NULL_HANDLE;
PFN_StartCalibAppBD g_start_bd = nullptr;
PFN_StartCalibAppPico g_start_pico = nullptr;

struct Library {
    bool tried = false, loaded = false;
    void *client = nullptr;
    PFN_GetConnectState connect_state = nullptr;
    PFN_GetBattery battery = nullptr;
    PFN_GetInt calib_state = nullptr;
    PFN_GetInt max_battery = nullptr;
};
Library g_library;  // только из потока опроса (TrackingService)

std::mutex g_mutex;  // состояние трекеров и просьб
int8_t g_calibrated = -1;
uint8_t g_count = kTrackerCountUnknown;
PicoTracker g_trackers[kMaxPicoTrackers];
int g_max_battery = 0;  // полный заряд в единицах библиотеки (getSwiftTrackerMaxBattery; на 4 Pro — 10)
uint16_t g_calibration_request = 0;
int16_t g_calibration_result = picotrackers::kCalibrationNotRequested;

template <typename T>
T load(void *library, const char *name) {
    return reinterpret_cast<T>(dlsym(library, name));
}

bool load_library() {
    if (g_library.tried) return g_library.loaded;
    g_library.tried = true;
    void *library = dlopen("libtrackingclient.pxr.so", RTLD_NOW | RTLD_LOCAL);
    if (!library) {
        LOGW("pxr: libtrackingclient.pxr.so unavailable: %s", dlerror());
        return false;
    }
    auto create = load<PFN_CreateClient>(library, "CreateClient");
    g_library.connect_state = load<PFN_GetConnectState>(library, "GetSwiftConnectState");
    g_library.battery = load<PFN_GetBattery>(library, "GetSwiftTrackerBattery");
    g_library.calib_state = load<PFN_GetInt>(library, "GetSwiftCalibState");
    g_library.max_battery = load<PFN_GetInt>(library, "getSwiftTrackerMaxBattery");
    g_library.client = create ? create() : nullptr;
    LOGI("pxr: libtrackingclient.pxr.so loaded, client=%d connect=%d battery=%d calib=%d max=%d",
         g_library.client != nullptr, g_library.connect_state != nullptr, g_library.battery != nullptr,
         g_library.calib_state != nullptr, g_library.max_battery != nullptr);
    g_library.loaded = g_library.client && g_library.connect_state;
    return g_library.loaded;
}

// Заряд в процентах: по полному заряду из библиотеки; не сказала — уровни 0–5 (как у PICO SDK) или уже проценты.
int8_t to_percent(int raw, int max) {
    if (raw < 0) return -1;
    if (max <= 0) max = raw > 5 ? 100 : 5;
    return static_cast<int8_t>(std::clamp(raw * 100 / max, 0, 100));
}

}  // namespace

void set_runtime(XrInstance instance) {
    std::lock_guard<std::mutex> lock(g_runtime_mutex);
    g_instance = instance;
    g_start_bd = nullptr;
    g_start_pico = nullptr;
    if (instance == XR_NULL_HANDLE) return;

    auto get = [&](const char *name) {
        PFN_xrVoidFunction fn = nullptr;
        xrGetInstanceProcAddr(instance, name, &fn);
        return fn;
    };
    g_start_bd = reinterpret_cast<PFN_StartCalibAppBD>(get("xrStartBodyTrackingCalibAppBD"));
    g_start_pico = reinterpret_cast<PFN_StartCalibAppPico>(get("xrStartBodyTrackingCalibrationAppPICO"));
    // Какие функции трекеров PICO есть у рантайма — для будущих версий (заряд через OpenXR и т.п.).
    std::string found;
    for (const char *name : {"xrStartBodyTrackingCalibAppBD", "xrStartBodyTrackingCalibrationAppPICO",
                             "xrGetBodyTrackingStatePICO", "xrGetBodyTrackerBatteryPICO",
                             "xrGetBodyTrackerConnectStatePICO", "xrGetBodyTrackerCalibStatePICO",
                             "xrGetMotionTrackerBatteryStatePICO", "xrGetMotionTrackerLocationsBD"})
        if (get(name)) found += std::string(found.empty() ? "" : ",") + name;
    LOGI("runtime tracker functions: %s", found.empty() ? "none" : found.c_str());
}

int32_t start_calibration() {
    std::lock_guard<std::mutex> lock(g_runtime_mutex);
    if (g_instance == XR_NULL_HANDLE) return XR_ERROR_HANDLE_INVALID;
    XrResult result = XR_ERROR_FUNCTION_UNSUPPORTED;
    if (g_start_bd) result = g_start_bd(g_instance, "", kCalibAppMotionTracker2);
    if (XR_FAILED(result) && g_start_pico) result = g_start_pico(g_instance);
    LOGI("calibration via runtime: %d", result);
    return result;
}

void set_calibration_result(uint16_t request, int16_t result) {
    std::lock_guard<std::mutex> lock(g_mutex);
    g_calibration_request = request;
    g_calibration_result = result;
}

bool poll() {
    if (!load_library()) return false;

    alignas(8) uint8_t buffer[64]{};  // с запасом: структура PICO — 13 байт
    auto *state = reinterpret_cast<ConnectState *>(buffer);
    void *client = g_library.client;
    int r = g_library.connect_state(client, state);
    uint8_t count = r == 0 ? std::min<uint8_t>(state->num, kMaxPicoTrackers) : kTrackerCountUnknown;

    PicoTracker trackers[kMaxPicoTrackers];
    memset(trackers, 0, sizeof(trackers));
    int raw[kMaxPicoTrackers];
    std::fill(std::begin(raw), std::end(raw), -1);
    for (int i = 0; count != kTrackerCountUnknown && i < count; i++) {
        trackers[i].id = state->ids[i];
        if (g_library.battery && g_library.battery(client, state->ids[i], &raw[i]) != 0) raw[i] = -1;
    }
    int calibrated = -1;
    if (g_library.calib_state && g_library.calib_state(client, &calibrated) != 0) calibrated = -1;
    int max = 0;
    if (g_library.max_battery && g_library.max_battery(client, &max) != 0) max = 0;

    std::lock_guard<std::mutex> lock(g_mutex);
    if (max > 0) g_max_battery = max;
    bool changed = count != g_count || calibrated != g_calibrated;
    for (int i = 0; i < kMaxPicoTrackers; i++) {
        trackers[i].battery = to_percent(raw[i], g_max_battery);
        changed |= trackers[i].id != g_trackers[i].id || trackers[i].battery != g_trackers[i].battery;
    }
    if (changed) {
        std::string list;
        for (int i = 0; count != kTrackerCountUnknown && i < count; i++)
            list += " #" + std::to_string(trackers[i].id) + "=" + std::to_string(raw[i]);
        LOGI("pxr: result=%d trackers=%d%s calibrated=%d max=%d", r, count == kTrackerCountUnknown ? -1 : count,
             list.c_str(), calibrated, g_max_battery);
    }
    g_count = count;
    g_calibrated = static_cast<int8_t>(calibrated < 0 ? -1 : calibrated != 0);
    memcpy(g_trackers, trackers, sizeof(trackers));
    return true;
}

std::string describe(bool russian) {
    std::lock_guard<std::mutex> lock(g_mutex);
    if (g_count == kTrackerCountUnknown)
        return russian ? "Трекеры PICO: нет сведений о подключении и заряде" : "PICO trackers: no connection or battery info";
    std::string text = (russian ? "Трекеры PICO: " : "PICO trackers: ") + std::to_string(g_count);
    std::string batteries;
    for (int i = 0; i < g_count; i++) {
        if (g_trackers[i].battery < 0) continue;
        batteries += (batteries.empty() ? "" : ", ") + std::to_string(g_trackers[i].battery) + " %";
    }
    if (!batteries.empty()) text += (russian ? ", заряд " : ", battery ") + batteries;
    if (g_calibrated == 1) text += russian ? ", откалиброваны" : ", calibrated";
    if (g_calibrated == 0) text += russian ? ", НЕ откалиброваны" : ", NOT calibrated";
    return text;
}

bool not_calibrated() {
    std::lock_guard<std::mutex> lock(g_mutex);
    return g_calibrated == 0 && g_count != kTrackerCountUnknown && g_count > 0;
}

picotrackers::StatusPacket status_packet(int8_t body_status, int8_t body_error) {
    picotrackers::StatusPacket packet{};
    memcpy(packet.magic, picotrackers::kStatusMagic, sizeof(packet.magic));
    packet.body_status = body_status;
    packet.body_error = body_error;
    std::lock_guard<std::mutex> lock(g_mutex);
    packet.calibrated = g_calibrated;
    packet.tracker_count = g_count;
    memcpy(packet.trackers, g_trackers, sizeof(packet.trackers));
    packet.calibration_request = g_calibration_request;
    packet.calibration_result = g_calibration_result;
    return packet;
}

}  // namespace pico
