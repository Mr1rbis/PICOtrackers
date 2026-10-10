# CLAUDE.md — PICOtrackers

Всё, что известно о проекте на 2026-10-10. Общение с пользователем — на русском.
Репозиторий: git, GitHub **`Mr1rbis/PICOtrackers`** (ветка main), релизы собирает CI по тегу `v<appVersion>`.

## Цель

Пользователь играет в PCVR на **PICO 4 Pro** через **Steam Link** (SteamVR на **Linux**, Arch) и хочет,
чтобы его **PICO Motion Trackers на ногах** появились в SteamVR как трекеры (full body, VRChat и т.п.).

Готовых решений нет (проверено 2026-10):
- **Steam Link для PICO** (2.0.22 от 2026-07-29) трекеры не поддерживает; в 2.0.22 добавили только
  face tracking для 4 Pro. Valve: «PICO Trackers … not supported at the moment, but we are hoping…».
- **PICO Connect** трекеры в SteamVR передаёт, но только под Windows.
- **ALVR** на Linux не работает с SteamVR ≥ 2.16.7 (Steam Runtime ломает драйвер) — vronlinux.org.
- **WiVRn** трекеры PICO поддерживает, но заменяет SteamVR (игры через xrizer/OpenComposite), а
  пользователь хочет Steam Link + SteamVR.

## Архитектура

```
PICO 4 Pro: сервис PICO Trackers (стартует при загрузке шлема / открытии приложения / по adb)
   ▲  ждёт на UDP 7789 просьбу «нужны данные» (PTC1) ─────────────────────────────┐
   │  пока просьбы идут (раз в 1 с) — headless-сессия OpenXR, XR_BD_body_tracking  │
   └─► UDP 7788, ~100 Гц: 24 сустава (PTB1) ──► ПК: драйвер SteamVR picotrackers ──┘ (broadcast + известный IP)
                                                     │ трекеры SteamVR, калибровка head ↔ HMD
                                                     │ 127.0.0.1: состояние JSON → :7790, команды PTA1 ← :7788
                                                     ▼
                                    программа PICO Trackers (Go + GTK4/libadwaita, pc/)
                                    установка, adb, статус, включение/выключение трекеров на лету
```
Видео и шлем/контроллеры идут через Steam Link как обычно; наш канал — только трекинг тела.
Без программы драйвер сам просит данные и держит последний набор трекеров (настройка SteamVR).

## Статус

1. ✅ Пробник → фоновый доступ подтверждён (2026-10-07): со Steam Link на переднем плане все суставы ~100 раз/с.
2. ✅ Первая реализация (2026-10-07 21:22): 3 трекера, калибровка ~7 мм (yaw −3.2°, t ≈ 3 см,
   head→hmd d ≈ (0.01, 0.05, −0.10) м, Q ≈ 3°). У процесса, запущенного при спящем шлеме, STAGE отличался
   (yaw −13.4°, t_y −0.33 м) — калибровка сама сбросилась и пересчиталась.
3. ✅ Версия 2 (2026-10-07 ~22:00), по просьбам пользователя:
   - **автозапуск**: драйвер рассылает PTC1 hello, сервис на шлеме сам включает/выключает трекинг —
     проверено (поддельный hello с ПК → старт за миллисекунды, стоп через 5 с; реальный драйвер после
     перезапуска SteamVR → шлем начал трекинг сам);
   - **все суставы**: по умолчанию 10 трекеров с ролями SteamVR, ещё 14 — по желанию; добавлены в SteamVR;
   - **программа с интерфейсом на Go** (gotk4 + libadwaita) вместо Python; запускается, драйвер видит
     «программа на ПК подключилась».
   - Не проверено пользователем: переключение трекеров в окне (живое добавление/«удаление» в SteamVR),
     внешний вид окна, VRChat.
4. ✅ Версия 3 (2026-10-07 ~22:20): регулировка частоты (10–250 снимков/с, проверена цепочка программа →
   драйвер → шлем: 60/с применилось за ~70 мс), вкладки «Трекеры / 3D-вид / Настройки», схема «человечек»
   (скриншот проверен), 3D-вид скелета (первый вариант был зеркальным — исправлено, повторно не смотрели).

5. ✅ Версия 4 (2026-10-07 ~22:45): программа и драйвер **под Windows** (кросс-сборка из Linux), драйвер и APK
   **вшиты в программу**, установка — **мастером с кнопками «Далее»** (драйвер → приложение на шлем по кабелю →
   готово). Проверено: Linux-сборка, мастер открывается сам (скриншот), тест установки драйвера во временном
   HOME, Windows-драйвер (DLL: только системные импорты), Windows-программа запускается под Wine (окно GTK
   рисуется; шрифты в Wine/Xvfb кривые — артефакт Wine). На реальной Windows не проверялось.
   Найдено и исправлено: сокет драйвера без SOCK_CLOEXEC наследовали дочерние процессы vrserver.
6. ✅ Версия 5 (программа 1.3, 2026-10-07 ~23:30): **установщик для Windows** (Inno Setup 7.1, собирается под Wine)
   — `pc/build/PICOtrackers-setup-1.3.exe`: для текущего пользователя без прав администратора, галочки «драйвер
   SteamVR» / значок на рабочем столе / автозапуск, деинсталлятор снимает драйвер и удаляет данные. Значок
   программы (`pc/windows/icon.svg`) и версия вшиты в exe. Переносной zip остался. Проверено под Wine:
   страницы мастера (скриншоты), тихая установка (драйвер скопирован и зарегистрирован, чужой драйвер в
   openvrpaths сохранён), окно ошибки «SteamVR не найден» → «Повторить» → драйвер встал, удаление (регистрация
   снята, данные удалены). На реальной Windows не проверялось.
7. ✅ Переносимость Linux (2026-10-07 ~23:30): драйвер требовал glibc 2.43 (собран компилятором Arch) — теперь
   собирается clang'ом с sysroot Debian 11 (как Steam Runtime sniper), требует glibc ≤ 2.17; проверено загрузкой
   через ld.so/libc 2.31 из SteamLinuxRuntime_sniper (старый падал «GLIBC_2.43 not found», новый создаёт провайдер).
   Установка драйвера теперь подменяет файлы переименованием (раньше перезапись .so на месте при запущенном SteamVR
   могла уронить vrserver). **Программа на ПК (Linux) требует GNOME 50**: gotk4 v0.4.1 / gotk4-adwaita при init
   вызывают *_get_type() для всех типов — GTK 4.22, libadwaita 1.9, GLib/GIO 2.88, Pango 1.57 (проверено по
   `nm -D` + gir). На Ubuntu 24.04/Debian 13/SteamOS не стартует. Вариант на будущее — Flatpak (GNOME 50 runtime).
