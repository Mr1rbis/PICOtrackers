#!/usr/bin/env bash
# Сборка программы для ПК под Linux → build/picotrackers.
# Сначала собирает драйвер SteamVR (Linux + Windows) и приложение для шлема и вшивает их в программу
# (папка assets/, go:embed). Первая сборка долгая: биндинги GTK 4 на cgo (~10 мин).
# НЕ добавлять -trimpath: он меняет ключи кэша Go, и биндинги компилируются заново.
set -euo pipefail
cd "$(dirname "$0")"

./prepare_assets.sh
go build -ldflags "-s -w" -o build/picotrackers .
echo "готово: $(pwd)/build/picotrackers"
