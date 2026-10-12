// Драйвер SteamVR «picotrackers»: принимает скелет тела со шлема PICO (UDP) и показывает
// выбранные суставы как трекеры SteamVR. Связь пространств шлема и SteamVR находит Calibration.
// Настройки программы на ПК (трекинг вкл/выкл, набор трекеров, частота) драйвер хранит в настройках
// SteamVR и работает с ними и без программы. Пока трекеры PICO не подключены к шлему, в SteamVR
// ничего не передаётся: скелет без них PICO достраивает сам, и ноги были бы выдуманными.
// Состояние трекеров PICO со шлема (откалиброваны ли, заряд) уходит программе, заряд — и в SteamVR (суставам ноги —
// заряд трекера на этой ноге, поясу — трекера на поясе, остальным — самый низкий);
// просьбу программы «откалибровать трекеры» драйвер передаёт шлему.

#include <openvr_driver.h>

#include <algorithm>
#include <chrono>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <sstream>
#include <string>
#include <vector>

#include "calibration.h"
#include "log.h"
#include "protocol.h"
#include "receiver.h"
#include "tracker_device.h"

namespace picotrackers {
namespace {

constexpr char kSettingsSection[] = "driver_picotrackers";
// Все суставы, для которых в SteamVR есть роль трекера; "all" в настройке — все 24 сустава.
constexpr char kDefaultTrackers[] =
    "waist,chest,left_foot,right_foot,left_knee,right_knee,left_elbow,right_elbow,left_shoulder,right_shoulder";
constexpr auto kStaleAfter = std::chrono::milliseconds(500);  // без пакетов — трекеры «потеряны»
constexpr auto kAppTimeout = std::chrono::seconds(3);          // программа на ПК шлёт команду раз в секунду
constexpr auto kAppStatusInterval = std::chrono::milliseconds(500);
constexpr auto kAppPosesInterval = std::chrono::milliseconds(33);  // позы для 3D-вида программы
constexpr int kDefaultRate = 100, kMinRate = 10, kMaxRate = 250;     // снимков/с со шлема
constexpr double kMaxHmdSpeed = 0.3;           // м/с — быстрее калибровочную пару не берём
constexpr double kMaxHmdAngularSpeed = 1.0;    // рад/с
constexpr auto kStatusLogInterval = std::chrono::seconds(10);
constexpr auto kPicoStatusStale = std::chrono::seconds(3);  // шлем шлёт состояние трекеров раз в секунду

struct TrackerSpec {
    const char *name;
    Joint joint;
    const char *role;
    TrackerPlace place;  // чей заряд показывать: трекера PICO на этой ноге / поясе; kPlaceUnknown — самый низкий
};

// Все 24 сустава скелета. Стопы (left/right_foot) — по суставу лодыжки: он ближе к кости стопы
// аватара; сустав носка (kLeftFoot) — left/right_toes. TrackerRole_None — роли в SteamVR нет.
constexpr TrackerSpec kTrackerSpecs[] = {
    {"waist", kPelvis, "TrackerRole_Waist", kPlaceWaist},
    {"chest", kSpine3, "TrackerRole_Chest", kPlaceUnknown},
    {"left_foot", kLeftAnkle, "TrackerRole_LeftFoot", kPlaceLeftLeg},
    {"right_foot", kRightAnkle, "TrackerRole_RightFoot", kPlaceRightLeg},
    {"left_knee", kLeftKnee, "TrackerRole_LeftKnee", kPlaceLeftLeg},
    {"right_knee", kRightKnee, "TrackerRole_RightKnee", kPlaceRightLeg},
    {"left_elbow", kLeftElbow, "TrackerRole_LeftElbow", kPlaceUnknown},
    {"right_elbow", kRightElbow, "TrackerRole_RightElbow", kPlaceUnknown},
    {"left_shoulder", kLeftShoulder, "TrackerRole_LeftShoulder", kPlaceUnknown},
    {"right_shoulder", kRightShoulder, "TrackerRole_RightShoulder", kPlaceUnknown},
    {"left_hip", kLeftHip, "TrackerRole_None", kPlaceLeftLeg},
    {"right_hip", kRightHip, "TrackerRole_None", kPlaceRightLeg},
    {"spine1", kSpine1, "TrackerRole_None", kPlaceWaist},
    {"spine2", kSpine2, "TrackerRole_None", kPlaceUnknown},
    {"left_toes", kLeftFoot, "TrackerRole_None", kPlaceLeftLeg},
    {"right_toes", kRightFoot, "TrackerRole_None", kPlaceRightLeg},
    {"neck", kNeck, "TrackerRole_None", kPlaceUnknown},
    {"left_collar", kLeftCollar, "TrackerRole_None", kPlaceUnknown},
    {"right_collar", kRightCollar, "TrackerRole_None", kPlaceUnknown},
    {"head", kHead, "TrackerRole_None", kPlaceUnknown},
    {"left_wrist", kLeftWrist, "TrackerRole_None", kPlaceUnknown},
    {"right_wrist", kRightWrist, "TrackerRole_None", kPlaceUnknown},
    {"left_hand", kLeftHand, "TrackerRole_None", kPlaceUnknown},
    {"right_hand", kRightHand, "TrackerRole_None", kPlaceUnknown},
};

Quat quat_from_matrix(const vr::HmdMatrix34_t &mat) {
    const auto &m = mat.m;
    double trace = m[0][0] + m[1][1] + m[2][2];
    Quat q;
    if (trace > 0) {
        double s = std::sqrt(trace + 1) * 2;
        q = {0.25 * s, (m[2][1] - m[1][2]) / s, (m[0][2] - m[2][0]) / s, (m[1][0] - m[0][1]) / s};
    } else if (m[0][0] > m[1][1] && m[0][0] > m[2][2]) {
        double s = std::sqrt(1 + m[0][0] - m[1][1] - m[2][2]) * 2;
        q = {(m[2][1] - m[1][2]) / s, 0.25 * s, (m[0][1] + m[1][0]) / s, (m[0][2] + m[2][0]) / s};
    } else if (m[1][1] > m[2][2]) {
        double s = std::sqrt(1 + m[1][1] - m[0][0] - m[2][2]) * 2;
        q = {(m[0][2] - m[2][0]) / s, (m[0][1] + m[1][0]) / s, 0.25 * s, (m[1][2] + m[2][1]) / s};
    } else {
        double s = std::sqrt(1 + m[2][2] - m[0][0] - m[1][1]) * 2;
        q = {(m[1][0] - m[0][1]) / s, (m[0][2] + m[2][0]) / s, (m[1][2] + m[2][1]) / s, 0.25 * s};
    }
    return q.normalized();
}

Vec3 joint_position(const JointPose &j) { return {j.position[0], j.position[1], j.position[2]}; }

Quat joint_rotation(const JointPose &j) {
    return Quat{j.orientation[3], j.orientation[0], j.orientation[1], j.orientation[2]}.normalized();
}

bool joint_valid(const JointPose &j) {
    constexpr uint32_t kValid = kJointOrientationValid | kJointPositionValid;
    return (j.flags & kValid) == kValid;
}

double length(const vr::HmdVector3_t &v) { return std::sqrt(v.v[0] * v.v[0] + v.v[1] * v.v[1] + v.v[2] * v.v[2]); }

class Provider : public vr::IServerTrackedDeviceProvider {
public:
    vr::EVRInitError Init(vr::IVRDriverContext *context) override {
        VR_INIT_SERVER_DRIVER_CONTEXT(context);

        vr::EVRSettingsError error;
        int port = vr::VRSettings()->GetInt32(kSettingsSection, "port", &error);
        if (error != vr::VRSettingsError_None || port <= 0) port = kDefaultPort;

        // Создаём все суставы сразу; в SteamVR попадают только включённые (и при первых данных).
        for (const auto &spec : kTrackerSpecs)
            trackers_.push_back(std::make_unique<TrackerDevice>(spec.name, spec.joint, spec.role));
        char list[512] = "";
        vr::VRSettings()->GetString(kSettingsSection, "trackers", list, sizeof(list), &error);
        set_enabled(error == vr::VRSettingsError_None && list[0] ? list : kDefaultTrackers, false);

        rate_ = vr::VRSettings()->GetInt32(kSettingsSection, "rate", &error);
        if (error != vr::VRSettingsError_None || rate_ <= 0) rate_ = kDefaultRate;
        receiver_.set_rate(static_cast<uint16_t>(std::clamp(rate_, kMinRate, kMaxRate)));

        tracking_ = vr::VRSettings()->GetBool(kSettingsSection, "tracking", &error);
        if (error != vr::VRSettingsError_None) tracking_ = true;
        receiver_.set_request_data(tracking_);

        char ip[64] = "";
        vr::VRSettings()->GetString(kSettingsSection, "headset_ip", ip, sizeof(ip), &error);
        saved_headset_ip_ = error == vr::VRSettingsError_None ? ip : "";

        log("запуск: трекинг %s, трекеры «%s», %d снимков/с, UDP-порт %d", tracking_ ? "включён" : "выключен",
            enabled_list_.c_str(), rate_, port);
        if (!receiver_.start(static_cast<uint16_t>(port), saved_headset_ip_))
            log("ОШИБКА: не удалось открыть UDP-порт %d", port);
        return vr::VRInitError_None;
    }