8. ✅ README и CI (2026-10-08): README переписан целиком для GitHub — русский + английский, дисклеймер «100% ИИ,
   руками не трогалось, возможен нейрослоп», проверено только Linux + PICO 4 Pro (Windows — следующим), что нужно
   для запуска (пакеты по дистрибутивам, Windows), сборка, CI, «покопаться руками» в фурри-стиле (просьба
   пользователя). CI `.github/workflows/build.yml` (см. раздел CI) прогнан локально в контейнерах из чистого
   git-клона: все задачи собрались, `go test` прошёл, в программы для Linux и Windows вшиты одни и те же APK и
   драйвер. Программа: переустановка APK при чужой подписи. Потом проект попал на GitHub (`Mr1rbis/PICOtrackers`),
   CI отработал там: релиз **v1.3** (picotrackers-linux-x86_64, setup.exe, zip, APK, SHA256SUMS.txt) опубликован.
9. ✅ Автообновление (программа 1.4, 2026-10-10, по просьбе пользователя): при **каждом запуске** — последний релиз
   на GitHub; новее → окно «Доступна версия …». Скачивается и ставится **только** при закрытом SteamVR и шлеме,
   подключённом **USB-кабелем** с разрешённой отладкой (adb `device`); иначе кнопка неактивна. «Позже» закрывает окно,
   при следующем запуске оно откроется снова. Проверено на Linux во вложенном KWin с подставным релизом и поддельным
   adb: блокировка по условиям, загрузка + SHA256, установка APK, подмена файла, перезапуск `--wait-pid --updated`,
   отказ при чужой контрольной сумме, папка без прав записи, нет сети → мастер. Установщик в режиме `/UPDATE` —
   под Wine (драйвер переустановлен, программа запущена с `--updated`). PowerShell-ожидание на Windows и обновление
   с реальным шлемом не проверены. Версия 1.3 обновляться не умеет — с неё только вручную.
10. ✅ Версия 1.5 (2026-10-10, по просьбе пользователя; 1.4 уже выпущена с тегом v1.4):
    - **Драйвер хранит настройки программы**: кроме `trackers` и `rate` теперь и `tracking`; программа отключилась —
      драйвер работает с её последними настройками (раньше сбрасывал трекинг на «вкл»), после перезапуска SteamVR тоже.
    - **Без подключённых трекеров PICO данные не идут**: шлем не шлёт скелет (только пакет состояния без суставов,
      5 раз/с), драйвер не добавляет трекеры в SteamVR и уже добавленным ставит `deviceIsConnected=false`.
    - **Сервис на шлеме**: программа видит, что он выключен (`dumpsys`, раз в 5 с + строка лога), карточка
      «Сервис на шлеме выключен», кнопки «Включить сервис» (строка шлема, карточка, меню).
    Проверено: драйвер — поддельным vrserver (scratchpad `drvtest/host.cpp` + `scenario.py`, 19 проверок, в
    `unshare -rn`), программа — во вложенном KWin с поддельным adb и поддельным состоянием драйвера (скриншоты).
    На реальном шлеме и в SteamVR не проверено.
11. ✅ Версия 1.6 (2026-10-11, по просьбе пользователя): **одна кнопка «Включить/Выключить сервис»** (строка шлема +
    один пункт меню, надпись по состоянию по adb, без SteamVR) и **выбор «выключен» запоминается**: в программе
    (`service_off` в config.json — при подключении к шлему сервис не запускается) и на шлеме (SharedPreferences
    `service/disabled` — BootReceiver и открытие приложения его не запускают). Проверено во вложенном KWin с
    поддельным adb, изображающим приложение 1.6 (6 шагов: автозапуск → выкл → перезапуск без автозапуска → вкл →
    автозапуск → выключен на самом шлеме). Java-часть на реальном шлеме не проверена.

## Железо и окружение

**Шлем**
- Модель `A8110`, systemName `PICO 4 Pro HMD`, PICO OS **5.13.7**, Android **10 (SDK 29)**.
- adb serial `PA8A10MGJ4120202D`, Wi-Fi `192.168.0.11` (DHCP, может смениться). Подключён по USB.
- OpenXR-рантайм: пакет `com.pico.xr.openxr_runtime`, `/system/priv-app/XRRuntime` (versionName 2.1.12),
  runtimeName **`Pico(XRT) by Pico et al '99d719953' 3.0.1`** — форк **Monado** (функции `oxr_*`).
  Внутри APK: `libpxrruntime.so`, `libpxrruntime_service.so`, `libpxrplugin.so`, `libpxrinputmanager.so` …
- Системные либы трекеров: `/system/lib64/libtrackerclient.pxr.so`, `libAlgSwiftTracking.so`,
  `libAlgSwiftBodyPose*.so` (Swift = Motion Trackers). Приложение трекеров — `com.pvr.swift` («Motion Tracker» 2.0.4).
- Сопряжены 2 трекера (`mBindTrackerCount 2`). Свойства: `persist.pxr.tracking.swiftVersion=2`,
  `ro.pxr.support.swiftversion=3`, `debug.pxr.tracking.swift_body_tracking_mode=0`.
- Установлено: PICO Trackers `dev.picotrackers.headset` v1.2 (в репозитории уже 1.6; пробник `dev.picotrackers.probe` удалён), Steam Link `com.valvesoftware.steamlinkvr` 2.0.22 (активности `com.valvesoftware.steamlink.SteamLink`
  — меню, `.VRLink` — стрим), ALVR `alvr.client.stable` 20.14.1 и `alvr.client.dev` (nightly 2026.06.06),
  Virtual Desktop 1.34.22, VRChat, Connect (`com.picovr.picostreamassistant`).
- Шлем засыпает, когда снят (датчик приближения): `dumpsys power` → `mWakefulness=Asleep`, `Display Power: state=OFF`.
  Во сне трекинга нет — тестировать только в надетом шлеме.

**ПК**
- Arch Linux, zsh. IP `192.168.0.18` (`enp6s0`), есть tailscale/docker-интерфейсы.
- SteamVR 2.17.10: `~/.local/share/Steam/steamapps/common/SteamVR` (buildid 25330290). vrserver работает в
  контейнере pressure-vessel (SteamLinuxRuntime_sniper). Драйвер Steam Link — `drivers/vrlink`. Внешние драйверы:
  Standable Full Body Estimation, ALVR nightly, наш picotrackers.
- Android: **`/opt/android-sdk`** — build-tools 37.0.0, platforms android-32/37, NDK 29.0.14206865, platform-tools.
  (`~/Android/Sdk` — только cmdline-tools и NDK 29.0.13846066.) JDK 26 (`javac --release 11`), python3, zip, keytool.
  Gradle не установлен — и не нужен. **На 2026-10-10 `cmake` и `ninja` в системе нет** (`pacman -Q` их не знает) —
  либо `sudo pacman -S cmake ninja`, либо без изменений в системе: `python3 -m venv <scratchpad>/buildtools &&
  <…>/bin/pip install cmake ninja` (cmake 4.4, ninja 1.13 — оба build.sh с ними собирают) и этот `bin` первым в PATH.
  Старые папки CMake (`driver/build/cmake-*`, `headset/build/cmake`) помнили `/usr/bin/ninja` — их пришлось удалить.

**Особенности окружения Claude Code**
- В окружении задан `HTTP(S)_PROXY`. **`git clone` с GitHub через него не работает** («Proxy CONNECT aborted»),
  `curl` работает → качать архивами: `https://codeload.github.com/<org>/<repo>/tar.gz/HEAD`.
