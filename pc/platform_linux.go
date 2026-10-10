// Особенности Linux: пути, SteamVR, автозапуск через .desktop, установка и обновление самой программы.
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
)

const (
	adbName            = "adb"
	platformToolsURL   = "https://dl.google.com/android/repository/platform-tools-latest-linux.zip"
	driverBinarySubdir = "bin/linux64/driver_picotrackers.so"
)

func dataDir() string { return filepath.Join(home(), ".local/share/picotrackers") }

func configDir() string { return filepath.Join(home(), ".config/picotrackers") }

func openVRPathsFile() string { return filepath.Join(home(), ".config/openvr/openvrpaths.vrpath") }

func vrpathregPath(runtime string) string { return filepath.Join(runtime, "bin/vrpathreg.sh") }

func hideConsole(*exec.Cmd) {}

// steamVRStartedAt — время запуска vrserver; ok=false, если SteamVR не запущен.
func steamVRStartedAt() (time.Time, bool) {
	out, err := run("pgrep", "-x", "vrserver")
	pids := strings.Fields(out)
	if err != nil || len(pids) == 0 {
		return time.Time{}, false
	}
	elapsed, err := run("ps", "-o", "etimes=", "-p", pids[0])
	seconds, convErr := strconv.Atoi(strings.TrimSpace(elapsed))
	if err != nil || convErr != nil {
		return time.Now(), true
	}
	return time.Now().Add(-time.Duration(seconds) * time.Second), true
}

func launchSteamVR() {
	cmd := exec.Command("steam", fmt.Sprintf("steam://rungameid/%d", steamVRAppID))
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

// selfInstall — программа ставит себя сама (шаг «Программа» в мастере); на Windows это делает установщик.
const selfInstall = true

func installedExecutable() string { return filepath.Join(home(), ".local/bin/picotrackers") }

func iconPath() string {
	return filepath.Join(home(), ".local/share/icons/hicolor/scalable/apps", appID+".svg")
}

func menuEntryPath() string {
	return filepath.Join(home(), ".local/share/applications", appID+".desktop")
}

// programExecutable — установленная копия, если она есть, иначе запущенный файл.
func programExecutable() string {
	if _, err := os.Stat(installedExecutable()); err == nil {
		return installedExecutable()
	}
	exe, _ := os.Executable()
	return exe
}

func desktopEntry(exe string) string {
	return "[Desktop Entry]\nType=Application\nName=PICO Trackers\nComment=Трекеры PICO в SteamVR\n" +
		"Exec=" + exe + "\nIcon=" + appID + "\nTerminal=false\nCategories=Game;Utility;\n"
}

func programState() ProgramState {
	var s ProgramState
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
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

// installProgram копирует запущенную программу в ~/.local/bin (подменой — можно обновлять запущенную),
// ставит значок и пункт в меню приложений (или убирает его).
func installProgram(menu bool, log logFunc) error {
	exe := installedExecutable()
	if state := programState(); !state.Current {
		self, err := os.Executable()
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
		log("копирую программу в " + exe)
		if err := replaceFile(exe, data, 0o755); err != nil {
			return err
		}
	}
	writeIfChanged(iconPath(), appIcon)
	if menu {
		writeIfChanged(menuEntryPath(), desktopEntry(exe))
		log("пункт «PICO Trackers» добавлен в меню приложений")
	} else {
		removeMenuEntry()
	}
	if autostartEnabled() {
		writeIfChanged(autostartPath(), desktopEntry(exe))
	}
	return nil
}

// removeProgramFiles — для удаления: копия программы, значок, пункт меню.
func removeProgramFiles() {
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
		writeIfChanged(autostartPath(), desktopEntry(programExecutable()))
	} else {
		os.Remove(autostartPath())
	}
}

func removeMenuEntry() { os.Remove(menuEntryPath()) }

func markRunning() {}

// ---------- обновление (update.go) ----------

// isProgramAsset — программа для Linux в релизе (собирается только под x86_64).
func isProgramAsset(name string) bool {
	return runtime.GOARCH == "amd64" && name == "picotrackers-linux-x86_64"
}

// updateTargets — файлы, которые заменяет обновление: запущенный и установленная копия в ~/.local/bin.
func updateTargets() ([]string, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	targets := []string{self}
	if _, err := os.Stat(installedExecutable()); err == nil && installedExecutable() != self {
		targets = append(targets, installedExecutable())
	}
	return targets, nil
}

// canSelfUpdate — можно ли заменить файлы программы (нельзя, если её поставили в системную папку).
func canSelfUpdate() error {
	targets, err := updateTargets()
	if err != nil {
		return err
	}
	for _, path := range targets {
		if dir := filepath.Dir(path); syscall.Access(dir, 2 /* W_OK */) != nil {
			return fmt.Errorf("нет прав на запись в %s — обновите программу тем же способом, каким устанавливали", dir)
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
		log("обновляю программу: " + path)
		if err := replaceFile(path, data, 0o755); err != nil {
			return nil, err
		}
	}
	exe := targets[0]
	// Драйвер вшит в новую версию — она его и ставит. Не установлен — поставит мастер, как и раньше.
	if driverState().Installed {
		if _, err := run(exe, "install-driver"); err != nil {
			log("драйвер SteamVR не обновлён — обновите его в мастере установки: " + err.Error())
		} else {
			log("драйвер SteamVR обновлён")
		}
	}
	return func() error {
		cmd := exec.Command(exe, fmt.Sprintf("--wait-pid=%d", os.Getpid()), "--updated")
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
