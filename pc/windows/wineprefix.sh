# Подключается (source) из build-windows.sh и fetch_innosetup.sh, текущая папка — pc/.
# Свой Wine-префикс сборки (build/wineprefix), отвязанный от домашней папки: winemenubuilder выключен
# (иначе Wine создаёт ярлыки в меню и на рабочем столе Linux и перехватывает ассоциации файлов вроде
# text/plain), «Рабочий стол», «Документы» и т.п. внутри префикса — обычные папки, а не ссылки на ~.
export WINEPREFIX="$PWD/build/wineprefix" WINEDEBUG=-all WINEDLLOVERRIDES="mscoree,mshtml=;winemenubuilder.exe=d"
if [[ ! -f $WINEPREFIX/system.reg ]]; then
    # Родительскую папку Wine сам не создаёт (на свежем клоне build/ ещё нет).
    mkdir -p "$(dirname "$WINEPREFIX")"
    wineboot -i >/dev/null 2>&1 || true
    wineserver -w
    [[ -f $WINEPREFIX/system.reg ]] || { echo "Wine-префикс $WINEPREFIX не создался" >&2; exit 1; }
fi
# В контейнере (CI) USER не задан — Wine тогда берёт имя из passwd.
for link in "$WINEPREFIX/drive_c/users/${USER:-$(id -un)}"/*; do
    if [[ -L $link ]]; then rm "$link" && mkdir "$link"; fi
done