- Сетевые команды (curl, приём UDP, сборка с загрузкой зависимостей) требуют запуска вне песочницы.
- zsh: `==` в начале слова раскрывается (писать `'=='`/`---`), глобы вроде `--include=*.cpp` и фильтры
  logcat `'Tag:*'` надо брать в кавычки.
- Foreground `sleep` заблокирован — ждать через `timeout N команда`.
- Wine 11 (`build/wineprefix` в pc/): gdk-pixbuf-query-loaders.exe и Inno Setup (ISCC.exe). Под Wine программа
  целиком не работает: Go вызывает WSAIoctl(SIO_UDP_CONNRESET) на UDP-сокетах, Wine его не поддерживает (10045).
  - Префикс — только через `pc/windows/wineprefix.sh` (source): winemenubuilder выключен, папки Desktop/Documents
    в префиксе — не ссылки на ~. Без этого Wine клал ярлыки на настоящий рабочий стол и в меню KDE, а установщик
    Inno Setup зарегистрировал себя обработчиком text/plain и перезаписал wine-extension-*.desktop пользователя
    (`~/.wine`) — всё это было вычищено/восстановлено 2026-10-07.
  - Окна под Wine смотреть на Xvfb :77 (`import -window root`, `xdotool key Return`), не на экране пользователя.
    **На 2026-10-10 Xvfb в системе нет** — вместо него вложенный невидимый KWin (см. «Проверка окон» ниже).
  - Inno под Wine: в списках картинок (`WizardImageFile=a.png,b.png`) путь приводится к нижнему регистру и Wine не
    находит `PROJ` → картинки отдаются через диск `R:` (симлинк в dosdevices).
  - Pascal Script Inno 7: `Trim`, `Copy`, `AnsiUppercase` идут через ANSI и портят кириллицу на нерусской Windows
    (`Format`, `LoadStringsFromFile`, `Utf8Decode`, склейка `+` — нормально). Сообщения готовит программа.
- **Проверка окон программы без экрана пользователя** (2026-10-10): `dbus-run-session -- kwin_wayland --virtual
  --width 1100 --height 1000 --socket <имя> --xwayland --exit-with-session <скрипт, пишущий env и спящий>`; программу
  запускать с `GDK_BACKEND=x11`, отдельным `HOME`, `PATH` с поддельным `adb` (скрипт, отвечающий на devices / pm path /
  sha256sum / install), снимки — `import -window <id из xdotool search>`. Мышь и клавиатура xdotool в этот Xwayland
  **не доходят** — кнопки нажимать через AT-SPI: в том же D-Bus запустить `/usr/lib/at-spi-bus-launcher
  --launch-immediately` и `/usr/lib/at-spi2-registryd`, программу без `NO_AT_BRIDGE`, Python `gi` Atspi →
  `get_action_iface().do_action(0)`. GTK 4.22 иногда падает (SIGABRT, `gtk_at_spi_root_to_ref` assertion), если
  AT-SPI-клиент обходит дерево сразу после закрытия диалога — артефакт теста, не программы.
- Если запущена программа пользователя (занят UDP 7790), тестовую копию запускать в своём сетевом пространстве:
  `unshare --user --map-current-user --net …` (с `--map-current-user` uid прежний — D-Bus принимает EXTERNAL-авторизацию;
  `ip link set lo up` там запрещён, но привязка к 127.0.0.1 работает). Всплывающее меню GTK под Xwayland — отдельное
  окно X: искать `xdotool search --onlyvisible --name ''` по размеру и снимать `import -window <id>`.
- **SteamVR на этом ПК** (2026-10-11): vrcompositor падает (SIGSEGV, вызов по нулевому указателю в главном потоке,
  стек vrcompositor/vrclient.so/libX11/SDL3) почти при каждом закрытии SteamVR с 2026-10-01 — и без нашего драйвера
  (он в компоновщик не грузится). Ошибка **303** «Основной компонент SteamVR работает со сбоями» = vrmonitor не получил
  интерфейс компоновщика: тот стартовал 5 с, а шлем Steam Link был уже подключён и VR_Init прошёл сразу (гонка).
  Дампы — /tmp/dumps/crash_*.dmp (разбор: scratchpad `minidump.py` — модули, RIP, адреса со стека). Пользователь
  тогда выключил драйвер в «Управлении надстройками» (`driver_picotrackers.enable=false` в steamvr.vrsettings).
- `pkill -f <шаблон>` / `pgrep -f` совпадают и с командной строкой своей же оболочки (zsh -c '…шаблон…') — убивают её
  (код 144). Писать шаблон как `[u]pdtest/…`. Поддельный «SteamVR» для проверок (копия sleep с именем vrserver)
  убивать по PID, **не** `pkill -x vrserver` — так можно закрыть настоящий SteamVR пользователя.
- **Проверка драйвера без SteamVR**: поддельный vrserver — C++ (IVRDriverContext с IVRSettings в файле,
  IVRServerDriverHost с неподвижным HMD, IVRProperties, IVRDriverLog; VRDriverManager/VRResources — любой ненулевой
  указатель, драйвер их не вызывает), dlopen собранного .so, RunFrame ~100 раз/с. Шлем и программу изображает Python
  (PTB1/PTA1 на 127.0.0.1:7788, слушает :7789 и :7790). Запускать в `unshare -rn sh -c 'ip link set lo up; …'` — иначе
  просьбы драйвера уйдут broadcast'ом настоящему шлему.
- VS Code: gopls анализирует ~685 тыс. строк gotk4 и перечитывал проект после каждой сборки (тысячи файлов в
  pc/build, pc/third_party) — грузил 8 ядер. В `.vscode/settings.json` эти папки исключены (directoryFilters, watcherExclude).
- `adb install` пишет «Unknown command: install-incremental» и сам переходит на streamed install — это норма.
- `am stopservice` возвращает код 255 даже при успехе.
- В XML-комментариях нельзя `--` (aapt2: «not well-formed»).

## Находки по OpenXR на PICO 4 Pro (главное)

**Расширения**, которые рантайм объявляет (важные): `XR_BD_body_tracking` v1, `XR_BD_controller_interaction`,
`XR_MND_headless` v2, `XR_EXTX_overlay` v5, `XR_KHR_convert_timespec_time`, `XR_KHR_android_create_instance`,
`XR_KHR_loader_init(_android)`, `XR_EXT_local_floor`, `XR_EXT_hand_tracking`, `XR_EXT_eye_gaze_interaction`,
`XR_FB_passthrough`, `XR_FB_display_refresh_rate`, `XR_KHR_locate_spaces`, `XR_KHR_maintenance1` и др.
**Не объявляет**: `XR_BD_motion_tracking`, `XR_PICO_motion_tracking`, `XR_PICO_configuration` (вообще ни одного `XR_PICO_*`).

