// Особенности Windows: пути, SteamVR, автозапуск через реестр, команды без окна консоли.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	adbName            = "adb.exe"
	platformToolsURL   = "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
	driverBinarySubdir = `bin\win64\driver_picotrackers.dll`
	autostartKey       = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	autostartValue     = "PICO Trackers"
	appMutex           = "PICOtrackers.Control" // AppMutex в windows/installer.iss
)

func localAppData() string {
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		return dir
	}
	return filepath.Join(home(), "AppData", "Local")
}

func dataDir() string { return filepath.Join(localAppData(), "PICOtrackers") }

func configDir() string { return dataDir() }

func openVRPathsFile() string { return filepath.Join(localAppData(), "openvr", "openvrpaths.vrpath") }

func vrpathregPath(runtime string) string { return filepath.Join(runtime, `bin\win64\vrpathreg.exe`) }

// hideConsole — чтобы adb, reg и powershell не мигали окнами консоли.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// steamVRStartedAt — время запуска vrserver.exe; ok=false, если SteamVR не запущен.
func steamVRStartedAt() (time.Time, bool) {
	out, err := run("powershell", "-NoProfile", "-Command",
		"(Get-Process vrserver -ErrorAction SilentlyContinue | Select-Object -First 1).StartTime.ToUniversalTime().ToString('o')")
	out = strings.TrimSpace(out)
	if err != nil || out == "" {
		return time.Time{}, false
	}
	started, err := time.Parse(time.RFC3339Nano, out)
	if err != nil {
		return time.Now(), true
	}
	return started, true
}

func launchSteamVR() {
	cmd := exec.Command("cmd", "/c", "start", "", fmt.Sprintf("steam://rungameid/%d", steamVRAppID))
	hideConsole(cmd)
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

func autostartEnabled() bool {
	_, err := run("reg", "query", autostartKey, "/v", autostartValue)
	return err == nil
}

func setAutostart(enabled bool) {
	if enabled {
		exe, _ := os.Executable()
		run("reg", "add", autostartKey, "/v", autostartValue, "/t", "REG_SZ", "/d", `"`+exe+`"`, "/f")
	} else {
		run("reg", "delete", autostartKey, "/v", autostartValue, "/f")
	}
}

// selfInstall — на Windows программу ставит установщик (windows/installer.iss), шага «Программа» в мастере нет.
const selfInstall = false

func installedExecutable() string { return "" }

func programState() ProgramState { return ProgramState{} }

func installProgram(bool, logFunc) error { return nil }

func removeProgramFiles() {}

func removeMenuEntry() {}

// markRunning создаёт именованный mutex: по нему установщик видит, что программа запущена, и просит
// закрыть её перед обновлением или удалением. Mutex живёт до выхода из процесса.
func markRunning() {
	name, _ := syscall.UTF16PtrFromString(appMutex)
	syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
}