    void Cleanup() override {
        receiver_.stop();
        trackers_.clear();
        VR_CLEANUP_SERVER_DRIVER_CONTEXT();
    }

    const char *const *GetInterfaceVersions() override { return vr::k_InterfaceVersions; }
    bool ShouldBlockStandbyMode() override { return false; }
    void EnterStandby() override {}
    void LeaveStandby() override {}

    void RunFrame() override {
        vr::VREvent_t event;
        while (vr::VRServerDriverHost()->PollNextEvent(&event, sizeof(event))) {
        }

        auto now = Receiver::Clock::now();
        apply_app_command(now);

        vr::TrackedDevicePose_t hmd{};
        vr::VRServerDriverHost()->GetRawTrackedDevicePoses(0, &hmd, 1);  // индекс 0 — всегда HMD
        hmd_connected_ = hmd.bDeviceIsConnected;

        Receiver::Snapshot snapshot;
        fresh_ = receiver_.latest(snapshot) && now - snapshot.received < kStaleAfter;
        packet_flags_ = fresh_ ? snapshot.packet.flags : 0;
        // Позы — только при подключённых трекерах: без них шлем шлёт лишь пакеты состояния (joint_count = 0),
        // а старое приложение — скелет, который PICO достроил без трекеров.
        poses_ = fresh_ && (packet_flags_ & kFlagTrackersReady) && snapshot.packet.joint_count > 0;
        if (fresh_) {
            remember_headset_ip();
            bool ready = packet_flags_ & kFlagTrackersReady;
            if (ready != trackers_ready_ || !trackers_known_) {
                trackers_ready_ = ready;
                trackers_known_ = true;
                log(ready ? "трекеры PICO подключены — позы идут в SteamVR"
                          : "трекеры PICO не подключены к шлему — в SteamVR ничего не передаётся");
            }
        }
        if (poses_) feed_calibration(snapshot.packet, hmd, seconds(now));
        update_pico_status(now);

        for (size_t i = 0; i < trackers_.size(); i++) {
            auto &tracker = trackers_[i];
            tracker->set_battery(battery_for(kTrackerSpecs[i].place));
            if (poses_ && tracker->enabled && !tracker->added) add_device(*tracker);
            if (!tracker->added) continue;
            vr::DriverPose_t pose = make_pose(*tracker, snapshot.packet);
            // Выключен («удалён») или трекеры PICO не подключены: игры его не видят.
            if (!tracker->enabled || (fresh_ && !trackers_ready_)) pose.deviceIsConnected = false;
            tracker->update(pose);
        }

        if (now >= next_rate_) {
            next_rate_ = now + std::chrono::seconds(1);
            uint64_t packets = receiver_.packet_count();
            packets_per_second_ = static_cast<int>(packets - last_packet_count_);
            last_packet_count_ = packets;
        }
        if (now >= next_app_status_) {
            next_app_status_ = now + kAppStatusInterval;
            receiver_.send_to_app(status_json());
        }
        if (poses_ && app_connected_ && now >= next_app_poses_) {
            next_app_poses_ = now + kAppPosesInterval;
            receiver_.send_to_app(poses_json(snapshot.packet));
        }
        if (now >= next_status_log_) {
            next_status_log_ = now + kStatusLogInterval;
            log("пакетов/с %d, калибровка: %s", packets_per_second_, calibration_.describe().c_str());
        }
    }

private:
    static double seconds(Receiver::Clock::time_point t) {
        return std::chrono::duration<double>(t.time_since_epoch()).count();
    }

