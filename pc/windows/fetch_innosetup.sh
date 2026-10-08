#!/usr/bin/env bash
# Inno Setup (компилятор установщика для Windows) → third_party/innosetup, ставится под Wine.
# Версия и sha256 закреплены (sha256 — из релиза jrsoftware/issrc на GitHub).
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=7.1.0
FILE=innosetup-$VERSION-x64.exe
SHA256=0362a383ed217d4c4239b5933866dd96d3eb2102737da92f80f6057a4b40df2f
URL=https://github.com/jrsoftware/issrc/releases/download/is-${VERSION//./_}/$FILE

DEST=third_party/innosetup
DL=third_party/download
[[ -f $DEST/ISCC.exe ]] && exit 0
mkdir -p "$DL"
if [[ ! -f $DL/$FILE ]]; then
    echo "== скачиваю $FILE"
    curl -fL --retry 3 -o "$DL/$FILE.part" "$URL"
    mv "$DL/$FILE.part" "$DL/$FILE"
fi
echo "$SHA256  $DL/$FILE" | sha256sum -c --quiet

echo "== ставлю Inno Setup под Wine"
source windows/wineprefix.sh
wine "$DL/$FILE" /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /CURRENTUSER /NOICONS /TASKS= \
    "/DIR=$(winepath -w "$(pwd)")\\third_party\\innosetup" 2>/dev/null
wineserver -w
[[ -f $DEST/ISCC.exe ]] || { echo "Inno Setup не установился" >&2; exit 1; }
