#!/usr/bin/env bash
# Собирает драйвер SteamVR (Linux + Windows) и приложение для шлема и кладёт их в assets/ для go:embed.
# PREBUILT_ASSETS=1 — ничего не собирать, assets/ уже заполнена: так делает CI (.github/workflows/build.yml),
# где драйвер и APK собираются отдельными задачами и одни и те же файлы вшиваются в программы для Linux и Windows.
set -euo pipefail
cd "$(dirname "$0")"

if [[ ${PREBUILT_ASSETS:-} == 1 ]]; then
    for f in PicoTrackers.apk driver/picotrackers/driver.vrdrivermanifest \
        driver/picotrackers/bin/linux64/driver_picotrackers.so driver/picotrackers/bin/win64/driver_picotrackers.dll; do
        [[ -f assets/$f ]] || { echo "PREBUILT_ASSETS=1, но нет assets/$f" >&2; exit 1; }
    done
else
    ../driver/build.sh
    ../headset/build.sh
    rm -rf assets
    mkdir -p assets/driver
    cp -r ../driver/build/picotrackers assets/driver/
    cp ../headset/build/PicoTrackers.apk assets/
fi
echo "assets: $(du -sh assets | cut -f1)"