    // Включает перечисленные суставы ("all" — все, "none" — ни одного), остальные выключает.
    void set_enabled(const std::string &list, bool persist) {
        std::stringstream names(list);
        std::string name;
        std::vector<std::string> wanted;
        while (std::getline(names, name, ',')) {
            name.erase(0, name.find_first_not_of(" \t"));
            name.erase(name.find_last_not_of(" \t") + 1);
            if (!name.empty()) wanted.push_back(name);
        }
        std::string normalized;
        for (auto &tracker : trackers_) {
            tracker->enabled = false;
            for (const auto &w : wanted) tracker->enabled |= w == tracker->name() || w == "all";
            if (tracker->enabled) normalized += (normalized.empty() ? "" : ",") + tracker->name();
        }
        if (normalized == enabled_list_) return;
        enabled_list_ = normalized;
        if (persist) {
            // Запоминаем в настройках SteamVR: без программы на ПК драйвер возьмёт тот же набор.
            vr::VRSettings()->SetString(kSettingsSection, "trackers",
                                        enabled_list_.empty() ? "none" : enabled_list_.c_str());
            log("трекеры: %s", enabled_list_.empty() ? "все выключены" : enabled_list_.c_str());
        }
    }

    // Команда программы на ПК: «tracking=0|1 rate=N trackers=a,b,c». Всё запоминается в настройках SteamVR:
    // без программы (нет команд 3 с) и после перезапуска SteamVR драйвер работает так, как она его оставила.
    void apply_app_command(Receiver::Clock::time_point now) {
        std::string command;
        Receiver::Clock::time_point received;
        bool connected = receiver_.app_command(command, received) && now - received < kAppTimeout;
        if (connected != app_connected_) {
            app_connected_ = connected;
            if (connected)
                log("программа на ПК подключилась");
            else
                log("программа на ПК отключилась — работаю с её настройками: трекинг %s, трекеры «%s», %d снимков/с",
                    tracking_ ? "включён" : "выключен", enabled_list_.empty() ? "нет" : enabled_list_.c_str(), rate_);
        }
        if (!connected) return;

        std::stringstream parts(command);
        std::string part;
        while (parts >> part) {
            auto eq = part.find('=');
            if (eq == std::string::npos) continue;
            std::string key = part.substr(0, eq), value = part.substr(eq + 1);
            if (key == "tracking") set_tracking(value != "0");
            if (key == "trackers") set_enabled(value, true);
            if (key == "rate") set_rate(std::atoi(value.c_str()));
            if (key == "calibrate") request_calibration(std::atoi(value.c_str()));
        }
    }

