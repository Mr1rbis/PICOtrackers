#!/usr/bin/env bash
# Сборка программы для Windows из Linux: Go + GTK 4/libadwaita (пакеты MSYS2 ucrt64, windows/fetch_gtk.py),
# компилятор mingw-w64. Результат — папка build/windows/PICOtrackers (picotrackers.exe, DLL GTK, данные),
# архив build/PICOtrackers-windows.zip и установщик build/PICOtrackers-setup-<версия>.exe (Inno Setup под Wine,
# windows/installer.iss). Первая сборка долгая: биндинги GTK для Windows (~15 мин).
set -euo pipefail
cd "$(dirname "$0")"

S=$(pwd)/third_party/msys2
U=$S/ucrt64
[[ -f $U/lib/pkgconfig/gtk4.pc ]] || python3 windows/fetch_gtk.py
windows/fetch_innosetup.sh
./prepare_assets.sh
source windows/wineprefix.sh

VERSION=$(sed -nE 's/^\s*appVersion\s*=\s*"([^"]+)".*/\1/p' core.go)
echo "== значок и версия exe ($VERSION)"
# Значок (icon.svg) в нескольких размерах, картинки установщика под разные DPI (размеры — из справки
# Inno Setup), ресурсы exe → rsrc_windows_amd64.syso (go build подхватывает сам, только для Windows).
RES=build/windows-res
rm -rf "$RES"
mkdir -p "$RES"
for size in 16 20 24 32 40 48 64 256; do rsvg-convert -w $size -h $size icon.svg -o "$RES/icon-$size.png"; done
magick "$RES"/icon-{16,20,24,32,40,48,64,256}.png "$RES/picotrackers.ico"
for size in 202x386 269x515 336x643 403x772 498x953; do
    rsvg-convert -w ${size%x*} -h ${size#*x} windows/wizard.svg -o "$RES/wizard-${size%x*}.png"
done
for size in 58 77 97 116 143; do rsvg-convert -w $size -h $size icon.svg -o "$RES/small-$size.png"; done
IFS=. read -r V1 V2 V3 <<< "$VERSION"
cat > "$RES/picotrackers.rc" <<RC
#include <winver.h>
1 ICON "picotrackers.ico"
1 VERSIONINFO
FILEVERSION ${V1:-0},${V2:-0},${V3:-0},0
PRODUCTVERSION ${V1:-0},${V2:-0},${V3:-0},0
FILEOS VOS_NT_WINDOWS32
FILETYPE VFT_APP
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "040904B0"
    BEGIN
      VALUE "CompanyName", "PICOtrackers"
      VALUE "FileDescription", "PICO Trackers"
      VALUE "FileVersion", "$VERSION"
      VALUE "ProductName", "PICO Trackers"
      VALUE "ProductVersion", "$VERSION"
      VALUE "OriginalFilename", "picotrackers.exe"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x409, 1200
  END
END
RC
x86_64-w64-mingw32-windres --include-dir="$RES" -O coff -i "$RES/picotrackers.rc" -o rsrc_windows_amd64.syso

OUT=build/windows/PICOtrackers
rm -rf "$OUT"
mkdir -p "$OUT"
echo "== picotrackers.exe"
env CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
    PKG_CONFIG_LIBDIR="$U/lib/pkgconfig:$U/share/pkgconfig" PKG_CONFIG_SYSROOT_DIR="$S" PKG_CONFIG_PATH= \
    go build -ldflags "-s -w -H windowsgui" -o "$OUT/picotrackers.exe" .

echo "== GTK runtime"
# Загрузчики картинок gdk-pixbuf (SVG-значки через librsvg) — вместе с их зависимостями.
LOADERS=lib/gdk-pixbuf-2.0/2.10.0
mkdir -p "$OUT/$LOADERS/loaders"
cp "$U/$LOADERS"/loaders/*.dll "$OUT/$LOADERS/loaders/"
python3 windows/collect_dlls.py "$U/bin" "$OUT" "$OUT/picotrackers.exe" "$OUT/$LOADERS"/loaders/*.dll
# Список загрузчиков генерирует родная утилита (под Wine), пути делаем относительными —
# gdk-pixbuf ищет их относительно папки программы.
cp "$U/bin/gdk-pixbuf-query-loaders.exe" "$OUT/"
python3 windows/collect_dlls.py "$U/bin" "$OUT" "$OUT/gdk-pixbuf-query-loaders.exe" >/dev/null
wine "$OUT/gdk-pixbuf-query-loaders.exe" 2>/dev/null | tr -d '\r' |
    sed -E 's#^"[^"]*[\\/]+loaders[\\/]+([^"]+)"#"lib\\\\gdk-pixbuf-2.0\\\\2.10.0\\\\loaders\\\\\1"#' \
    > "$OUT/$LOADERS/loaders.cache"
rm "$OUT/gdk-pixbuf-query-loaders.exe"
echo "загрузчиков картинок: $(grep -c '^"lib' "$OUT/$LOADERS/loaders.cache")"

# Схемы GSettings, значки (Adwaita + hicolor), настройки шрифтов.
mkdir -p "$OUT/share/glib-2.0/schemas" "$OUT/share/icons" "$OUT/etc"
cp "$U"/share/glib-2.0/schemas/*.xml "$OUT/share/glib-2.0/schemas/"
glib-compile-schemas "$OUT/share/glib-2.0/schemas"
cp -r "$U/share/icons/Adwaita" "$U/share/icons/hicolor" "$OUT/share/icons/"
cp -r "$U/etc/fonts" "$OUT/etc/"

rm -f build/PICOtrackers-windows.zip
(cd build/windows && zip -qr ../PICOtrackers-windows.zip PICOtrackers)
echo "архив: $(pwd)/build/PICOtrackers-windows.zip ($(du -sh build/PICOtrackers-windows.zip | cut -f1))"

echo "== установщик"
SETUP=build/PICOtrackers-setup-$VERSION.exe
rm -f "$SETUP"
# Списки картинок Inno приводит к нижнему регистру, и Wine не находит папку PROJ — картинки отдаём через диск R:.
ln -sfn "$(pwd)/$RES" "$WINEPREFIX/dosdevices/r:"
wine third_party/innosetup/ISCC.exe /Q "/DAppVersion=$VERSION" "/DSourceDir=$(winepath -w "$OUT")" \
    "/DResDir=R:" "/DOutputDir=$(winepath -w build)" "$(winepath -w windows/installer.iss)"
[[ -f $SETUP ]] || { echo "установщик не собрался" >&2; exit 1; }
echo "установщик: $(pwd)/$SETUP ($(du -sh "$SETUP" | cut -f1))"
