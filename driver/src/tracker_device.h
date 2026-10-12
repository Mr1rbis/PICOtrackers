// Один виртуальный трекер SteamVR, привязанный к суставу скелета PICO.
#pragma once

#include <openvr_driver.h>

#include <string>

#include "protocol.h"

namespace picotrackers {

class TrackerDevice final : public vr::ITrackedDeviceServerDriver {
public:
    TrackerDevice(const std::string &name, Joint joint, const std::string &role);

    const std::string &name() const { return name_; }
    const std::string &serial() const { return serial_; }
    const std::string &role() const { return role_; }
    Joint joint() const { return joint_; }
    const vr::DriverPose_t &pose() const { return pose_; }

    // Включён пользователем (выключенный трекер SteamVR видит как отключённое устройство).
    bool enabled = false;
    // Уже передан SteamVR через TrackedDeviceAdded (убрать устройство до перезапуска SteamVR нельзя).
    bool added = false;

    // Передаёт позу в SteamVR, если устройство уже активировано.
    void update(const vr::DriverPose_t &pose);
    // Заряд для SteamVR, %: у виртуального трекера — самый низкий из трекеров PICO; < 0 — неизвестен.
    void set_battery(int percent);

    // Объект, который отдаём SteamVR в TrackedDeviceAdded (на Windows — обёртка под MSVC ABI).
    vr::ITrackedDeviceServerDriver *steamvr_interface();

#ifdef _WIN32
    // SteamVR собран MSVC, драйвер — MinGW. У методов, возвращающих структуру (GetPose), MSVC передаёт
    // this первым, а адрес результата вторым; MinGW — наоборот. Поэтому SteamVR получает объект с
    // собственной таблицей функций, которые принимают аргументы в порядке MSVC (см. tracker_device.cpp).
    struct MsvcShim {
        const void *const *vtable;
        TrackerDevice *device;
    };
#endif

    vr::EVRInitError Activate(uint32_t object_id) override;
    void Deactivate() override;
    void EnterStandby() override {}
    void *GetComponent(const char *) override { return nullptr; }
    void DebugRequest(const char *, char *response, uint32_t response_size) override;
    vr::DriverPose_t GetPose() override { return pose_; }

private:
    std::string name_;
    std::string serial_;
    std::string role_;
    Joint joint_;
    uint32_t object_id_ = vr::k_unTrackedDeviceIndexInvalid;
    vr::DriverPose_t pose_{};
    int battery_ = -1;

    void apply_battery();

#ifdef _WIN32
    MsvcShim shim_{nullptr, this};
#endif
};

// Поза «нет данных» с корректными единичными преобразованиями.
vr::DriverPose_t invalid_pose();

}  // namespace picotrackers
