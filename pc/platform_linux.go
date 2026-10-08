// Особенности Linux: пути, SteamVR, автозапуск через .desktop, установка самой программы.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