    // Просьба программы открыть на шлеме калибровку трекеров PICO; номер меняется при каждом нажатии кнопки.
    void request_calibration(int request) {
        if (request <= 0 || request > 0xffff || request == calibration_request_) return;
        calibration_request_ = request;
        receiver_.request_calibration(static_cast<uint16_t>(request));
        log("программа просит открыть калибровку трекеров PICO на шлеме (%d)", request);
    }

    // Заряд для трекера SteamVR с местом place: трекера PICO на этом месте, иначе самый низкий.
    int battery_for(TrackerPlace place) const {
        int battery = place <= kPlaceWaist ? battery_[place] : -1;
        return battery >= 0 ? battery : battery_[kPlaceUnknown];
    }

    // Состояние трекеров PICO со шлема: заряд — трекерам SteamVR (по месту на теле), остальное — программе.
    void update_pico_status(Receiver::Clock::time_point now) {
        Receiver::Status status;
        pico_known_ = receiver_.latest_status(status) && now - status.received < kPicoStatusStale;
        if (pico_known_) pico_ = status.packet;
        int battery[4] = {-1, -1, -1, -1};  // [kPlaceUnknown] — самый низкий из всех, остальные — по месту
        if (pico_known_ && pico_.tracker_count != kTrackerCountUnknown) {
            for (int i = 0; i < std::min<int>(pico_.tracker_count, kMaxPicoTrackers); i++) {
                const PicoTracker &t = pico_.trackers[i];
                if (t.battery < 0) continue;
                if (battery[0] < 0 || t.battery < battery[0]) battery[0] = t.battery;
                TrackerPlace place = tracker_place(t.id);
                if (place != kPlaceUnknown && place <= kPlaceWaist) battery[place] = t.battery;
            }
        }
        if (!std::equal(std::begin(battery), std::end(battery), std::begin(battery_))) {
            std::copy(std::begin(battery), std::end(battery), std::begin(battery_));
            if (battery[0] >= 0)
                log("заряд трекеров PICO: левая нога %d %%, правая %d %%, пояс %d %%, самый низкий %d %% (-1 — нет)",
                    battery[kPlaceLeftLeg], battery[kPlaceRightLeg], battery[kPlaceWaist], battery[0]);
        }
        std::string description = pico_known_ ? describe_pico() : "";
        if (description != pico_description_) {
            pico_description_ = description;
            if (!description.empty()) log("трекеры PICO: %s", description.c_str());
        }
        if (pico_known_ && pico_.calibration_request == calibration_request_ &&
            pico_.calibration_result != calibration_result_) {
            calibration_result_ = pico_.calibration_result;
            log("калибровка трекеров PICO на шлеме: %s (%d)",
                calibration_result_ >= 0 ? "открыта" : "не открылась", calibration_result_);
        }
    }

