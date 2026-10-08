// Лог драйвера → ~/.local/share/Steam/logs/vrserver.txt (строки «picotrackers: …»).
#pragma once

#include <openvr_driver.h>

#include <cstdarg>
#include <cstdio>

namespace picotrackers {

inline void log(const char *fmt, ...) __attribute__((format(printf, 1, 2)));
inline void log(const char *fmt, ...) {
    char buf[1024];
    va_list args;
    va_start(args, fmt);
    vsnprintf(buf, sizeof(buf), fmt, args);
    va_end(args);
    if (vr::VRDriverLog()) vr::VRDriverLog()->Log(buf);
}

}  // namespace picotrackers
