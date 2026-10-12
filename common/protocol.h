// UDP-протокол между шлемом и драйвером. Little-endian (шлем — arm64, ПК — x86_64), без выравнивания.
//  * Packet        шлем → ПК:7788  — снимок скелета XR_BD_body_tracking, ~100 раз/с; пока трекеры PICO
//                                    не подключены — только пакет состояния без суставов (joint_count = 0),
//                                    5 раз/с: шлем на связи, но поз нет;
//  * StatusPacket  шлем → ПК:7788  — состояние трекеров PICO: подключены ли, откалиброваны ли, заряд;
//                                    раз в секунду, пока идёт трекинг (драйверы до 1.7 его пропускают);
//  * ControlPacket ПК → шлем:7789  — драйвер раз в секунду просит данные (шлем сам начинает трекинг
//                                    на адрес отправителя и останавливается, если просьбы прекратились),
//                                    по просьбе программы — открыть калибровку трекеров PICO на шлеме;
//  * программа на ПК ↔ драйвер по 127.0.0.1: команды «PTA1 tracking=1 rate=100 trackers=a,b calibrate=N» → :7788,
//    состояние драйвера и позы суставов (JSON) → :7790 (программа pc/, Go).
#pragma once

#include <cstdint>

namespace picotrackers {

constexpr uint16_t kDefaultPort = 7788;
constexpr uint16_t kControlPort = 7789;
constexpr uint16_t kAppPort = 7790;
constexpr char kMagic[4] = {'P', 'T', 'B', '1'};
constexpr char kControlMagic[4] = {'P', 'T', 'C', '1'};
constexpr char kAppMagic[4] = {'P', 'T', 'A', '1'};
constexpr char kStatusMagic[4] = {'P', 'T', 'S', '1'};

enum ControlCommand : uint16_t {
    kCommandHello = 1,  // нужны данные на data_port отправителя
    kCommandBye = 2,    // SteamVR закрывается — можно остановить трекинг сразу
    kCommandCalibrate = 3,  // открыть на шлеме калибровку трекеров PICO; ControlPacket::request — номер просьбы
};

// Совпадает с XrBodyJointBD.
enum Joint : uint8_t {
    kPelvis = 0,
    kLeftHip = 1,
    kRightHip = 2,
    kSpine1 = 3,
    kLeftKnee = 4,
    kRightKnee = 5,
    kSpine2 = 6,
    kLeftAnkle = 7,
    kRightAnkle = 8,
    kSpine3 = 9,
    kLeftFoot = 10,
    kRightFoot = 11,
    kNeck = 12,
    kLeftCollar = 13,
    kRightCollar = 14,
    kHead = 15,
    kLeftShoulder = 16,
    kRightShoulder = 17,
    kLeftElbow = 18,
    kRightElbow = 19,
    kLeftWrist = 20,
    kRightWrist = 21,
    kLeftHand = 22,
    kRightHand = 23,
    kJointCount = 24,
};

// Packet::flags
constexpr uint32_t kFlagAllJointsTracked = 1u << 0;  // allJointPosesTracked
constexpr uint32_t kFlagTrackersReady = 1u << 1;     // трекеры подключены и откалиброваны (иначе позы не шлются)

// JointPose::flags — младшие биты XrSpaceLocationFlags.
constexpr uint32_t kJointOrientationValid = 0x1;
constexpr uint32_t kJointPositionValid = 0x2;

// StatusPacket: состояние body tracking от рантайма (xrGetBodyTrackingStateBD) и трекеры PICO
// (GetSwiftConnectState / GetSwiftTrackerBattery / GetSwiftCalibState из системной libtrackingclient.pxr.so). -1 — неизвестно.
enum BodyTrackingStatus : int8_t {
    kBodyStatusUnknown = -1,
    kBodyStatusInvalid = 0,  // данных нет (трекеры не подключены или не откалиброваны — см. error)
    kBodyStatusValid = 1,
    kBodyStatusLimited = 2,  // данные есть, но неточные
};
enum BodyTrackingError : int8_t {  // BodyTrackingMessage из PICO SDK
    kBodyErrorUnknown = -1,
    kBodyErrorInner = 0,
    kBodyErrorNotCalibrated = 1,     // трекеры не откалиброваны
    kBodyErrorNotEnoughTrackers = 2, // подключено слишком мало трекеров
    kBodyErrorTrackerState = 3,
    kBodyErrorInvisible = 4,
    kBodyErrorDataError = 5,
    kBodyErrorUserChanged = 6,
    kBodyErrorPoseError = 7,
};
constexpr int kMaxPicoTrackers = 12;

// Место трекера PICO на теле — в старших 4 битах PicoTracker::id (номер трекера — в младших; так драйверы и программы
// 1.7, где места не было, пакет понимают по-прежнему). Числа — как в getSwiftTrackerInfo у PICO (1 и 2 сверены с логом
// системного трекинга «Swifter_StatusInfo … Role:7/8», 3 — трекер на поясе, не проверено).
enum TrackerPlace : uint8_t {
    kPlaceUnknown = 0,
    kPlaceLeftLeg = 1,
    kPlaceRightLeg = 2,
    kPlaceWaist = 3,
};
constexpr uint8_t kTrackerNumberMask = 0x0f;
inline uint8_t tracker_number(uint8_t id) { return id & kTrackerNumberMask; }
inline TrackerPlace tracker_place(uint8_t id) { return static_cast<TrackerPlace>(id >> 4); }
constexpr uint8_t kTrackerCountUnknown = 0xff;
constexpr int16_t kCalibrationNotRequested = -32768;

#pragma pack(push, 1)
struct JointPose {
    float position[3];     // метры, пространство STAGE шлема: y вверх, пол ≈ 0
    float orientation[4];  // x y z w
    uint32_t flags;
};

struct Packet {
    char magic[4];
    uint32_t seq;
    int64_t xr_time_ns;
    uint32_t flags;
    uint32_t joint_count;  // 24, или 16 если рантайм дал скелет без рук; 0 — пакет состояния без поз
    JointPose joints[kJointCount];
};
struct ControlPacket {
    char magic[4];
    uint16_t data_port;
    uint16_t command;  // ControlCommand
    uint16_t rate_hz;  // сколько снимков в секунду слать (0 — не менять)
    uint16_t request;  // kCommandCalibrate: номер просьбы (шлем выполняет каждую один раз), иначе 0
};

struct PicoTracker {
    uint8_t id;      // номер трекера у PICO (младшие 4 бита) и TrackerPlace (старшие)
    int8_t battery;  // заряд, % (0–100); -1 — неизвестен
};

struct StatusPacket {
    char magic[4];
    int8_t body_status;     // BodyTrackingStatus
    int8_t body_error;      // BodyTrackingError
    int8_t calibrated;      // трекеры откалиброваны (libtrackingclient): 1 да, 0 нет, -1 неизвестно
    uint8_t tracker_count;  // подключено трекеров PICO; kTrackerCountUnknown — неизвестно
    PicoTracker trackers[kMaxPicoTrackers];
    uint16_t calibration_request;  // последняя выполненная просьба откалибровать (ControlPacket::request)
    int16_t calibration_result;    // ≥ 0 — калибровка открыта, < 0 — ошибка; kCalibrationNotRequested — просьб не было
};
#pragma pack(pop)

static_assert(sizeof(JointPose) == 32, "protocol layout");
static_assert(sizeof(Packet) == 24 + kJointCount * 32, "protocol layout");
static_assert(sizeof(ControlPacket) == 12, "protocol layout");
static_assert(sizeof(StatusPacket) == 36, "protocol layout");

}  // namespace picotrackers
