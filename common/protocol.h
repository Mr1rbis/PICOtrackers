// UDP-протокол между шлемом и драйвером. Little-endian (шлем — arm64, ПК — x86_64), без выравнивания.
//  * Packet        шлем → ПК:7788  — снимок скелета XR_BD_body_tracking, ~100 раз/с;
//  * ControlPacket ПК → шлем:7789  — драйвер раз в секунду просит данные (шлем сам начинает трекинг
//                                    на адрес отправителя и останавливается, если просьбы прекратились);
//  * программа на ПК ↔ драйвер по 127.0.0.1: команды «PTA1 tracking=1 rate=100 trackers=a,b» → :7788,
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

enum ControlCommand : uint16_t {
    kCommandHello = 1,  // нужны данные на data_port отправителя
    kCommandBye = 2,    // SteamVR закрывается — можно остановить трекинг сразу
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
constexpr uint32_t kFlagTrackersReady = 1u << 1;     // трекеры подключены и откалиброваны

// JointPose::flags — младшие биты XrSpaceLocationFlags.
constexpr uint32_t kJointOrientationValid = 0x1;
constexpr uint32_t kJointPositionValid = 0x2;

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
    uint32_t joint_count;  // 24, или 16 если рантайм дал скелет без рук
    JointPose joints[kJointCount];
};
struct ControlPacket {
    char magic[4];
    uint16_t data_port;
    uint16_t command;  // ControlCommand
    uint16_t rate_hz;  // сколько снимков в секунду слать (0 — не менять)
    uint16_t reserved;
};
#pragma pack(pop)

static_assert(sizeof(JointPose) == 32, "protocol layout");
static_assert(sizeof(Packet) == 24 + kJointCount * 32, "protocol layout");
static_assert(sizeof(ControlPacket) == 12, "protocol layout");

}  // namespace picotrackers