1. **`XR_BD_motion_tracking` (подход ALVR, «сырые» позы трекеров) на 4 Pro недоступен.** Функции есть в
   `libpxrruntime.so` (`oxr_xrGetMotionTrackerLocationsBD` и т.д., «Requires XR_BD_motion_tracking extension
   enabled»), но расширение включает ключ конфиг-сервиса **`sys_tracking_enable_standalone_mode_tracker`**
   (`libpxrruntime_service.so`, лог «from config service, oxr_get_motion_tracking_enable»), на 4 Pro он выключен
   (видимо, фича Pico 4 Ultra). В читаемых файлах `/system/etc` и т.п. ключа нет.
   Метаданные манифеста (`pvr.sdk.version=OpenXR`, `pxr.sdk.version_code=5900`, `pvr.app.type=vr`) список
   расширений **не меняют** — проверено, из манифеста убраны.
2. **Рабочий источник — `XR_BD_body_tracking`**: 24 сустава, ноги считаются по Motion Trackers.
   `XrSystemBodyTrackingPropertiesBD.supportsBodyTracking=1`, `xrCreateBodyTrackerBD(FULL_BODY_JOINTS)` — успех.
3. **Headless-сессия** (`XR_MND_headless` + `xrCreateSession` без graphics binding) создаётся из фонового
   сервиса, но **навсегда остаётся в `IDLE`**: PICO выдаёт READY только приложению с фокусом activity
   (`activityFocusableChanged`, `oxr_session_manage_state` в логах рантайма).
4. **`xrLocateSpace` в headless-сессии до `xrBeginSession` роняет рантайм** (SIGSEGV, null deref, fault addr 0xc0,
   стек в `XRRuntime.apk` ← `xrLocateSpace`). Позу HMD в IDLE не получить.
5. **`xrLocateBodyJointsBD` в `IDLE` работает** — в т.ч. когда на переднем плане Steam Link со стримом.
   Флаги суставов `0xf`, `allJointPosesTracked=1`, ~100 обновлений/с. Это и есть основа проекта.
   Для калибровки вместо позы HMD использовать сустав **`head`** (XR_BODY_JOINT_HEAD_BD).
6. **`xrGetBodyTrackingStateBD`**: PICO реализует сигнатуру ALVR `(XrInstance, int32_t* status, int32_t* error)`,
   а в заголовках Khronos 1.1.63 она другая `(XrSession, XrBodyTrackingStateBD*)`. Вызываем по-ALVR с буферами
   `int32_t[16]` на случай несовпадения. `status=1` — трекеры подключены и откалиброваны, `0` — трекеры выключены.
   Есть также `xrStartBodyTrackingCalibAppBD(instance, package, flag=1 MOTION_TRACKER_2)` — запускает калибровку.
7. Пространство `STAGE` доступно (используем как базовое): y вверх, пол ≈ 0. Пример сидя: head y≈1.05,
   pelvis ≈0.50, chest ≈0.76, knees ≈0.36, ankles ≈0.0–0.06, feet ≈ −0.08 (стопы чуть ниже пола).
8. Запасной путь (сейчас не нужен): overlay-сессия `XR_EXTX_overlay` — у PICO есть
   `XR_TYPE_SESSION_CREATE_INFO_OVERLAY_EXTX` с флагами `controllerable`, `need_package_change`,
   свойство `persist.pxr.sdk.overlay.fps`. Требует graphics binding (EGL) и цикл кадров.
9. Loader: ALVR для Pico 4 / 4 Pro / Neo3 использует **старый loader из PICO OpenXR SDK 2.2.0**
   (`PICO_OpenXR_SDK_220.zip`, «_pico_old») и apiVersion 1.0.34; мы делаем так же, apiVersion `XR_API_VERSION_1_0`.
   Рантайм при этом поднимается с «OpenXR API version 1.1». Заголовки PICO SDK 2.2.0 — OpenXR 1.0.27,
   в них нет `XR_BD_body_tracking`, поэтому заголовки берём от Khronos (OpenXR-SDK release-1.1.63).
10. Индексы `XrBodyJointBD`: 0 PELVIS, 1 L_HIP, 2 R_HIP, 3 SPINE1, 4 L_KNEE, 5 R_KNEE, 6 SPINE2, 7 L_ANKLE,
    8 R_ANKLE, 9 SPINE3, 10 L_FOOT, 11 R_FOOT, 12 NECK, 13 L_COLLAR, 14 R_COLLAR, 15 HEAD, 16 L_SHOULDER,
    17 R_SHOULDER, 18 L_ELBOW, 19 R_ELBOW, 20 L_WRIST, 21 R_WRIST, 22 L_HAND, 23 R_HAND.
    `BODY_WITHOUT_ARM` = первые 16 суставов.

**Полезные теги logcat шлема**: `PicoTrackers` (наше приложение), `PvrTrackingService-MCUTracker` (`mTrackerConnectCount`,
`mBindTrackerCount`, `mAlgRuningBodyTracking`), `APxrRuntime`, `xrt_session`, `PxrLoader`, буфер `-b crash`.

**Референсы** (качать архивами через codeload, см. выше):
- ALVR `alvr/client_openxr/src/extra_extensions/motion_tracking_bd.rs`, `body_tracking_bd.rs`,
  `interaction.rs` (маппинг в трекеры), `lib.rs` (выбор loader), `client_openxr/Cargo.toml` (манифест),
  PR alvr-org/ALVR#2712 (режимы трекеров; `get_motion_tracker_locations` работает независимо от режима).
- WiVRn `client/xr/pico_body_tracker.cpp` — тот же `XR_BD_body_tracking` (у них в FOCUSED-сессии).

## Структура репозитория

