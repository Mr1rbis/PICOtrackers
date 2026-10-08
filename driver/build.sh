#!/usr/bin/env bash
# Сборка драйвера SteamVR под Linux и Windows (mingw-w64) в одну папку build/picotrackers:
# bin/linux64/driver_picotrackers.so и bin/win64/driver_picotrackers.dll.
# Linux-версия собирается clang'ом с sysroot Debian 11 (fetch_sysroot.py) — как Steam Runtime «sniper», в котором
# SteamVR грузит драйверы: требует glibc не новее 2.31 и работает на любом дистрибутиве, где работает SteamVR.
#   ./build.sh            — обе платформы
#   ./build.sh linux      — только Linux (быстро, для разработки)
set -euo pipefail
cd "$(dirname "$0")"

[[ -f third_party/openvr/headers/openvr_driver.h ]] || ./fetch_deps.sh
[[ -f third_party/sysroot-bullseye/usr/lib/x86_64-linux-gnu/libc.so ]] || python3 -I fetch_sysroot.py

root=$(pwd)/build/picotrackers
build() {  # $1 — папка сборки, дальше — аргументы cmake
    local dir=$1
    shift
    cmake -S . -B "$dir" -G Ninja -DCMAKE_BUILD_TYPE=Release -DDRIVER_ROOT="$root" "$@" >/dev/null
    cmake --build "$dir"
}

build build/cmake-sniper -DCMAKE_TOOLCHAIN_FILE="$(pwd)/cmake/linux-sniper.cmake"
so=$root/bin/linux64/driver_picotrackers.so
newest=$(objdump -T "$so" | grep -o 'GLIBC_[0-9.]*' | sort -uV | tail -1)
if [[ $(printf '%s\n' "$newest" GLIBC_2.31 | sort -V | tail -1) != GLIBC_2.31 ]] || objdump -T "$so" | grep -q GLIBCXX; then
    echo "драйвер требует $newest или чужой libstdc++ — не загрузится в Steam Runtime sniper" >&2
    exit 1
fi
if [[ ${1:-all} != linux ]]; then
    build build/cmake-windows -DCMAKE_TOOLCHAIN_FILE="$(pwd)/cmake/mingw-w64.cmake"
fi
echo "готово: $root"
