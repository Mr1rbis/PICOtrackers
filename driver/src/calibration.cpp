#include "calibration.h"

#include <algorithm>
#include <cstdio>
#include <vector>

namespace picotrackers {

namespace {

constexpr double kSampleInterval = 0.1;    // с
constexpr size_t kMaxSamples = 300;        // окно ~30 с
constexpr size_t kMinSamplesForOffset = 30;  // раньше смещение head→HMD не оцениваем (Q = 1, d ≈ 0)
constexpr double kSolveInterval = 0.5;     // с
constexpr double kOutlierDistance = 0.25;  // м
constexpr int kOutliersToReset = 10;       // подряд: пространства сдвинулись (recenter, перезапуск)
constexpr double kOffsetRidge = 1.0;       // притяжение d к нулю, пока поворотов головы мало
constexpr double kNoOffsetRidge = 1e6;

double component(const Vec3 &v, int i) { return i == 0 ? v.x : i == 1 ? v.y : v.z; }

}  // namespace

bool Calibration::add(const Sample &sample, double now) {
    if (now - last_sample_time_ < kSampleInterval) return false;
    last_sample_time_ = now;

    if (valid_ && residual(sample) > kOutlierDistance) {
        if (++outliers_ < kOutliersToReset) return false;
        samples_.clear();
        valid_ = false;
    }
    outliers_ = 0;

    samples_.push_back(sample);
    if (samples_.size() > kMaxSamples) samples_.pop_front();
    if (valid_ && now - last_solve_time_ < kSolveInterval) return false;
    last_solve_time_ = now;
    solve();
    return true;
}

void Calibration::solve() {
    const bool estimate_offset = samples_.size() >= kMinSamplesForOffset;

    // Повороты: R_hmd = R_yaw · R_head · Q. Чередуем оценку yaw при известном Q и Q при известном yaw.
    Quat q = estimate_offset ? head_to_hmd_rotation_ : Quat{};
    double yaw = 0;
    for (int iter = 0; iter < (estimate_offset ? 5 : 1); iter++) {
        double s = 0, c = 0;
        for (const auto &x : samples_) {
            double a = (x.hmd_rotation * q.conjugate() * x.head_rotation.conjugate()).yaw();
            s += std::sin(a);
            c += std::cos(a);
        }
        yaw = std::atan2(s, c);
        if (!estimate_offset) break;

        Quat inv_yaw = Quat::from_yaw(yaw).conjugate();
        Quat sum{0, 0, 0, 0}, first;
        for (size_t i = 0; i < samples_.size(); i++) {
            const auto &x = samples_[i];
            Quat qi = x.head_rotation.conjugate() * inv_yaw * x.hmd_rotation;
            if (i == 0) first = qi;
            if (qi.dot(first) < 0) qi = {-qi.w, -qi.x, -qi.y, -qi.z};
            sum = {sum.w + qi.w, sum.x + qi.x, sum.y + qi.y, sum.z + qi.z};
        }
        q = sum.normalized();
    }
    Quat ry = Quat::from_yaw(yaw);

    // Сдвиги: hmd = R_yaw·head_pos + (R_yaw·R_head)·d + t — линейно по x = [t, d].
    double a[6][6]{}, b[6]{}, x[6]{};
    for (const auto &s : samples_) {
        Quat u = ry * s.head_rotation;
        Vec3 columns[3] = {u.rotate({1, 0, 0}), u.rotate({0, 1, 0}), u.rotate({0, 0, 1})};
        Vec3 r = s.hmd_position - ry.rotate(s.head_position);
        for (int i = 0; i < 3; i++) {
            a[i][i] += 1;
            a[3 + i][3 + i] += 1;  // MᵀM = I
            b[i] += component(r, i);
            for (int j = 0; j < 3; j++) {
                double m = component(columns[j], i);  // M[i][j]
                a[i][3 + j] += m;
                a[3 + j][i] += m;
                b[3 + j] += m * component(r, i);
            }
        }
    }
    for (int j = 0; j < 3; j++) a[3 + j][3 + j] += estimate_offset ? kOffsetRidge : kNoOffsetRidge;
    if (!solve_linear(a, b, x)) return;

    transform_ = {ry, {x[0], x[1], x[2]}};
    head_to_hmd_rotation_ = q;
    head_to_hmd_offset_ = {x[3], x[4], x[5]};
    valid_ = true;

    std::vector<double> errors;
    errors.reserve(samples_.size());
    for (const auto &s : samples_) errors.push_back(residual(s));
    std::nth_element(errors.begin(), errors.begin() + errors.size() / 2, errors.end());
    residual_ = errors[errors.size() / 2];
}

double Calibration::residual(const Sample &s) const {
    Vec3 hmd = transform_.apply(s.head_position) + (transform_.rotation * s.head_rotation).rotate(head_to_hmd_offset_);
    return (s.hmd_position - hmd).length();
}

std::string Calibration::describe() const {
    if (!valid_) return "нет калибровки";
    char buf[256];
    std::snprintf(buf, sizeof(buf),
                  "yaw=%.1f° t=(%.3f %.3f %.3f) head→hmd d=(%.3f %.3f %.3f) Q=%.1f° ошибка=%.3f м выборка=%zu",
                  transform_.rotation.yaw() * 180 / kPi, transform_.translation.x, transform_.translation.y,
                  transform_.translation.z, head_to_hmd_offset_.x, head_to_hmd_offset_.y, head_to_hmd_offset_.z,
                  head_to_hmd_rotation_.angle() * 180 / kPi, residual_, samples_.size());
    return buf;
}

}  // namespace picotrackers
