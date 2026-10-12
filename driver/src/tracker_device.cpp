#include "tracker_device.h"

#include "log.h"

namespace picotrackers {

vr::DriverPose_t invalid_pose() {
    vr::DriverPose_t pose{};
    pose.qWorldFromDriverRotation.w = 1;
    pose.qDriverFromHeadRotation.w = 1;
    pose.qRotation.w = 1;
    pose.deviceIsConnected = true;
    pose.poseIsValid = false;
    pose.result = vr::TrackingResult_Uninitialized;
    return pose;
}

TrackerDevice::TrackerDevice(const std::string &name, Joint joint, const std::string &role)
    : name_(name), serial_("PICOTRK-" + name), role_(role), joint_(joint), pose_(invalid_pose()) {}

vr::EVRInitError TrackerDevice::Activate(uint32_t object_id) {
    object_id_ = object_id;
    auto *props = vr::VRProperties();
    auto c = props->TrackedDeviceToPropertyContainer(object_id);

    props->SetStringProperty(c, vr::Prop_TrackingSystemName_String, "picotrackers");
    props->SetStringProperty(c, vr::Prop_ManufacturerName_String, "PICOtrackers");
    props->SetStringProperty(c, vr::Prop_ModelNumber_String, "PICO Body Tracker");
    props->SetStringProperty(c, vr::Prop_SerialNumber_String, serial_.c_str());
    props->SetStringProperty(c, vr::Prop_RenderModelName_String, "{htc}vr_tracker_vive_1_0");
    props->SetStringProperty(c, vr::Prop_ControllerType_String, "picotrackers_tracker");
    props->SetStringProperty(c, vr::Prop_InputProfilePath_String,
                             "{picotrackers}/input/picotrackers_tracker_profile.json");
    props->SetInt32Property(c, vr::Prop_ControllerRoleHint_Int32, vr::TrackedControllerRole_OptOut);
    props->SetBoolProperty(c, vr::Prop_WillDriftInYaw_Bool, false);
    props->SetBoolProperty(c, vr::Prop_DeviceIsWireless_Bool, true);

    // Иконки статуса в окне SteamVR — свои (силуэт трекера PICO, driver/icons/make_icons.py), рядом версии _2x.
    props->SetStringProperty(c, vr::Prop_NamedIconPathDeviceOff_String, "{picotrackers}/icons/pico_tracker_off.png");
    props->SetStringProperty(c, vr::Prop_NamedIconPathDeviceSearching_String,
                             "{picotrackers}/icons/pico_tracker_searching.gif");
    props->SetStringProperty(c, vr::Prop_NamedIconPathDeviceSearchingAlert_String,
                             "{picotrackers}/icons/pico_tracker_searching_alert.gif");
    props->SetStringProperty(c, vr::Prop_NamedIconPathDeviceReady_String, "{picotrackers}/icons/pico_tracker_ready.png");
    props->SetStringProperty(c, vr::Prop_NamedIconPathDeviceReadyAlert_String,
                             "{picotrackers}/icons/pico_tracker_ready_alert.png");
    props->SetStringProperty(c, vr::Prop_NamedIconPathDeviceNotReady_String,
                             "{picotrackers}/icons/pico_tracker_error.png");
    props->SetStringProperty(c, vr::Prop_NamedIconPathDeviceStandby_String,
                             "{picotrackers}/icons/pico_tracker_standby.png");
    props->SetStringProperty(c, vr::Prop_NamedIconPathDeviceAlertLow_String,
                             "{picotrackers}/icons/pico_tracker_ready_low.png");

    apply_battery();
    log("трекер %s (%s) активирован, id %u", serial_.c_str(), role_.c_str(), object_id);
    return vr::VRInitError_None;
}

void TrackerDevice::Deactivate() { object_id_ = vr::k_unTrackedDeviceIndexInvalid; }

void TrackerDevice::set_battery(int percent) {
    if (percent == battery_) return;
    battery_ = percent;
    apply_battery();
}

void TrackerDevice::apply_battery() {
    if (object_id_ == vr::k_unTrackedDeviceIndexInvalid) return;
    auto *props = vr::VRProperties();
    auto c = props->TrackedDeviceToPropertyContainer(object_id_);
    props->SetBoolProperty(c, vr::Prop_DeviceProvidesBatteryStatus_Bool, battery_ >= 0);
    props->SetBoolProperty(c, vr::Prop_DeviceIsCharging_Bool, false);
    if (battery_ >= 0) props->SetFloatProperty(c, vr::Prop_DeviceBatteryPercentage_Float, battery_ / 100.0f);
}

void TrackerDevice::DebugRequest(const char *, char *response, uint32_t response_size) {
    if (response_size > 0) response[0] = '\0';
}

#ifdef _WIN32
namespace {

using Shim = TrackerDevice::MsvcShim;

// Функции-методы в порядке слотов ITrackedDeviceServerDriver; первый аргумент — this (как у MSVC).
vr::EVRInitError shim_activate(Shim *self, uint32_t id) { return self->device->Activate(id); }
void shim_deactivate(Shim *self) { self->device->Deactivate(); }
void shim_enter_standby(Shim *self) { self->device->EnterStandby(); }
void *shim_get_component(Shim *self, const char *name) { return self->device->GetComponent(name); }
void shim_debug_request(Shim *self, const char *request, char *response, uint32_t size) {
    self->device->DebugRequest(request, response, size);
}
vr::DriverPose_t *shim_get_pose(Shim *self, vr::DriverPose_t *result) {
    *result = self->device->GetPose();
    return result;
}

const void *const kShimVtable[] = {
    reinterpret_cast<const void *>(&shim_activate),      reinterpret_cast<const void *>(&shim_deactivate),
    reinterpret_cast<const void *>(&shim_enter_standby), reinterpret_cast<const void *>(&shim_get_component),
    reinterpret_cast<const void *>(&shim_debug_request), reinterpret_cast<const void *>(&shim_get_pose),
};

}  // namespace

vr::ITrackedDeviceServerDriver *TrackerDevice::steamvr_interface() {
    shim_.vtable = kShimVtable;
    return reinterpret_cast<vr::ITrackedDeviceServerDriver *>(&shim_);
}
#else
vr::ITrackedDeviceServerDriver *TrackerDevice::steamvr_interface() { return this; }
#endif

void TrackerDevice::update(const vr::DriverPose_t &pose) {
    pose_ = pose;
    if (object_id_ != vr::k_unTrackedDeviceIndexInvalid)
        vr::VRServerDriverHost()->TrackedDevicePoseUpdated(object_id_, pose_, sizeof(pose_));
}

}  // namespace picotrackers