```
CLAUDE.md, README.md (для людей на GitHub: русский + английский, дисклеймер «100% ИИ»), .gitignore
.github/workflows/build.yml   CI (GitHub Actions): driver ‖ headset → linux ‖ windows → release (на тег v<appVersion>)
common/protocol.h              UDP-протокол: Packet (PTB1, шлем→драйвер), ControlPacket (PTC1, драйвер→шлем),
                               порты 7788/7789/7790, флаги, индексы суставов (= XrBodyJointBD)
headset/                       приложение для шлема, пакет dev.picotrackers.headset v1.6, «PICO Trackers»
  AndroidManifest.xml          2D-приложение (без pvr.app.type=vr); TrackingService exported с permission
                               DUMP (запускать может только adb shell и само приложение); BootReceiver
  build.sh / fetch_deps.sh     сборка без Gradle → build/PicoTrackers.apk; loader PICO + заголовки Khronos (sha256)
  debug.keystore               ключ подписи (alias probe, пароль android; gitignore). НЕ удалять (иначе install -r
                               не пройдёт — программа тогда переустанавливает приложение). В CI — секрет HEADSET_KEYSTORE_BASE64
  cpp/tracking.cpp             loader, instance, headless-сессия, трекер тела, цикл 100 Гц, UDP, JNI (start/stop/running/status)
  src/…/TrackingService.java   постоянный foreground-сервис: слушатель UDP 7789, автостарт/стоп трекинга, WifiLock+MulticastLock
  src/…/BootReceiver.java      запуск сервиса после загрузки шлема
  src/…/MainActivity.java      окно: статус, «Передавать вручную» (отладка), «Авто» (включить), «Выключить» (запоминается)
driver/                        драйвер SteamVR «picotrackers» (C++17, Linux и Windows)
  build.sh / fetch_deps.sh     CMake+Ninja → build/picotrackers/; openvr_driver.h v2.15.6 (sha256)
  fetch_sysroot.py             sysroot Debian 11 (libc6, libc6-dev, linux-libc-dev, libgcc-s1, libgcc-10-dev,
                               libstdc++-10-dev; индекс archive.debian.org закреплён sha256) → third_party/sysroot-bullseye
  cmake/linux-sniper.cmake     тулчейн Linux-драйвера (clang + lld + этот sysroot); cmake/mingw-w64.cmake — Windows
  package/                     driver.vrdrivermanifest (alwaysActivate), default.vrsettings, input profile
  src/driver.cpp               провайдер: 24 трекера, включение на лету, команды программы, JSON-состояние
  src/receiver.*               поток сети: снимки со шлема, hello/bye шлему, команды/состояние программы
  src/calibration.*            STAGE шлема → raw-пространство SteamVR
  src/tracker_device.*         устройство SteamVR (свойства, иконки {htc}, модель {htc}vr_tracker_vive_1_0)
pc/                            программа на ПК (Go, модуль picotrackers; gotk4 v0.4.1 + gotk4-adwaita), Linux и Windows
  update.go                    обновление: последний релиз GitHub, сравнение версий, загрузка + SHA256SUMS, условия,
                               Release.Install (APK на шлем, затем installUpdate из platform_*.go); update_test.go
  updatedialog.go              окно обновления (на wizardStep из wizard.go), строка «Обновление» в настройках
  build.sh                     prepare_assets.sh + go build → build/picotrackers. Первая сборка ~9 мин (cgo-биндинги
                               GTK); НЕ добавлять -trimpath — он меняет ключи кэша и всё компилируется заново
  prepare_assets.sh            driver/build.sh (Linux+Windows) + headset/build.sh → assets/ (go:embed, gitignore);
                               PREBUILT_ASSETS=1 — не собирать, assets/ уже заполнена (CI), только проверка состава
  build-windows.sh             кросс-сборка picotrackers.exe (mingw-w64 + GTK из MSYS2) + упаковка DLL/данных →
                               build/windows/PICOtrackers/, build/PICOtrackers-windows.zip (~34 МБ) и установщик
                               build/PICOtrackers-setup-<версия>.exe. Значок/версия exe: windres → rsrc_windows_amd64.syso
                               (генерируется, gitignore), картинки установщика → build/windows-res
  windows/installer.iss        сценарий Inno Setup (русский, WizardStyle modern dynamic windows11, AppMutex)
  windows/fetch_innosetup.sh   Inno Setup 7.1.0 (sha256 из релиза GitHub) → ставится под Wine в third_party/innosetup
  icon.svg                     значок программы (вшит: окно/«О программе» через кэш + IconTheme.AddSearchPath,
                               значок в меню Linux; Windows: .ico в exe и картинки установщика)
  windows/wizard.svg           картинка слева в установщике (рендер rsvg-convert, .ico — magick)
  windows/fetch_gtk.py         пакеты MSYS2 ucrt64 (gtk4, libadwaita, adwaita-icon-theme, librsvg,
                               gobject-introspection + зависимости, sha256) → third_party/msys2/ucrt64 (sysroot)
  windows/collect_dlls.py      рекурсивно копирует нужные DLL (objdump -p) из sysroot
  assets.go                    embed: assets/driver/picotrackers (bin/linux64 + bin/win64), assets/PicoTrackers.apk
  platform_linux.go / platform_windows.go  пути, SteamVR (pgrep / PowerShell), автозапуск (.desktop / реестр Run),
                               скрытие консоли у дочерних процессов на Windows
  wizard.go                    мастер установки (Adw.Dialog + NavigationView); на Linux первый шаг «Программа»
  install_test.go              тест установки драйвера во временном HOME
  main.go                      вход: окно; подкоманды setup / stop / install-driver / uninstall (последние две —
                               для установщика: --message-file=F, коды 0 / 1 ошибка / 3 мешает запущенный SteamVR);
                               --updated (тост «обновлена»), --wait-pid=N (дождаться выхода прошлой версии)
  core.go                      adb и шлем, установка APK, сборка/регистрация драйвера, LogcatFollower, SteamVR
  controller.go                фон: UDP с драйвером, установка, состояние (Snapshot для окна)
  gui.go                       окно libadwaita, вкладки; GTK только из главного потока (glib.IdleAdd / TimeoutAdd)
  body.go                      схема «человечек» (Cairo + Gtk.Fixed поверх DrawingArea)
  view3d.go                    3D-вид скелета (Cairo, своя перспективная проекция)
tools/udp_listen.py            отладка (Python): показывает приходящие суставы (порт общий с драйвером)
```

### Протокол (`common/protocol.h`)
- **PTB1** шлем → ПК:7788, 792 байта: `magic, seq u32, xr_time_ns i64, flags u32 (1 = allJointPosesTracked,
  2 = трекеры готовы), joint_count u32, 24 × {pos[3], rot[4] xyzw, flags u32}`; позиции в STAGE шлема.
  Без флага 2 (трекеры PICO не подключены) шлем шлёт **пакет состояния**: тот же размер, `joint_count = 0`, суставы
  нулевые, 5 раз/с. Драйвер 1.3 такой пакет переживает (позы просто невалидны).
- **PTC1** драйвер → шлем:7789, 12 байт: `magic, data_port u16, command u16 (1 hello, 2 bye), rate_hz u16,
  reserved u16` (шлем читает rate, если длина ≥ 10; при смене частоты hello уходит сразу). Hello раз в 1 с
  на broadcast всех IPv4-интерфейсов + на известный IP шлема (`driver_picotrackers.headset_ip` в настройках
  SteamVR, драйвер запоминает адрес отправителя данных). Шлем шлёт данные на IP отправителя hello.
- **PTA1** программа → драйвер (127.0.0.1:7788, текст): `PTA1 tracking=0|1 rate=N trackers=a,b,c|none`, раз в 1 с
  и при изменениях. Нет команд 3 с → драйвер считает программу отключённой: трекинг включён, набор прежний.
- **JSON-состояние** драйвер → программа (127.0.0.1:7790) раз в 0.5 с: app, tracking, data, hmd, packets_per_s,
  headset_ip, body_tracked, trackers_ready, rate, calibration{valid, error, samples, yaw}, trackers[{name, role,
  enabled, added, valid}]. Плюс `{"type":"poses","joints":[[x,y,z,valid]×24]}` ~30 раз/с (только пока
  программа подключена и идут данные) — для 3D-вида; координаты STAGE шлема.

### Приложение на шлеме
- Сервис постоянный (START_STICKY): onCreate → foreground-уведомление, WifiLock HIGH_PERF (deprecated, но
  LOW_LATENCY действует только на переднем плане), MulticastLock (иначе broadcast hello может отбрасываться),
  поток-слушатель UDP 7789 (таймаут 1 с).
- hello от X → если трекинг не идёт или идёт на другой адрес — `Native.start(X:data_port)`; нет hello 5 с или
  bye → `Native.stop()`. Интент с `host` — ручной режим (hello игнорируются), интент без `host` — назад в авто.
