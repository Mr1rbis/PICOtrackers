# PICO Trackers → SteamVR (Steam Link)

**[Русский](#русский)** · **[English](#english)**

---

## Русский

> [!WARNING]
> **Проект на 100% написан ИИ** (Claude). Автор репозитория сделал его, чтобы быстро решить свою
> проблему без заморочек. Код руками не трогался (было лень), так что внутри может быть много нейрослопа.
> У автора работает — и ладно.

PICO Motion Trackers на ногах → трекеры в SteamVR, когда вы играете в PCVR через **Steam Link**.

Подходит для связки **Steam Link + PICO Motion Trackers** — через программу PICO Trackers. Сам Steam Link
трекеры PICO не передаёт (на октябрь 2026 Valve обещает когда-нибудь), а PICO Connect передаёт, но только
под Windows и только если играть через него.

Как это устроено: приложение на шлеме берёт скелет тела, который PICO сам считает по трекерам, и шлёт его
по Wi-Fi драйверу SteamVR на ПК. Драйвер превращает суставы в трекеры SteamVR: пояс, грудь, стопы, колени,
локти, плечи (10 штук по умолчанию) и ещё 14 суставов по желанию. Программа на ПК ставит всё это и даёт
включать и выключать трекеры на лету.

```
шлем: сервис PICO Trackers ── UDP 7788: скелет, ~100 раз/с ──► ПК: драйвер SteamVR picotrackers
        ▲  (запускается вместе со шлемом)                            │  ▲
        └────────────── UDP 7789: «нужны данные» ────────────────────┘  │ 127.0.0.1: состояние / команды
                                                                        ▼
                                                   программа PICO Trackers (окно, Go + GTK 4)
```

Видео, шлем и контроллеры по-прежнему идут через Steam Link — программа добавляет только трекинг тела.

### Что проверено

| Где | Статус |
|---|---|
| Linux (Arch / CachyOS), PICO 4 Pro, Steam Link 2.0.22, SteamVR 2.17 | ✅ работает у автора |
| Windows 10/11 | 🟡 собирается, установщик и программа запускаются под Wine; на настоящей Windows ещё не проверялось — ближайшие тесты будут на Windows |
| PICO 4 (не Pro), PICO 4 Ultra | ❔ не проверялось |

### Что нужно

**Везде**
- PICO 4 Pro и PICO Motion Trackers, сопряжённые и откалиброванные в системном приложении PICO «Motion Tracker».
- Steam Link на шлеме, Steam и SteamVR на ПК (SteamVR хотя бы раз запускался).
- Шлем и ПК в одной локальной сети.
- Для первой установки — USB-кабель и **отладка по USB** на шлеме: включить режим разработчика
  (Настройки → Общие → Об устройстве → несколько раз нажать «Версия ПО»), затем в разделе для разработчиков
  включить отладку по USB и разрешить её на шлеме, когда он спросит. adb ставить не обязательно: если его нет,
  программа скачает его сама.

**Linux**
- Дистрибутив уровня GNOME 50: **GTK ≥ 4.22 и libadwaita ≥ 1.9** — Arch и производные, Fedora 44+,
  Ubuntu 26.04+, openSUSE Tumbleweed. На более старых (Ubuntu 24.04, Mint 22, Debian 13, SteamOS) программа
  не запустится. Сам драйвер SteamVR собран под Steam Runtime и от дистрибутива не зависит, но ставит его программа.
- Пакеты (на десктопе с GNOME обычно уже стоят):

  | Дистрибутив | Команда |
  |---|---|
  | Arch и производные | `sudo pacman -S --needed gtk4 libadwaita gobject-introspection-runtime` |
  | Fedora | `sudo dnf install gtk4 libadwaita gobject-introspection` |
  | Ubuntu 26.04+ | `sudo apt install libgtk-4-1 libadwaita-1-0 libgirepository-1.0-1` |
  | openSUSE Tumbleweed | `sudo zypper install libgtk-4-1 libadwaita-1-0 libgirepository-1_0-1` |

- Если adb пишет `no permissions` — нужны udev-правила для Android-устройств: на Arch пакет `android-udev`,
  на Ubuntu/Debian — `android-sdk-platform-tools-common`.
- Если включён брандмауэр — открыть входящий **UDP 7788**: `sudo ufw allow 7788/udp` или
  `sudo firewall-cmd --permanent --add-port=7788/udp && sudo firewall-cmd --reload`.

**Windows**
- Windows 10/11 x64. Больше ничего ставить не надо: GTK и всё нужное лежит в установщике.
- Установщик не подписан, поэтому SmartScreen скажет «Windows защитила ваш компьютер» →
  «Подробнее» → «Выполнить в любом случае».
- Брандмауэр: данные со шлема принимает SteamVR (`vrserver.exe`, UDP 7788). Если Windows спросит —
  разрешить доступ в частных сетях. Сеть Wi-Fi/Ethernet должна быть «Частной».
- Если программа не видит шлем по кабелю — проверьте, что на шлеме нажали «Разрешить» в запросе отладки по USB;
  если не помогло — поставьте ADB-драйвер (например, Google USB Driver).

### Установка

Готовые файлы — на странице [Releases](../../releases/latest).

**Linux.** Скачать `picotrackers-linux-x86_64` и запустить:

```sh
chmod +x picotrackers-linux-x86_64
./picotrackers-linux-x86_64
```

Программа сама себе установщик: откроется мастер с кнопками «Далее».
1. Программа — копия в `~/.local/bin/picotrackers`, значок и пункт «PICO Trackers» в меню (по желанию).
2. Драйвер SteamVR — копируется в `~/.local/share/picotrackers` и регистрируется в SteamVR.
3. Приложение на шлем — по USB-кабелю, затем adb переключается на Wi-Fi, и кабель можно отключить.

После этого скачанный файл можно удалить. Права администратора не нужны.

**Windows.** Запустить `PICOtrackers-setup-<версия>.exe`. Права администратора не нужны: программа ставится
для текущего пользователя. Галочки: драйвер SteamVR (SteamVR перед этим лучше закрыть), значок на рабочем
столе, автозапуск. При первом запуске программа поможет поставить приложение на шлем — подключите его кабелем.
Есть и переносная версия без установки: `PICOtrackers-windows.zip` (распаковать, запустить `picotrackers.exe`).

### Как пользоваться

Дальше всё автоматически: запускаете Steam Link → стартует SteamVR → драйвер просит шлем прислать данные →
шлем начинает трекинг. Закрыли SteamVR — через 5 секунд шлем перестаёт передавать. Калибровка (совмещение
скелета с позицией шлема в SteamVR) идёт сама в первые секунды.

Шлем должен быть **надет**: снятый шлем засыпает, и трекинга нет.

Трекеры PICO должны быть **подключены к шлему**. Пока их нет, шлем не передаёт скелет (без трекеров PICO
достраивает ноги сам, и они были бы выдуманными), а в SteamVR трекеры показываются отключёнными. В программе
это видно как «Трекеры не подключены».

Окно программы:
- **Трекеры** — состояние шлема, SteamVR и калибровки; «Передавать трекинг»; частота передачи
  (10–250 раз/с); схема-человечек с переключателями трекеров; наборы «Ноги / Основные / Все» и ещё
  14 дополнительных суставов. Изменения сразу видны в SteamVR: выключенный трекер отключается и пропадает из игр.
- **3D-вид** — скелет в реальном времени: мышью — облёт, колесом — масштаб.
- **Настройки** — автозапуск при входе в систему, мастер установки, удаление, журнал.

Программу можно не держать открытой: драйвер запоминает её настройки (передавать ли трекинг, набор трекеров,
частоту) и работает с ними и без неё, и после перезапуска SteamVR.

Если сервис на шлеме выключен (кнопкой в приложении на шлеме, из меню программы или после переустановки
приложения), программа это покажет и предложит «Включить сервис» — нужна связь со шлемом по adb.

**Обновление.** При каждом запуске программа проверяет, нет ли на GitHub версии новее. Если есть — откроется окно
«Доступна версия …». Обновление скачивается и ставится, только когда **SteamVR закрыт** и **шлем подключён
к компьютеру USB-кабелем** с разрешённой отладкой по USB: вместе с программой обновляются драйвер SteamVR и
приложение на шлеме. Окно можно закрыть кнопкой «Позже» — при следующем запуске оно откроется снова (и есть
строка «Обновление» в настройках). Linux: программа заменит свой файл и перезапустится. Windows: программа
закроется, установщик обновит её в тихом режиме и запустит снова; переносная версия из zip сама не обновляется.
Файлы сверяются с `SHA256SUMS.txt` релиза. Обновляться так умеет версия 1.4 и новее — с 1.3 обновитесь вручную.

**Удаление.** Linux: Настройки → «Удалить программу» (или `picotrackers uninstall`). Windows: «Параметры →
Приложения» — деинсталлятор снимает драйвер с регистрации в SteamVR и удаляет данные программы.

Команды: `picotrackers setup` (установка без мастера), `picotrackers stop` (выключить сервис на шлеме),
`picotrackers install-driver` / `picotrackers uninstall` (их вызывает установщик Windows).

### Если не работает

- Журнал событий — внизу окна программы.
- Лог драйвера: `~/.local/share/Steam/logs/vrserver.txt` (Windows: `Steam\logs\vrserver.txt`), строки `picotrackers:`.
- Лог шлема: `adb logcat -s PicoTrackers`.
- «Сервис на шлеме выключен» — кнопка «Включить сервис» (или меню → «Включить сервис на шлеме»).
- Шлем не начинает передавать — сеть может резать broadcast (изоляция клиентов в Wi-Fi). Тогда укажите IP шлема
  в настройках SteamVR: `steamvr.vrsettings`, секция `driver_picotrackers`, ключ `headset_ip`.
- `tools/udp_listen.py` — показать приходящие со шлема суставы (при выключенном SteamVR: порт общий).

### Сборка из исходников

Всё собирается на Linux, Windows-версия — кросс-сборкой.

```sh
pc/build.sh            # драйвер (Linux + Windows) + APK → вшиваются в программу → pc/build/picotrackers
pc/build-windows.sh    # то же для Windows → pc/build/PICOtrackers-setup-<версия>.exe и PICOtrackers-windows.zip
```

Пакеты (Arch — на нём собирает и автор, и CI):

```sh
sudo pacman -S --needed base-devel git go gtk4 libadwaita gobject-introspection \
    clang lld cmake ninja python jdk-openjdk zip unzip \
    mingw-w64-gcc wine xorg-server-xvfb librsvg imagemagick   # последняя строка — только для Windows-версии
```

Плюс Android SDK в `/opt/android-sdk` (или `ANDROID_HOME`): `build-tools;37.0.0`, `platforms;android-37.0`,
`ndk;29.0.14206865`. Для сборки под Windows нужен Python ≥ 3.14 (пакеты MSYS2 сжаты zstd).

Остальное скачивается само при первой сборке, с проверкой sha256: loader PICO OpenXR SDK, заголовки OpenXR и
OpenVR, sysroot Debian 11 для драйвера, Go-модули, GTK для Windows из MSYS2, Inno Setup. Первая сборка долгая:
биндинги GTK компилируются ~10 мин под Linux и ~15 мин под Windows.

`headset/debug.keystore` — ключ подписи APK, создаётся при первой сборке. Не удаляйте его, иначе обновление
приложения на шлеме пойдёт через удаление и повторную установку.

**CI** (`.github/workflows/build.yml`, GitHub Actions). На каждый push и pull request: драйвер, APK, программа
для Linux (с тестами) и для Windows (установщик + zip); файлы — в артефактах запуска. Тег `vX.Y`, совпадающий
с `appVersion` в `pc/core.go`, создаёт релиз со всеми файлами и `SHA256SUMS.txt`. Чтобы APK из CI был подписан
тем же ключом, что и локальные сборки, добавьте секрет репозитория `HEADSET_KEYSTORE_BASE64`:

```sh
base64 -w0 headset/debug.keystore   # вывод → Settings → Secrets and variables → Actions → HEADSET_KEYSTORE_BASE64
```

Как всё устроено внутри и что выяснилось про OpenXR-рантайм PICO — в [CLAUDE.md](CLAUDE.md) (рабочие заметки ИИ).

### Покопаться руками

Хотите поковырять код лапками? Ковыряйтесь на здоровье, лапки не отвалятся~ :3 *виляет хвостиком*
Issue и pull request'ы приветствуются. Нейрослопа не пугайтесь — он не кусается (наверное) OwO

Проект не связан с PICO (ByteDance) и Valve.

---

## English

> [!WARNING]
> **This project is 100% AI-written** (Claude). The repo author made it to quickly solve their own problem
> without any hassle. Not a single line was touched by hand (too lazy), so there may be plenty of AI slop
> inside. It works for the author, and that's good enough.

PICO Motion Trackers on your legs → SteamVR trackers while you play PCVR over **Steam Link**.

Made for the **Steam Link + PICO Motion Trackers** combo, via the PICO Trackers app. Steam Link itself doesn't
forward PICO trackers (as of October 2026 Valve says "hopefully some day"), and PICO Connect does, but only on
Windows and only if you stream through it.

How it works: an app on the headset takes the body skeleton that PICO computes from the trackers and sends it
over Wi-Fi to a SteamVR driver on the PC. The driver turns the joints into SteamVR trackers: waist, chest,
feet, knees, elbows, shoulders (10 by default), plus 14 more joints if you want them. The PC app installs all
of this and lets you turn trackers on and off on the fly.

```
headset: PICO Trackers service ── UDP 7788: skeleton, ~100 Hz ──► PC: SteamVR driver picotrackers
           ▲  (starts with the headset)                                │  ▲
           └─────────────── UDP 7789: "send me data" ──────────────────┘  │ 127.0.0.1: state / commands
                                                                          ▼
                                                     PICO Trackers app (window, Go + GTK 4)
```

Video, the headset and the controllers still go through Steam Link as usual; this only adds body tracking.

### What's been tested

| Setup | Status |
|---|---|
| Linux (Arch / CachyOS), PICO 4 Pro, Steam Link 2.0.22, SteamVR 2.17 | ✅ works for the author |
| Windows 10/11 | 🟡 builds; the installer and the app start under Wine; not tried on real Windows yet, that's next |
| PICO 4 (non-Pro), PICO 4 Ultra | ❔ not tested |

### Requirements

**Everywhere**
- PICO 4 Pro and PICO Motion Trackers, paired and calibrated in PICO's system "Motion Tracker" app.
- Steam Link on the headset, Steam and SteamVR on the PC (SteamVR started at least once).
- Headset and PC on the same local network.
- For the first install: a USB cable and **USB debugging** on the headset. Turn on developer mode
  (Settings → General → About → tap "Software version" several times), enable USB debugging in the developer
  section, and allow it on the headset when asked. You don't need to install adb: if it's missing, the app
  downloads it.

**Linux**
- A GNOME 50-era distro: **GTK ≥ 4.22 and libadwaita ≥ 1.9**, e.g. Arch and derivatives, Fedora 44+,
  Ubuntu 26.04+, openSUSE Tumbleweed. On older ones (Ubuntu 24.04, Mint 22, Debian 13, SteamOS) the app won't
  start. The SteamVR driver itself is built for the Steam Runtime and works on any distro, but the app is what
  installs it.
- Packages (usually already there on a GNOME desktop):

  | Distro | Command |
  |---|---|
  | Arch and derivatives | `sudo pacman -S --needed gtk4 libadwaita gobject-introspection-runtime` |
  | Fedora | `sudo dnf install gtk4 libadwaita gobject-introspection` |
  | Ubuntu 26.04+ | `sudo apt install libgtk-4-1 libadwaita-1-0 libgirepository-1.0-1` |
  | openSUSE Tumbleweed | `sudo zypper install libgtk-4-1 libadwaita-1-0 libgirepository-1_0-1` |

- If adb says `no permissions`, you need udev rules for Android devices: `android-udev` on Arch,
  `android-sdk-platform-tools-common` on Ubuntu/Debian.
- If you run a firewall, open incoming **UDP 7788**: `sudo ufw allow 7788/udp` or
  `sudo firewall-cmd --permanent --add-port=7788/udp && sudo firewall-cmd --reload`.

**Windows**
- Windows 10/11 x64. Nothing else to install: GTK and everything else is bundled in the installer.
- The installer isn't signed, so SmartScreen will say "Windows protected your PC" → "More info" → "Run anyway".
- Firewall: SteamVR (`vrserver.exe`, UDP 7788) receives the data from the headset. If Windows asks, allow
  access on private networks. Your Wi-Fi/Ethernet network should be set to "Private".
- If the app doesn't see the headset over USB, check that you tapped "Allow" on the USB debugging prompt on the
  headset; if that doesn't help, install an ADB driver (e.g. Google USB Driver).

### Installation

Downloads are on the [Releases](../../releases/latest) page.

**Linux.** Download `picotrackers-linux-x86_64` and run it:

```sh
chmod +x picotrackers-linux-x86_64
./picotrackers-linux-x86_64
```

The app is its own installer: a wizard with "Next" buttons opens.
1. The app: copied to `~/.local/bin/picotrackers`, with an icon and a "PICO Trackers" menu entry (optional).
2. The SteamVR driver: copied to `~/.local/share/picotrackers` and registered with SteamVR.
3. The headset app: installed over USB, then adb switches to Wi-Fi and you can unplug the cable.

After that you can delete the downloaded file. No root needed.

**Windows.** Run `PICOtrackers-setup-<version>.exe`. No admin rights needed: it installs for the current user.
Checkboxes: SteamVR driver (better close SteamVR first), desktop icon, start with Windows. On first launch the
app helps you install the headset app; plug the headset in with a cable. There's also a portable version,
`PICOtrackers-windows.zip` (unzip, run `picotrackers.exe`).

> The app's interface is in Russian for now.

### Usage

Everything else is automatic: start Steam Link → SteamVR starts → the driver asks the headset for data → the
headset starts tracking. Close SteamVR and the headset stops sending 5 seconds later. Calibration (aligning the
skeleton with the headset pose in SteamVR) happens by itself within the first few seconds.

You need to be **wearing** the headset: when it's off your head it goes to sleep and there's no tracking.

The PICO trackers have to be **connected to the headset**. Until they are, the headset doesn't send the skeleton
(without trackers PICO makes the legs up by itself) and the trackers show up as disconnected in SteamVR. The app
shows this as "Trackers not connected".

The app window:
- **Trackers**: headset, SteamVR and calibration status; "Send tracking"; send rate (10–250 per second);
  a body diagram with tracker toggles; "Legs / Main / All" presets and 14 extra joints. Changes show up in
  SteamVR right away: a disabled tracker is disconnected and disappears from games.
- **3D view**: live skeleton; drag to orbit, scroll to zoom.
- **Settings**: start on login, setup wizard, uninstall, event log.

You don't have to keep the app open: the driver remembers its settings (whether to send tracking, the tracker
selection, the rate) and keeps using them without the app and after a SteamVR restart.