    std::string describe_pico() const {
        char buf[160];
        std::snprintf(buf, sizeof(buf), "состояние %d, причина %d, откалиброваны %d, подключено %d", pico_.body_status,
                      pico_.body_error, pico_.calibrated,
                      pico_.tracker_count == kTrackerCountUnknown ? -1 : pico_.tracker_count);
        return buf;
    }

    void set_tracking(bool tracking) {
        if (tracking == tracking_) return;
        tracking_ = tracking;
        receiver_.set_request_data(tracking);
        vr::VRSettings()->SetBool(kSettingsSection, "tracking", tracking);
        log(tracking ? "трекинг включён" : "трекинг выключен программой на ПК");
    }

    void set_rate(int rate) {
        rate = std::clamp(rate, kMinRate, kMaxRate);
        if (rate == rate_) return;
        rate_ = rate;
        receiver_.set_rate(static_cast<uint16_t>(rate));
        vr::VRSettings()->SetInt32(kSettingsSection, "rate", rate);
        log("частота снимков со шлема: %d/с", rate);
    }

    // Позы всех суставов (STAGE шлема) для 3D-вида: [x, y, z, валиден].
    std::string poses_json(const Packet &packet) const {
        std::string json = "{\"type\":\"poses\",\"joints\":[";
        char buf[96];
        for (uint32_t i = 0; i < kJointCount; i++) {
            const JointPose &j = packet.joints[i];
            bool valid = i < packet.joint_count && joint_valid(j);
            std::snprintf(buf, sizeof(buf), "%s[%.3f,%.3f,%.3f,%d]", i ? "," : "", j.position[0], j.position[1],
                          j.position[2], valid ? 1 : 0);
            json += buf;
        }
        return json + "]}";
    }

