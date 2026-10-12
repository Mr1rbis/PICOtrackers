// Особенности Linux: пути, SteamVR, автозапуск через .desktop, установка и обновление самой программы.
// Программа бывает обычным файлом, AppImage (в нём свои библиотеки GTK — pc/appimage) и Flatpak (песочница:
// команды системы — через flatpak-spawn --host, данные — в ~/.var/app; pc/flatpak).
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

const (
	adbName            = "adb"
	platformToolsURL   = "https://dl.google.com/android/repository/platform-tools-latest-linux.zip"
	driverBinarySubdir = "bin/linux64/driver_picotrackers.so"
	appImageAsset      = "PICOtrackers-x86_64.AppImage"
	flatpakAsset       = "PICOtrackers.flatpak"
)

// packaging — как запущена программа: "" обычный файл, "appimage", "flatpak".
var packaging = func() string {
	switch {
	case os.Getenv("FLATPAK_ID") != "":
		return "flatpak"
	case os.Getenv("APPIMAGE") != "" && os.Getenv("APPDIR") != "":
		return "appimage"
	}
	return ""
}()

// xdgDir — папка XDG: во Flatpak она своя у приложения (~/.var/app/…), без него — как раньше (~/.local/share…),
// даже если XDG_*_HOME задана: иначе программа «потеряла» бы драйвер и настройки прошлых версий.
func xdgDir(variable, fallback string) string {
	if dir := os.Getenv(variable); packaging == "flatpak" && dir != "" {
		return dir
	}
	return filepath.Join(home(), fallback)
}

func dataDir() string { return filepath.Join(xdgDir("XDG_DATA_HOME", ".local/share"), "picotrackers") }

func configDir() string { return filepath.Join(xdgDir("XDG_CONFIG_HOME", ".config"), "picotrackers") }

func openVRPathsFile() string { return filepath.Join(home(), ".config/openvr/openvrpaths.vrpath") }

func vrpathregPath(runtime string) string { return filepath.Join(runtime, "bin/vrpathreg.sh") }

func hideConsole(*exec.Cmd) {}

// hostCommand — команда в системе, а не в песочнице Flatpak (там не видно процессов SteamVR и его файлов).
func hostCommand(name string, args ...string) (string, []string) {
	if packaging == "flatpak" {
		return "flatpak-spawn", append([]string{"--host", name}, args...)
	}
	return name, args
}

// systemLanguage — язык системы: русский или английский.
func systemLanguage() string {
	if language := localeLanguage(); language != "" {
		return language
	}
	return languageEnglish
}