If the service on the headset is off (stopped from the headset app, from the app menu, or after reinstalling the
headset app), the app shows it and offers "Enable service" — this needs an adb connection to the headset.

**Updates.** Every time it starts, the app checks GitHub for a newer version. If there is one, an "Update
available" window opens. The update is only downloaded and installed when **SteamVR is closed** and **the
headset is connected to the PC with a USB cable** with USB debugging allowed: the SteamVR driver and the headset
app are updated together with the program. You can close the window with "Later", but it will open again on the
next start (there's also an "Update" row in Settings). Linux: the app replaces its own file and restarts.
Windows: the app closes, the installer updates it silently and starts it again; the portable zip version doesn't
update itself. Files are checked against the release's `SHA256SUMS.txt`. Versions 1.4 and later can update
this way; update from 1.3 by hand.

**Uninstall.** Linux: Settings → "Remove program" (or `picotrackers uninstall`). Windows: Settings → Apps; the
uninstaller unregisters the driver from SteamVR and removes the app's data.

Commands: `picotrackers setup` (install without the wizard), `picotrackers stop` (stop the service on the
headset), `picotrackers install-driver` / `picotrackers uninstall` (used by the Windows installer).

### Troubleshooting

- Event log: at the bottom of the app window.
- Driver log: `~/.local/share/Steam/logs/vrserver.txt` (Windows: `Steam\logs\vrserver.txt`), lines starting with `picotrackers:`.
- Headset log: `adb logcat -s PicoTrackers`.
- "Service on the headset is off": click "Enable service" (or menu → "Enable service on the headset").
- The headset never starts sending: your network may be dropping broadcasts (Wi-Fi client isolation). Put the
  headset's IP into the SteamVR settings: `steamvr.vrsettings`, section `driver_picotrackers`, key `headset_ip`.
- `tools/udp_listen.py` prints the joints coming from the headset (with SteamVR closed, since it's the same port).

### Building from source

Everything builds on Linux; the Windows version is cross-compiled.

```sh
pc/build.sh            # driver (Linux + Windows) + APK → embedded into the app → pc/build/picotrackers
pc/build-windows.sh    # same for Windows → pc/build/PICOtrackers-setup-<version>.exe and PICOtrackers-windows.zip
```

Packages (Arch, which both the author and CI use):

```sh
sudo pacman -S --needed base-devel git go gtk4 libadwaita gobject-introspection \
    clang lld cmake ninja python jdk-openjdk zip unzip \
    mingw-w64-gcc wine xorg-server-xvfb librsvg imagemagick   # last line: Windows build only
```

Plus the Android SDK in `/opt/android-sdk` (or `ANDROID_HOME`): `build-tools;37.0.0`, `platforms;android-37.0`,
`ndk;29.0.14206865`. The Windows build needs Python ≥ 3.14 (MSYS2 packages are zstd-compressed).

Everything else is downloaded on the first build and checked against pinned sha256 hashes: the PICO OpenXR SDK
loader, OpenXR and OpenVR headers, a Debian 11 sysroot for the driver, Go modules, GTK for Windows from MSYS2, and
Inno Setup. The first build is slow: the GTK bindings take about 10 minutes to compile for Linux and about 15 for
Windows.

`headset/debug.keystore` is the APK signing key, created on the first build. Keep it, or updating the headset app
will mean uninstalling and reinstalling it.

**CI** (`.github/workflows/build.yml`, GitHub Actions). On every push and pull request it builds the driver, the
APK, the Linux app (and runs its tests) and the Windows app (installer + zip); the files are attached to the run as
artifacts. A `vX.Y` tag that matches `appVersion` in `pc/core.go` creates a release with all files and
`SHA256SUMS.txt`. To have CI sign the APK with the same key as your local builds, add the repository secret
`HEADSET_KEYSTORE_BASE64`:

```sh
base64 -w0 headset/debug.keystore   # output → Settings → Secrets and variables → Actions → HEADSET_KEYSTORE_BASE64
```

Internals and findings about PICO's OpenXR runtime are in [CLAUDE.md](CLAUDE.md) (the AI's working notes, in Russian).

### Wanna dig in by paw?

Wanna poke at the code with your own paws? Go right ahead, paw away~ :3 *wags tail*
Issues and pull requests are welcome. Don't be scared of the AI slop, it doesn't bite (probably) OwO

Not affiliated with PICO (ByteDance) or Valve.