- `BodyTracking::init()` → instance (`XR_BD_body_tracking`, `XR_MND_headless`, `XR_KHR_convert_timespec_time`,
  android create instance, apiVersion 1.0) → system → headless-сессия → STAGE → трекер тела (FULL_BODY / WITHOUT_ARM).
  Цикл с частотой `g_rate` (по умолчанию 100/с, 10–250, `Native.setRate` из hello): `xrLocateBodyJointsBD` в IDLE;
  раз в 1 с `xrGetBodyTrackingStateBD` (PICO-сигнатура) → флаг 2. Трекеры не подключены → скелет не шлётся, только
  пакет состояния раз в 200 мс (`stats sent=` считает только скелеты). Если рантайм ни разу не ответил на запрос
  состояния — считаем трекеры подключёнными (иначе трекинг не пошёл бы вовсе; лог «tracker state unavailable»).
- `onDestroy` сервиса пишет `listener: service stopped` — по ней программа сразу видит выключенный сервис.
- **Выбор «выключен»** (1.6): интент с `enabled` (boolean extra; `am start-foreground-service … --ez enabled false|true`)
  — выбор пользователя, сохраняется в SharedPreferences `service`/`disabled` (commit) и пишется в лог `listener:
  service enabled|disabled by user`. Выключен → `onStartCommand` делает `stopSelf()` и `START_NOT_STICKY` для любого
  запуска без `enabled=true` (BootReceiver его и не шлёт, MainActivity при открытии не стартует сервис). Кнопки
  в приложении: «Авто» = enabled true, «Выключить» = enabled false, «Передавать вручную» тоже включает.
  Ошибка/потеря сессии → повтор через 2 с. Поток nice −10.
- Global ref на контекст — **один на процесс** (loader запоминает его при первом `xrInitializeLoaderKHR`;
  пересоздание роняло процесс «JNI ERROR: accessed deleted Global» — исправлено).
- logcat `PicoTrackers`: `stats sent=… tracked=… trackers=… session=… err=… target=…` (раз в 1 с) и
  `listener: waiting… / tracking -> ip:port (причина) / tracking stopped (причина)` — их разбирает программа.
- BootReceiver после реальной перезагрузки шлема **не проверен** (shell не может послать BOOT_COMPLETED).

### Драйвер
- Сборка: `driver/build.sh` — Linux (`build/cmake-sniper`, тулчейн `cmake/linux-sniper.cmake`: clang + lld +
  sysroot Debian 11 из `fetch_sysroot.py` — glibc 2.31, libstdc++/libgcc GCC 10 статически; build.sh падает, если
  .so требует glibc > 2.31 или GLIBCXX) и Windows (`build/cmake-windows`, тулчейн `cmake/mingw-w64.cmake`,
  `-static`, ws2_32) в одну папку `build/picotrackers` (bin/linux64 + bin/win64); `-s`.
- **Windows ABI**: SteamVR собран MSVC, драйвер — MinGW; у `GetPose()` (возвращает структуру) разный порядок
  this/адреса результата. Поэтому SteamVR получает `TrackerDevice::MsvcShim` — объект с ручной vtable из
  свободных функций (this первым, как у MSVC). У провайдера методов со структурами нет — обычный класс.
- Сокеты: winsock на Windows (broadcast только 255.255.255.255 + известный IP), на Linux — SOCK_CLOEXEC.
- Регистрация: мастер ставит в `dataDir()/driver/picotrackers` и регистрирует через vrpathreg; старая регистрация
  `driver/build/picotrackers` (из исходников) снимается мастером.
- Настройки `driver_picotrackers`: `trackers` (по умолчанию 10 с ролями; `all`, `none`), `rate` (снимков/с, 10–250,
  по умолчанию 100), `tracking` (просить ли данные у шлема, по умолчанию true) — все три сохраняются при изменениях
  из программы и действуют без неё (программа отключилась → «работаю с её настройками…», ничего не сбрасывается);
  `port` 7788, `headset_ip`.
- Трекеры (serial `PICOTRK-<имя>`): waist→PELVIS, chest→SPINE3, *_foot→ANKLE, *_knee, *_elbow, *_shoulder —
  с ролями `TrackerRole_*`; без роли (`TrackerRole_None`): *_hip, spine1, spine2, *_toes(FOOT), neck, *_collar,
  head, *_wrist, *_hand.
- Все 24 объекта создаются в Init. Включённый добавляется в SteamVR (`TrackedDeviceAdded` + роль в
  `trackers./devices/picotrackers/<serial>`) при первых свежих данных. Удалить устройство до перезапуска
  SteamVR нельзя — выключенный получает `deviceIsConnected=false` (игры его не видят), включение возвращает.
- `RunFrame`: команды программы → поза HMD (`GetRawTrackedDevicePoses`, индекс 0) → свежесть данных (< 0.5 с) →
  калибровка → позы трекеров → раз в 0.5 с JSON программе → раз в 10 с строка в vrserver.txt.
  `fresh_` — шлем на связи (любые пакеты), `poses_` — есть позы: флаг 2 + `joint_count > 0`. Только при `poses_`:
  калибровка, добавление трекеров в SteamVR, позы, 3D-вид. Шлем на связи, а трекеры не подключены → у добавленных
  трекеров `deviceIsConnected=false` (как у выключенных); просто нет данных → OutOfRange, как раньше. Смена
  состояния трекеров — строка в логе. `packets_per_s` считает только пакеты с позами.
- **Калибровка** (`calibration.cpp`): модель `HMD_raw = T · head · D`; T = поворот вокруг Y + сдвиг, D —
  смещение HMD от сустава head (Q + d). Пары раз в 100 мс при неподвижном HMD (< 0.3 м/с, < 1 рад/с), окно 300.
  Yaw и Q — чередованием, t и d — МНК 6×6 с ridge на d (пока пар < 30: Q = 1, d ≈ 0). 10 пар подряд с ошибкой
  > 0.25 м → сброс выборки. Пересчёт раз в 0.5 с.
- `-static-libstdc++ -static-libgcc`; требует glibc ≤ 2.17 (sysroot bullseye). Проверка загрузки с glibc sniper:
  ld-2.31.so из `SteamLinuxRuntime_sniper/sniper_platform_*/files/lib/x86_64-linux-gnu` + `--library-path` с
  симлинками libc.so.6 и т.п. + маленький загрузчик на C (dlopen RTLD_NOW + HmdDriverFactory). Ошибки `[Input] … has no configured binding` безвредны. Пока HMD не подключён,
  SteamVR держит трекеры в «started activation»; `Activate()` — при подключении HMD.
- Обновлённый `.so` подхватывается только перезапуском SteamVR (`pkill -TERM -x vrmonitor; pkill -TERM -x
  vrserver`, затем `steam steam://rungameid/250820`; Steam Link на шлеме нужно переподключить).

### Программа на ПК (`pc/`)
- Драйвер и APK вшиты (go:embed); программа их **не собирает** и сама ничего не ставит без мастера —
  в том числе **не создаёт пункт меню сама** (раньше `installMenuEntry` при каждом запуске; пользователь попросил
  убрать программу из меню — теперь только шаг мастера «Программа»).
