#!/usr/bin/env bash
# Сборка приложения PICO Trackers для шлема без Gradle: NDK/CMake + javac + d8 + aapt2 + apksigner.
#   ./build.sh           — собрать build/PicoTrackers.apk
#   ./build.sh install   — собрать и поставить на шлем через adb
set -euo pipefail
cd "$(dirname "$0")"

SDK=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-/opt/android-sdk}}
BT=$(ls -d "$SDK"/build-tools/* | sort -V | tail -1)
NDK=${ANDROID_NDK_HOME:-$(ls -d "$SDK"/ndk/* | sort -V | tail -1)}
ANDROID_JAR=
for p in $(ls -d "$SDK"/platforms/android-* | sort -Vr); do
    if [[ -f $p/android.jar ]]; then ANDROID_JAR=$p/android.jar; break; fi
done
[[ -n $ANDROID_JAR ]] || { echo "не найден android.jar в $SDK/platforms" >&2; exit 1; }

OUT=build
KEYSTORE=debug.keystore
MIN_SDK=29  # PICO OS 5 = Android 10

[[ -f third_party/pico_openxr/libs/android.arm64-v8a/libopenxr_loader.so && -f third_party/openxr/include/openxr/openxr.h ]] || ./fetch_deps.sh

rm -rf "$OUT/classes" "$OUT/apk"
mkdir -p "$OUT/classes" "$OUT/apk/lib/arm64-v8a"

echo "== native"
cmake -S cpp -B "$OUT/cmake" -G Ninja \
    -DCMAKE_TOOLCHAIN_FILE="$NDK/build/cmake/android.toolchain.cmake" \
    -DANDROID_ABI=arm64-v8a -DANDROID_PLATFORM=android-$MIN_SDK -DANDROID_STL=c++_static \
    -DCMAKE_BUILD_TYPE=Release >/dev/null
cmake --build "$OUT/cmake"
cp "$OUT/cmake/libpicotrackers.so" third_party/pico_openxr/libs/android.arm64-v8a/libopenxr_loader.so \
    "$OUT/apk/lib/arm64-v8a/"

echo "== java"
javac --release 11 -Xlint:-options -cp "$ANDROID_JAR" -d "$OUT/classes" $(find src -name '*.java')
"$BT/d8" --release --min-api $MIN_SDK --lib "$ANDROID_JAR" --output "$OUT/apk" \
    $(find "$OUT/classes" -name '*.class')

echo "== apk"
"$BT/aapt2" link -o "$OUT/unaligned.apk" --manifest AndroidManifest.xml -I "$ANDROID_JAR" \
    --min-sdk-version $MIN_SDK --target-sdk-version $MIN_SDK
# Фиксированное время файлов и -X: одинаковые исходники дают побайтово одинаковый APK
# (программа на ПК сравнивает APK на шлеме со своим по sha256).
find "$OUT/apk" -exec touch -h -d '2020-01-01 00:00:00 UTC' {} +
(cd "$OUT/apk" && zip -qrX ../unaligned.apk classes.dex lib)
"$BT/zipalign" -f -p 4 "$OUT/unaligned.apk" "$OUT/aligned.apk"

# Свой ключ рядом с проектом: подпись не меняется между сборками, adb install -r работает.
if [[ ! -f $KEYSTORE ]]; then
    keytool -genkeypair -keystore "$KEYSTORE" -storepass android -keypass android \
        -alias probe -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=PicoTrackers" >/dev/null
fi
"$BT/apksigner" sign --ks "$KEYSTORE" --ks-pass pass:android --key-pass pass:android \
    --ks-key-alias probe --out "$OUT/PicoTrackers.apk" "$OUT/aligned.apk"
echo "готово: $(pwd)/$OUT/PicoTrackers.apk"

if [[ ${1:-} == install ]]; then
    adb install -r "$OUT/PicoTrackers.apk"
fi
