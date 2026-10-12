#!/usr/bin/env bash
# Flatpak программы для Linux → build/PICOtrackers.flatpak (собрать программу: build.sh). Ставится так:
#   flatpak install --user PICOtrackers.flatpak   (runtime GNOME 51 скачается с Flathub)
# Нужны flatpak-builder и установленные org.gnome.Platform//51 и org.gnome.Sdk//51 — как в образе
# ghcr.io/flathub-infra/flatpak-github-actions:gnome-51 (CI). Без них: build-flatpak.sh --docker — сборка в этом
# образе (нужен Docker; контейнеру — --privileged, иначе bubblewrap flatpak-builder не работает).
set -euo pipefail
cd "$(dirname "$0")/.."

IMAGE=ghcr.io/flathub-infra/flatpak-github-actions:gnome-51
APPID=dev.picotrackers.Control

[[ -f build/picotrackers ]] || { echo "нет build/picotrackers — сначала build.sh" >&2; exit 1; }
if [[ ${1:-} == --docker ]]; then
    docker run --rm --privileged -v "$(pwd):/src" -w /src "$IMAGE" sh -c \
        "flatpak/build-flatpak.sh && chown -R $(id -u):$(id -g) build/flatpak build/PICOtrackers.flatpak"
    exit
fi

rm -rf build/flatpak/app
flatpak-builder --force-clean --disable-rofiles-fuse --state-dir=build/flatpak/state --repo=build/flatpak/repo \
    build/flatpak/app flatpak/$APPID.yml
flatpak build-bundle --runtime-repo=https://dl.flathub.org/repo/flathub.flatpakrepo \
    build/flatpak/repo build/PICOtrackers.flatpak $APPID
echo "готово: $(pwd)/build/PICOtrackers.flatpak ($(du -h build/PICOtrackers.flatpak | cut -f1))"