- **Linux: программа — сама себе установщик** (`selfInstall`): шаг «Программа» копирует запущенный файл в
  `~/.local/bin/picotrackers` (replaceFile), значок в `~/.local/share/icons/hicolor/scalable/apps/<appID>.svg`,
  пункт меню `~/.local/share/applications/<appID>.desktop` (переключатель, Icon=<appID>). Настройки → «Удалить
  программу» = `uninstall()` (+ removeProgramFiles) и выход. На Windows `selfInstall=false` — установщик.
- Версия программы — `appVersion` в core.go (окно «О программе», ресурсы exe, имя установщика, сравнение с тегом
  релиза при обновлении). Новый релиз: поднять appVersion, закоммитить, тег `v<appVersion>` → CI.
- **Автообновление** (`update.go`, `updatedialog.go`): при запуске окна горутина чистит `dataDir()/update`, спрашивает
  `api.github.com/repos/Mr1rbis/PICOtrackers/releases/latest` (таймаут 15 с; `PICOTRACKERS_UPDATE_URL` подменяет адрес —
  для тестов). Тег новее `appVersion` (числа через точку; `/latest` не отдаёт pre-release) и в релизе есть программа для
  этой ОС + `PicoTrackers.apk` + `SHA256SUMS.txt` → строка в настройках и окно обновления; **мастер в этом случае не
  открывается** (обновление и так обновит драйвер и APK). Нет сети / ошибка — строка в журнале, дальше как раньше.
  - Условия (`UpdateConditions`, опрос раз в 2 с): `steamVRStartedAt` = не запущен, `usbHeadset()` в состоянии `device`
    (Wi-Fi adb не считается). Нет adb — кнопка «Скачать adb». Проверяются и перед загрузкой, и перед установкой.
  - `Release.Install`: SHA256SUMS → программа и APK в память, сверка хешей → APK на шлем по USB, если хеш на шлеме
    другой (`installedAPKHash`) → `installUpdate`. Во время установки окно закрыть нельзя (`SetCanClose(false)`).
  - Linux: заменяет (replaceFile) запущенный файл и копию в `~/.local/bin`, если они разные; нет прав на папку
    (`canSelfUpdate`, например /usr) — только кнопка «Страница загрузки». Драйвер ставит новая версия:
    `<новый файл> install-driver`, если драйвер был установлен. Перезапуск — `<файл> --wait-pid=<pid> --updated`
    (Setsid), новая версия ждёт выхода старой (`/proc/<pid>/stat`, зомби считается вышедшим), иначе D-Bus-имя
    приложения ещё занято и новый процесс просто активирует старый.
  - Windows: только установленная установщиком (рядом `unins000.exe`), переносная из zip — «Страница загрузки».
    Установщик → `dataDir()\update\`, затем PowerShell `Wait-Process <pid>; Start-Process setup /SILENT
    /SUPPRESSMSGBOXES /NORESTART /UPDATE` и выход программы (её mutex = AppMutex, в тихом режиме установка бы прервалась).
- **Установщик Windows** (`windows/installer.iss`): `{autopf}` при `PrivilegesRequired=lowest` = `%LOCALAPPDATA%\Programs\
  PICO Trackers`. После копирования файлов (ssPostInstall, галочка «driver») — `picotrackers.exe install-driver
  --message-file=…`, при ошибке окно «Повторить / Пропустить» (код 3 — «закройте SteamVR»); сообщение читается
  из файла (UTF-8), а не из stdout — кодировку перехваченного вывода Inno не документирует.
  Режим `/UPDATE` (запуск из программы): галочки — с прошлой установки; драйвер ставится и без галочки, если он уже
  есть в `{localappdata}\PICOtrackers\driver\picotrackers` (мог поставить мастер программы); на `ssDone` —
  `picotrackers.exe --updated`. В тихом режиме окно «Повторить / Пропустить» отвечает «Пропустить». Деинсталлятор
  (usUninstall) — `picotrackers.exe uninstall`: `registerDriver("")` (снять все регистрации picotrackers), автозапуск
  выкл., `adb kill-server` своего adb, удалить dataDir и configDir. Программа держит mutex `PICOtrackers.Control`
  (`markRunning`) — по нему установщик просит её закрыть. Галочка «автозапуск» пишет то же значение реестра,
  что переключатель в программе. Регистрацию драйвера в обоих случаях делает `registerDriver(keep)`.
- **Мастер установки** (`wizard.go`), открывается при запуске, если: не пройден ни разу (`wizard_done`), драйвер не
  установлен/устарел/есть другие регистрации picotrackers, или подключён по USB шлем без актуального APK; также
  из настроек, меню и кнопки «Открыть мастер». Шаги: приветствие → драйвер (копирование в
  `dataDir()/driver/picotrackers`, `version.txt` = хеш вшитого драйвера, `vrpathreg removedriver` старых копий,
  `adddriver` новой; если vrpathreg не сработал — правка openvrpaths.vrpath; на Windows при запущенном SteamVR
  просит закрыть его) → шлем (опрос раз в 2 с: adb есть? если нет — «Скачать adb» = platform-tools от Google в
  `dataDir()/platform-tools`; устройство USB device/unauthorized; состояние APK по sha256; «Установить и далее» =
  install → `adb tcpip` + connect → start-foreground-service; «Пропустить») → готово.
- Пути: Linux `~/.local/share/picotrackers` (данные), `~/.config/picotrackers/config.json`; Windows
  `%LOCALAPPDATA%\PICOtrackers` (и данные, и конфиг), openvrpaths — `%LOCALAPPDATA%\openvr\openvrpaths.vrpath`,
  vrpathreg — `SteamVR\bin\win64\vrpathreg.exe`.
- `Controller.connectLoop`: раз в 5 с (или по `Reconnect()`) — adb (сохранённый Wi-Fi адрес, иначе USB),
  проверка APK; если стоит — `am start-foreground-service` без host (авторежим) → LogcatFollower. Ничего не ставит.
  Дальше, пока шлем на связи, раз в 5 с `serviceRunning` (`dumpsys activity services <пакет>/.TrackingService`:
  есть `ServiceRecord{…TrackingService}` или «(nothing)»); строки лога `listener: waiting…` / `listener: service
  stopped` меняют состояние сразу. Выключенный сервис программа сама не включает — карточка «Сервис на шлеме
  выключен» с «Включить сервис». При подключении к шлему (`tryConnect`, мастер, `picotrackers setup`) сервис
  запускается обычным `startService` (без extra — приложение 1.6 его не запустит, если выключили на шлеме), но
  **не запускается вовсе, если `cfg.ServiceOff`** (кнопкой «Выключить сервис» или `picotrackers stop`). Кнопки —
  `setServiceEnabled(h, on)`: `--ez enabled on` + `ServiceOff = !on`; при выключении ждёт до 2 с, пока сервис
  остановится сам, иначе (приложение до 1.6 extra не знает) — `am stopservice`. Карточка при `ServiceOff`:
  «Выключен вами: сам он не включится, пока не нажмёте кнопку». Включение и выключение — **одна кнопка**
  в строке «Шлем PICO» и **один пункт меню** (`app.toggle-service` → `ToggleService`): надпись по состоянию сервиса
  по adb, от SteamVR не зависит — «Выключить сервис» / «Включить сервис» (выделена), на время операции «Включаю…» /
  «Выключаю…» и неактивна (`Snapshot.ServiceChange`); нет связи по adb — кнопки нет, пункт меню неактивен. Надпись
  пункта меню меняется через `gio.Menu` Remove/Insert (позиция 1). Действия `app.start-service` / `app.stop-service`
  остались без пунктов меню — для D-Bus `org.gtk.Actions.Activate`.
- APK собирается воспроизводимо (фиксированное время файлов, `zip -X`) — иначе хеш менялся бы при каждой сборке.
- `installApp`: если `adb install -r` отвечает `INSTALL_FAILED_UPDATE_INCOMPATIBLE` (на шлеме APK с другой подписью —
  локальная сборка против релиза из CI), приложение удаляется (`adb uninstall`) и ставится заново. На шлеме не проверено.
- Конфиг: `adb_wifi`, `tracking`, `trackers` (если нет — берётся из первого состояния драйвера), `rate`, `wizard_done`,
  `service_off` (сервис на шлеме выключен кнопкой).
- Окно — вкладки (Adw.ViewStack + ViewSwitcher в заголовке):
  - «Трекеры»: карточка общего состояния, «Шлем / SteamVR / Калибровка», «Передавать трекинг», «Частота
    передачи» (Adw.SpinRow), схема «человечек» (`body.go`: фигура со спины в центре, 10 трекеров с
    переключателями в двух колонках, линии к местам на теле; цвет — состояние) + наборы «Ноги / Основные / Все»,
    «Дополнительные суставы» (14, раскрывающийся список);
  - «3D-вид» (`view3d.go`): скелет линиями, включённые трекеры точками с подписями, сетка пола; камера за
    спиной и следует за поворотом (направление по линии бёдер), перетаскивание — облёт, колесо — масштаб;
    перерисовка ~30 раз/с только когда вкладка открыта;
  - «Настройки»: автозапуск при входе (`~/.config/autostart/picotrackers.desktop`), переустановка, журнал.
  Пункт меню приложений — `~/.local/share/applications/dev.picotrackers.Control.desktop`.
- Переключатели трекеров — общий интерфейс `trackerControl` (схема и строки списка); `updating` гасит
  обратные вызовы, когда переключатели меняет программа.
- Единственный экземпляр — через D-Bus (`dev.picotrackers.Control`) и занятый UDP 7790.
- `slog.SetLogLoggerLevel(Warn)` — иначе GTK засыпает stderr сообщениями Vulkan. Предупреждение
  «gtk-application-prefer-dark-theme with libadwaita is unsupported» — от настроек KDE, безвредно.

### Проверка
```sh
pc/build/picotrackers setup                      # сборка, регистрация, установка, сервис на шлеме
adb -s 192.168.0.11:5555 logcat -s PicoTrackers  # listener: … / stats …
grep picotrackers ~/.local/share/Steam/logs/vrserver.txt
python3 -c "import socket,struct;s=socket.socket(2,2);s.setsockopt(1,6,1);s.sendto(b'PTC1'+struct.pack('<HH',7788,1),('192.168.0.255',7789))"  # поддельный hello
```

### CI (`.github/workflows/build.yml`, GitHub Actions)
- Триггеры: push в main/master, теги `v*`, pull_request, workflow_dispatch; новый запуск на той же ветке отменяет
  старый (кроме тегов). Действия: checkout@v7, cache@v6, upload-artifact@v7, download-artifact@v8, setup-java@v6
  (последние мажорные на 2026-10). Общие шаги — YAML-якоря (`&safe-directory`, `*driver-asset` …), actionlint 1.7.12 чист.
- `driver` (контейнер `archlinux:base-devel`): pacman git clang lld cmake ninja python mingw-w64-gcc → `driver/build.sh`
  → артефакт `driver`. На теге первым делом проверка `тег == v$appVersion` — иначе всё падает сразу.
- `headset` (ubuntu-24.04 без контейнера — Android SDK есть в образе раннера): Temurin 25, sdkmanager
  `build-tools;37.0.0`, `platforms;android-37.0`, `ndk;29.0.14206865`, `ANDROID_NDK_HOME` переопределяется (в образе —
  другой NDK); ключ из секрета `HEADSET_KEYSTORE_BASE64` (нет — warning и одноразовый ключ) → артефакт `headset-apk`.
- `linux` / `windows` (Arch, после driver+headset): артефакты → `pc/assets`, `PREBUILT_ASSETS=1`; кэш
  `~/.cache/go-build` + `~/go/pkg/mod` с ключом `go-<os>-<GOVERSION>-<hash go.sum>`. `git config safe.directory` обязателен:
  рабочая папка принадлежит другому uid, без него `go build` падает на «error obtaining VCS status».
  linux: + `go test ./...`, печать требуемой glibc (2.34 на 2026-10) → артефакт `picotrackers-linux`.
  windows: + wine xorg-server-xvfb librsvg imagemagick zip python, `xvfb-run -a pc/build-windows.sh` → артефакт
  `picotrackers-windows` (setup.exe + zip).
- `release` (только тег): `gh release create` — `picotrackers-linux-x86_64`, setup.exe, zip, APK, `SHA256SUMS.txt`.
- Локальная проверка: git-клон (только отслеживаемые файлы) + те же команды в `docker run archlinux:base-devel`
  (`--cpus=4`, рабочая папка с чужим uid) и в `ubuntu:24.04` + Temurin 25 для APK. Так найдено: на свежем клоне нет
  `pc/build`, Wine не создаёт родителя `WINEPREFIX` → `wineprefix.sh` теперь делает `mkdir -p` и падает с понятной
  ошибкой; в контейнере не задан `USER` → `${USER:-$(id -un)}`. Wine в контейнере работает от root без проблем.

## Что дальше / идеи
- Дождаться отзыва: живое переключение трекеров, 3D-вид после исправления зеркальности, VRChat, задержка.
- Предсказание: скорости по разности поз + `poseTimeOffset`.
- Конфликт: в SteamVR зарегистрирован «Standable: Full Body Estimation» (свои виртуальные трекеры) и ALVR nightly.
- Если трекеры не откалиброваны (`status=0`) — `xrStartBodyTrackingCalibAppBD`.
- Проверить BootReceiver после перезагрузки шлема (может мешать автозапуск-политика PICO OS).

## Соглашения
- Комментарии в коде, UI и лог драйвера — на русском; logcat приложения — на английском.
- C++17; Java без внешних библиотек (UI кодом); Go — gofmt. IDE показывает ложные ошибки в Java (нет android.jar
  в её classpath) — собирать через build.sh.
- Сторонние зависимости не коммитить — только через fetch_deps.sh / go.sum с закреплёнными версиями.
- Проект в git, GitHub `Mr1rbis/PICOtrackers`. Коммитить и пушить — только когда пользователь попросит.