    std::string status_json() const {
        std::string json = "{\"type\":\"status\"";
        char buf[256];
        std::snprintf(buf, sizeof(buf),
                      ",\"app\":%s,\"tracking\":%s,\"data\":%s,\"hmd\":%s,\"packets_per_s\":%d,\"headset_ip\":\"%s\""
                      ",\"body_tracked\":%s,\"trackers_ready\":%s,\"rate\":%d",
                      app_connected_ ? "true" : "false", tracking_ ? "true" : "false", fresh_ ? "true" : "false",
                      hmd_connected_ ? "true" : "false", packets_per_second_, receiver_.headset_ip().c_str(),
                      packet_flags_ & kFlagAllJointsTracked ? "true" : "false",
                      packet_flags_ & kFlagTrackersReady ? "true" : "false", rate_);
        json += buf;
        std::snprintf(buf, sizeof(buf), ",\"calibration\":{\"valid\":%s,\"error\":%.4f,\"samples\":%zu,\"yaw\":%.2f}",
                      calibration_.valid() ? "true" : "false", calibration_.error(), calibration_.sample_count(),
                      calibration_.transform().rotation.yaw() * 180 / kPi);
        json += buf;
        if (pico_known_) {
            std::snprintf(buf, sizeof(buf),
                          ",\"pico\":{\"status\":%d,\"error\":%d,\"calibrated\":%d,\"count\":%d"
                          ",\"calibration_request\":%d,\"calibration_result\":%d,\"trackers\":[",
                          pico_.body_status, pico_.body_error, pico_.calibrated,
                          pico_.tracker_count == kTrackerCountUnknown ? -1 : pico_.tracker_count,
                          pico_.calibration_request, pico_.calibration_result);
            json += buf;
            for (int i = 0; pico_.tracker_count != kTrackerCountUnknown && i < std::min<int>(pico_.tracker_count, kMaxPicoTrackers); i++) {
                std::snprintf(buf, sizeof(buf), "%s{\"id\":%d,\"place\":%d,\"battery\":%d}", i ? "," : "",
                              tracker_number(pico_.trackers[i].id), tracker_place(pico_.trackers[i].id),
                              pico_.trackers[i].battery);
                json += buf;
            }
            json += "]}";
        }
        json += ",\"trackers\":[";
        for (size_t i = 0; i < trackers_.size(); i++) {
            const auto &t = *trackers_[i];
            std::snprintf(buf, sizeof(buf), "%s{\"name\":\"%s\",\"role\":\"%s\",\"enabled\":%s,\"added\":%s,\"valid\":%s}",
                          i ? "," : "", t.name().c_str(), t.role().c_str(), t.enabled ? "true" : "false",
                          t.added ? "true" : "false", t.enabled && t.pose().poseIsValid ? "true" : "false");
            json += buf;
        }
        return json + "]}";
    }

    // IP шлема запоминается в настройках SteamVR: при следующем запуске просьба «нужны данные»
    // уйдёт на него напрямую, даже если широковещательные пакеты не доходят.
    void remember_headset_ip() {
        std::string ip = receiver_.headset_ip();
        if (ip.empty() || ip == saved_headset_ip_) return;
        saved_headset_ip_ = ip;
        vr::VRSettings()->SetString(kSettingsSection, "headset_ip", ip.c_str());
        log("шлем: %s", ip.c_str());
    }

    void add_device(TrackerDevice &tracker) {
        tracker.added = true;
        // Роль трекера (ноги, пояс…) — как её задаёт окно «Manage Trackers» в SteamVR.
        std::string path = "/devices/picotrackers/" + tracker.serial();
        vr::VRSettings()->SetString(vr::k_pch_Trackers_Section, path.c_str(), tracker.role().c_str());
        vr::VRServerDriverHost()->TrackedDeviceAdded(tracker.serial().c_str(), vr::TrackedDeviceClass_GenericTracker,
                                                     tracker.steamvr_interface());
        log("трекер %s добавлен в SteamVR", tracker.serial().c_str());
    }

