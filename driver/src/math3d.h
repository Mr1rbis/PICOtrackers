// Минимальная 3D-математика для драйвера: векторы, кватернионы, решение небольших систем.
#pragma once

#include <cmath>
#include <cstddef>
#include <utility>

namespace picotrackers {

constexpr double kPi = 3.14159265358979323846;

struct Vec3 {
    double x = 0, y = 0, z = 0;

    Vec3 operator+(const Vec3 &o) const { return {x + o.x, y + o.y, z + o.z}; }
    Vec3 operator-(const Vec3 &o) const { return {x - o.x, y - o.y, z - o.z}; }
    Vec3 operator*(double s) const { return {x * s, y * s, z * s}; }
    double length() const { return std::sqrt(x * x + y * y + z * z); }
};

inline Vec3 cross(const Vec3 &a, const Vec3 &b) {
    return {a.y * b.z - a.z * b.y, a.z * b.x - a.x * b.z, a.x * b.y - a.y * b.x};
}

struct Quat {
    double w = 1, x = 0, y = 0, z = 0;

    Quat operator*(const Quat &o) const {
        return {w * o.w - x * o.x - y * o.y - z * o.z, w * o.x + x * o.w + y * o.z - z * o.y,
                w * o.y - x * o.z + y * o.w + z * o.x, w * o.z + x * o.y - y * o.x + z * o.w};
    }

    Quat conjugate() const { return {w, -x, -y, -z}; }

    double dot(const Quat &o) const { return w * o.w + x * o.x + y * o.y + z * o.z; }

    Quat normalized() const {
        double n = std::sqrt(w * w + x * x + y * y + z * z);
        if (n < 1e-12) return {};
        return {w / n, x / n, y / n, z / n};
    }

    Vec3 rotate(const Vec3 &v) const {
        Vec3 u{x, y, z};
        Vec3 t = cross(u, v) * 2.0;
        return v + t * w + cross(u, t);
    }

    // Поворот вокруг вертикальной оси Y (y вверх и в SteamVR, и в STAGE шлема).
    static Quat from_yaw(double yaw) { return {std::cos(yaw / 2), 0, std::sin(yaw / 2), 0}; }

    // Угол ближайшего к этому кватерниону поворота вокруг Y.
    double yaw() const {
        double m00 = 1 - 2 * (y * y + z * z);
        double m02 = 2 * (x * z + w * y);
        double m20 = 2 * (x * z - w * y);
        double m22 = 1 - 2 * (x * x + y * y);
        return std::atan2(m02 - m20, m00 + m22);
    }

    // Полный угол поворота, радианы.
    double angle() const { return 2 * std::acos(std::fmin(1.0, std::fabs(normalized().w))); }
};

// Решает A·x = b для симметричной N×N методом Гаусса с выбором ведущего элемента.
// false — система вырождена.
template <size_t N>
bool solve_linear(double (&a)[N][N], double (&b)[N], double (&x)[N]) {
    for (size_t col = 0; col < N; col++) {
        size_t pivot = col;
        for (size_t row = col + 1; row < N; row++)
            if (std::fabs(a[row][col]) > std::fabs(a[pivot][col])) pivot = row;
        if (std::fabs(a[pivot][col]) < 1e-12) return false;
        if (pivot != col) {
            for (size_t k = 0; k < N; k++) std::swap(a[col][k], a[pivot][k]);
            std::swap(b[col], b[pivot]);
        }
        for (size_t row = col + 1; row < N; row++) {
            double f = a[row][col] / a[col][col];
            for (size_t k = col; k < N; k++) a[row][k] -= f * a[col][k];
            b[row] -= f * b[col];
        }
    }
    for (size_t i = N; i-- > 0;) {
        double s = b[i];
        for (size_t k = i + 1; k < N; k++) s -= a[i][k] * x[k];
        x[i] = s / a[i][i];
    }
    return true;
}

}  // namespace picotrackers
