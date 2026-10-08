// Калибровка: связь пространства STAGE шлема (в нём приходят суставы) с raw-пространством
// SteamVR (в нём драйвер Steam Link отдаёт позу HMD).
//
// Модель: поза HMD в SteamVR = T · head · D, где
//   T    — искомое преобразование STAGE → SteamVR: поворот вокруг вертикали + сдвиг;
//   head — поза сустава head из скелета PICO;
//   D    — неизвестное постоянное смещение HMD относительно сустава head (поворот Q + сдвиг d).
// T и D оцениваются по накопленным парам (HMD, head) методом наименьших квадратов,
// пока голова почти неподвижна (чтобы не мешала разница задержек двух источников).
#pragma once

#include <deque>
#include <string>

#include "math3d.h"

namespace picotrackers {

struct RigidTransform {
    Quat rotation;
    Vec3 translation;

    Vec3 apply(const Vec3 &p) const { return rotation.rotate(p) + translation; }
};

class Calibration {
public:
    struct Sample {
        Vec3 hmd_position;
        Quat hmd_rotation;
        Vec3 head_position;
        Quat head_rotation;
    };

    // Добавляет пару поз (не чаще раза в 100 мс). true — преобразование пересчитано.
    bool add(const Sample &sample, double now);

    bool valid() const { return valid_; }
    double error() const { return residual_; }
    size_t sample_count() const { return samples_.size(); }
    const RigidTransform &transform() const { return transform_; }
    std::string describe() const;

private:
    void solve();
    double residual(const Sample &s) const;

    std::deque<Sample> samples_;
    double last_sample_time_ = -1e9;
    double last_solve_time_ = -1e9;
    int outliers_ = 0;

    bool valid_ = false;
    RigidTransform transform_;
    Quat head_to_hmd_rotation_;  // Q
    Vec3 head_to_hmd_offset_;    // d, в системе сустава head
    double residual_ = 0;        // медианная ошибка по выборке, м
};

}  // namespace picotrackers