    void feed_calibration(const Packet &packet, const vr::TrackedDevicePose_t &hmd, double now) {
        const JointPose &head = packet.joints[kHead];
        if (packet.joint_count <= kHead || !joint_valid(head)) return;
        if (!hmd.bPoseIsValid || !hmd.bDeviceIsConnected) return;
        if (length(hmd.vVelocity) > kMaxHmdSpeed || length(hmd.vAngularVelocity) > kMaxHmdAngularSpeed) return;

        const auto &m = hmd.mDeviceToAbsoluteTracking.m;
        Calibration::Sample sample{{m[0][3], m[1][3], m[2][3]}, quat_from_matrix(hmd.mDeviceToAbsoluteTracking),
                                   joint_position(head), joint_rotation(head)};
        bool was_valid = calibration_.valid();
        if (calibration_.add(sample, now) && calibration_.valid() != was_valid)
            log("калибровка готова: %s", calibration_.describe().c_str());
    }

    vr::DriverPose_t make_pose(const TrackerDevice &tracker, const Packet &packet) const {
        vr::DriverPose_t pose = invalid_pose();
        const JointPose &joint = packet.joints[tracker.joint()];
        if (!poses_ || !calibration_.valid() || tracker.joint() >= packet.joint_count || !joint_valid(joint)) {
            pose.result = vr::TrackingResult_Running_OutOfRange;
            return pose;
        }

        // Суставы приходят в STAGE шлема; в SteamVR их переводит преобразование калибровки.
        const RigidTransform &t = calibration_.transform();
        pose.qWorldFromDriverRotation = {t.rotation.w, t.rotation.x, t.rotation.y, t.rotation.z};
        pose.vecWorldFromDriverTranslation[0] = t.translation.x;
        pose.vecWorldFromDriverTranslation[1] = t.translation.y;
        pose.vecWorldFromDriverTranslation[2] = t.translation.z;
        Quat r = joint_rotation(joint);
        pose.qRotation = {r.w, r.x, r.y, r.z};
        for (int i = 0; i < 3; i++) pose.vecPosition[i] = joint.position[i];
        pose.poseIsValid = true;
        pose.result = vr::TrackingResult_Running_OK;
        return pose;
    }

    std::vector<std::unique_ptr<TrackerDevice>> trackers_;
    std::string enabled_list_ = "-";  // «-» — ещё не задан, чтобы первый set_enabled применился
    Receiver receiver_;
    Calibration calibration_;
    std::string saved_headset_ip_;

    bool fresh_ = false;           // шлем на связи: пакеты приходят
    bool poses_ = false;           // и в них позы суставов при подключённых трекерах
    bool trackers_ready_ = false;  // трекеры PICO подключены (по последнему свежему пакету)
    bool trackers_known_ = false;  // хоть один пакет уже был — состояние трекеров известно
    uint32_t packet_flags_ = 0;    // флаги последнего свежего пакета (Packet::flags)
    bool hmd_connected_ = false;
    bool pico_known_ = false;  // свежее состояние трекеров PICO со шлема (приложение 1.7+)
    StatusPacket pico_{};
    std::string pico_description_;
    int battery_[4] = {-1, -1, -1, -1};  // заряд трекеров PICO, %, по TrackerPlace ([0] — самый низкий); -1 — неизвестен
    int calibration_request_ = 0;
    int calibration_result_ = kCalibrationNotRequested;
    bool app_connected_ = false;
    bool tracking_ = true;
    int packets_per_second_ = 0;
    uint64_t last_packet_count_ = 0;
    int rate_ = kDefaultRate;
    Receiver::Clock::time_point next_rate_, next_app_status_, next_app_poses_, next_status_log_;
};

Provider g_provider;

}  // namespace
}  // namespace picotrackers

#ifdef _WIN32
#define PICOTRACKERS_EXPORT __declspec(dllexport)
#else
#define PICOTRACKERS_EXPORT __attribute__((visibility("default")))
#endif

extern "C" PICOTRACKERS_EXPORT void *HmdDriverFactory(const char *interface_name, int *return_code) {
    if (std::strcmp(interface_name, vr::IServerTrackedDeviceProvider_Version) == 0) return &picotrackers::g_provider;
    if (return_code) *return_code = vr::VRInitError_Init_InterfaceNotFound;
    return nullptr;
}
