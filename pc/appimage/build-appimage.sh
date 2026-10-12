#!/usr/bin/env bash
# AppImage программы для Linux → build/PICOtrackers-x86_64.AppImage (собрать программу: build.sh).
# В образ кладётся build/picotrackers и все её библиотеки из системы сборки — GTK 4, libadwaita и сама glibc
# с загрузчиком ld-linux (AppRun запускает программу через него): так AppImage идёт и там, где GTK старее
# GNOME 50 (Ubuntu 24.04, Debian 13, SteamOS), и где glibc старее, чем у Arch. Ещё — значки Adwaita, схемы
# GSettings GTK, раскладки клавиатуры, ярлык и значок. Собирать там же, где программу (Arch, как в CI).
# Нужны: mksquashfs (squashfs-tools), glib-compile-schemas, rsvg-convert, curl.
set -euo pipefail
cd "$(dirname "$0")/.."

# Runtime AppImage (type2, статический: FUSE 2 и 3, сжатие zstd), закреплён по sha256.
RUNTIME_VERSION=20251108
RUNTIME_SHA256=2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d
RUNTIME=third_party/appimage/runtime-x86_64-$RUNTIME_VERSION
APPID=dev.picotrackers.Control
APPDIR=build/appimage/AppDir
OUT=build/PICOtrackers-x86_64.AppImage

[[ -f build/picotrackers ]] || { echo "нет build/picotrackers — сначала build.sh" >&2; exit 1; }
if [[ ! -f $RUNTIME ]]; then
    mkdir -p "$(dirname "$RUNTIME")"
    curl -fsSL --retry 3 -o "$RUNTIME.tmp" \
        "https://github.com/AppImage/type2-runtime/releases/download/$RUNTIME_VERSION/runtime-x86_64"
    echo "$RUNTIME_SHA256  $RUNTIME.tmp" | sha256sum -c --quiet
    mv "$RUNTIME.tmp" "$RUNTIME"
fi

rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/lib/gio/modules" "$APPDIR/usr/share/glib-2.0/schemas" \
    "$APPDIR/usr/share/icons/hicolor/scalable/apps" "$APPDIR/usr/share/icons/hicolor/256x256/apps" \
    "$APPDIR/usr/share/applications" "$APPDIR/usr/share/metainfo"
cp build/picotrackers "$APPDIR/usr/bin/"
cp appimage/AppRun "$APPDIR/AppRun"

# Библиотеки: всё, что находит ldd (и загрузчик), — кроме vDSO ядра.
ldd build/picotrackers | while read -r name arrow path _; do
    [[ $arrow == "=>" && $path == /* ]] || path=$name  # «/lib64/ld-linux-x86-64.so.2 (0x…)»
    [[ $path == /* ]] || continue                      # linux-vdso.so.1
    cp -L "$path" "$APPDIR/usr/lib/$(basename "$name")"
done
[[ -f $APPDIR/usr/lib/ld-linux-x86-64.so.2 && -f $APPDIR/usr/lib/libgtk-4.so.1 ]] ||
    { echo "ldd не нашёл загрузчик или GTK" >&2; exit 1; }
strip --strip-unneeded "$APPDIR"/usr/lib/*.so* 2>/dev/null || true

# Схемы GSettings GTK (файловый диалог, инспектор): без них GTK падает, если до них дойдёт дело.
cp /usr/share/glib-2.0/schemas/org.gtk.gtk4.*.gschema.xml "$APPDIR/usr/share/glib-2.0/schemas/"
glib-compile-schemas "$APPDIR/usr/share/glib-2.0/schemas"

# Раскладки клавиатуры для libxkbcommon (AppRun: XKB_CONFIG_ROOT).
cp -rL /usr/share/xkeyboard-config-2 "$APPDIR/usr/share/"

# Значки Adwaita (без курсоров — их рисует система) и значок программы.
mkdir -p "$APPDIR/usr/share/icons/Adwaita"
for item in index.theme scalable symbolic 16x16; do
    [[ -e /usr/share/icons/Adwaita/$item ]] && cp -r "/usr/share/icons/Adwaita/$item" "$APPDIR/usr/share/icons/Adwaita/"
done
cp /usr/share/icons/hicolor/index.theme "$APPDIR/usr/share/icons/hicolor/"
cp icon.svg "$APPDIR/usr/share/icons/hicolor/scalable/apps/$APPID.svg"
rsvg-convert -w 256 -h 256 icon.svg -o "$APPDIR/usr/share/icons/hicolor/256x256/apps/$APPID.png"
cp "$APPDIR/usr/share/icons/hicolor/256x256/apps/$APPID.png" "$APPDIR/$APPID.png"
ln -s "$APPID.png" "$APPDIR/.DirIcon"
cp linux/$APPID.desktop "$APPDIR/$APPID.desktop"
cp linux/$APPID.desktop "$APPDIR/usr/share/applications/"
cp linux/$APPID.metainfo.xml "$APPDIR/usr/share/metainfo/"

# Образ: runtime + squashfs (zstd), без меток времени владельцев — одинаковые файлы дают одинаковый образ.
rm -f build/appimage/image.squashfs
mksquashfs "$APPDIR" build/appimage/image.squashfs -root-owned -noappend -comp zstd -Xcompression-level 19 \
    -mkfs-time 0 -all-time 0 -quiet >/dev/null
cat "$RUNTIME" build/appimage/image.squashfs > "$OUT.tmp"
chmod +x "$OUT.tmp"
mv "$OUT.tmp" "$OUT"
echo "готово: $(pwd)/$OUT ($(du -h "$OUT" | cut -f1))"