// selfExecutable — файл программы: у AppImage — сам .AppImage, а не программа внутри него.
func selfExecutable() (string, error) {
	if packaging == "appimage" {
		return os.Getenv("APPIMAGE"), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// restoreHostEnv — AppImage: AppRun задаёт переменные для вшитых библиотек (модули gdk-pixbuf, схемы GSettings,
// XDG_DATA_DIRS…) и запоминает прежние значения в PICOTRACKERS_HOST_<имя> («1:значение» или «0:» — не была задана).
// Сначала GTK и GLib читают свои переменные (initGTKFromEnv), потом окружение возвращается системным: его получат
// adb, SteamVR, браузер и всё, что программа запускает.
func restoreHostEnv() {
	if packaging != "appimage" {
		return
	}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		name, ok := strings.CutPrefix(key, "PICOTRACKERS_HOST_")
		if !ok {
			continue
		}
		if was, old, _ := strings.Cut(value, ":"); was == "1" {
			os.Setenv(name, old)
		} else {
			os.Unsetenv(name)
		}
		os.Unsetenv(key)
	}
}

// initGTKFromEnv — заставляет GLib, GIO и gdk-pixbuf прочитать переменные окружения сейчас (они делают это
// один раз, при первом обращении), чтобы restoreHostEnv их не испортила.
func initGTKFromEnv() {
	if packaging != "appimage" {
		return
	}
	gdkpixbuf.PixbufGetFormats()
	gio.SettingsSchemaSourceGetDefault()
	gio.VFSGetDefault()
	glib.GetSystemDataDirs()
}

// steamVRStartedAt — время запуска vrserver; ok=false, если SteamVR не запущен.
func steamVRStartedAt() (time.Time, bool) {
	out, err := runHost("pgrep", "-x", "vrserver")
	pids := strings.Fields(out)
	if err != nil || len(pids) == 0 {
		return time.Time{}, false
	}
	elapsed, err := runHost("ps", "-o", "etimes=", "-p", pids[0])
	seconds, convErr := strconv.Atoi(strings.TrimSpace(elapsed))
	if err != nil || convErr != nil {
		return time.Now(), true
	}
	return time.Now().Add(-time.Duration(seconds) * time.Second), true
}

func launchSteamVR() {
	name, args := "steam", []string{fmt.Sprintf("steam://rungameid/%d", steamVRAppID)}
	if packaging == "flatpak" {
		name, args = hostCommand("xdg-open", args...) // Steam может быть и Flatpak — ссылку откроет он сам
	}
	cmd := command(name, args...)
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

// selfInstall — программа ставит себя сама (шаг «Программа» в мастере); на Windows это делает установщик,
// во Flatpak — сам Flatpak.
var selfInstall = packaging != "flatpak"

// canUninstall — есть ли в настройках «Удалить программу»: во Flatpak она снимает драйвер и удаляет данные,
// а саму программу удаляют командой flatpak uninstall.
var canUninstall = true

func installedExecutable() string { return filepath.Join(home(), ".local/bin/picotrackers") }

func iconPath() string {
	return filepath.Join(home(), ".local/share/icons/hicolor/scalable/apps", appID+".svg")
}

func menuEntryPath() string {
	return filepath.Join(home(), ".local/share/applications", appID+".desktop")
}

// launchCommand — чем запускать программу из меню и автозапуска: установленная копия, если она есть,
// иначе запущенный файл; во Flatpak — flatpak run.
func launchCommand() string {
	if packaging == "flatpak" {
		return "flatpak run " + appID
	}
	if _, err := os.Stat(installedExecutable()); err == nil {
		return installedExecutable()
	}
	exe, _ := selfExecutable()
	return exe
}

func desktopEntry(exec string) string {
	return "[Desktop Entry]\nType=Application\nName=PICO Trackers\nComment=" + T("Трекеры PICO в SteamVR") + "\n" +
		"Exec=" + exec + "\nIcon=" + appID + "\nTerminal=false\nCategories=Game;Utility;\n"
}

func programState() ProgramState {
	var s ProgramState
	self, _ := selfExecutable()
	s.Running = self == installedExecutable()
	if installed, err := os.ReadFile(installedExecutable()); err == nil {
		s.Installed = true
		current, err := os.ReadFile(self)
		s.Current = s.Running || (err == nil && bytes.Equal(installed, current))
	}
	_, err := os.Stat(menuEntryPath())
	s.InMenu = err == nil
	return s
}

// installProgram копирует запущенную программу (или AppImage) в ~/.local/bin (подменой — можно обновлять
// запущенную), ставит значок и пункт в меню приложений (или убирает его).
func installProgram(menu bool, log logFunc) error {
	exe := installedExecutable()
	if state := programState(); !state.Current {
		self, err := selfExecutable()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(self)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
			return err
		}
		log(Tf("копирую программу в %s", exe))
		if err := replaceFile(exe, data, 0o755); err != nil {
			return err
		}
	}
	writeIfChanged(iconPath(), appIcon)
	if menu {
		writeIfChanged(menuEntryPath(), desktopEntry(exe))
		log(T("пункт «PICO Trackers» добавлен в меню приложений"))
	} else {
		removeMenuEntry()
	}
	if autostartEnabled() {
		writeIfChanged(autostartPath(), desktopEntry(exe))
	}
	return nil
}

// removeProgramFiles — для удаления: копия программы, значок, пункт меню (во Flatpak их нет).
func removeProgramFiles() {
	if packaging == "flatpak" {
		return
	}
	os.Remove(installedExecutable())
	os.Remove(iconPath())
	removeMenuEntry()
}

func autostartPath() string { return filepath.Join(home(), ".config/autostart/picotrackers.desktop") }

func autostartEnabled() bool {
	_, err := os.Stat(autostartPath())
	return err == nil
}

func setAutostart(enabled bool) {
	if enabled {
		writeIfChanged(autostartPath(), desktopEntry(launchCommand()))
	} else {
		os.Remove(autostartPath())
	}
}

func removeMenuEntry() { os.Remove(menuEntryPath()) }

func markRunning() {}

// restartProgram запускает программу заново (смена языка): тот же процесс заменяется новым — так и во Flatpak
// (новый процесс остаётся в той же песочнице), и D-Bus-имя с UDP-портом освобождаются сами (сокеты CLOEXEC).
// AppImage запускается заново целиком: его программе нужны свои библиотеки (AppRun).
func restartProgram(cleanup func()) error {
	exe, err := selfExecutable()
	if err != nil {
		return err
	}
	cleanup()
	return syscall.Exec(exe, append([]string{exe}, devArgs()...), os.Environ())
}

// ---------- обновление (update.go) ----------

// isProgramAsset — программа для Linux в релизе (собирается только под x86_64): файл, AppImage или Flatpak —
// смотря чем запущена эта.
func isProgramAsset(name string) bool {
	if runtime.GOARCH != "amd64" {
		return false
	}
	switch packaging {
	case "appimage":
		return name == appImageAsset
	case "flatpak":
		return name == flatpakAsset
	}
	return name == "picotrackers-linux-x86_64"
}

// updateTargets — файлы, которые заменяет обновление: запущенный и установленная копия в ~/.local/bin.
func updateTargets() ([]string, error) {
	self, err := selfExecutable()
	if err != nil {
		return nil, err
	}
	targets := []string{self}
	if _, err := os.Stat(installedExecutable()); err == nil && installedExecutable() != self {
		targets = append(targets, installedExecutable())
	}
	return targets, nil
}

// canSelfUpdate — можно ли заменить файлы программы (нельзя, если её поставили в системную папку или через Flatpak).
func canSelfUpdate() error {
	if packaging == "flatpak" {
		return fmt.Errorf(T("программа установлена через Flatpak — скачайте %s со страницы релиза и установите его: "+
			"flatpak install --user %s"), flatpakAsset, flatpakAsset)
	}
	targets, err := updateTargets()
	if err != nil {
		return err
	}
	for _, path := range targets {
		if dir := filepath.Dir(path); syscall.Access(dir, 2 /* W_OK */) != nil {
			return fmt.Errorf(T("нет прав на запись в %s — обновите программу тем же способом, каким устанавливали"), dir)
		}
	}
	return nil
}

// installUpdate подменяет файлы программы новой версией и ставит её драйвер (SteamVR закрыт — условие
// обновления). restart запускает новую версию: она дождётся выхода этой (D-Bus и UDP-порт заняты ею).
func installUpdate(_ string, data []byte, log logFunc) (restart func() error, err error) {
	targets, err := updateTargets()
	if err != nil {
		return nil, err
	}
	for _, path := range targets {
		log(Tf("обновляю программу: %s", path))
		if err := replaceFile(path, data, 0o755); err != nil {
			return nil, err
		}
	}
	exe := targets[0]
	// Драйвер вшит в новую версию — она его и ставит. Не установлен — поставит мастер, как и раньше.
	if driverState().Installed {
		if _, err := run(exe, "install-driver"); err != nil {
			log(T("драйвер SteamVR не обновлён — обновите его в мастере установки: ") + err.Error())
		} else {
			log(T("драйвер SteamVR обновлён"))
		}
	}
	return func() error {
		cmd := command(exe, fmt.Sprintf("--wait-pid=%d", os.Getpid()), "--updated")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}, nil
}

// waitForExit ждёт выхода процесса (прошлой версии программы после обновления), не дольше timeout.
func waitForExit(pid int, timeout time.Duration) {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return
		}
		// «pid (имя) состояние …»: Z — процесс завершился, но родитель ещё не забрал код выхода.
		if i := bytes.LastIndexByte(stat, ')'); i >= 0 && bytes.HasPrefix(stat[i+1:], []byte(" Z")) {
			return
		}
	}
}
