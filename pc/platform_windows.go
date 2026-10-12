// Особенности Windows: пути, SteamVR, автозапуск через реестр, команды без окна консоли, обновление установщиком.
package main

import (
	"errors"
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

func hostCommand(name string, args ...string) (string, []string) { return name, args }

func restoreHostEnv() {}

func initGTKFromEnv() {}

// systemLanguage — язык интерфейса Windows: русский или английский.
func systemLanguage() string {
	id, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	if id&0x3ff == 0x19 { // LANG_RUSSIAN
		return languageRussian
	}
	return languageEnglish
}

// restartProgram запускает программу заново (смена языка): новая дождётся выхода этой (UDP-порт занят ею).
func restartProgram(cleanup func()) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := command(exe, append([]string{fmt.Sprintf("--wait-pid=%d", os.Getpid())}, devArgs()...)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.Process.Release()
	cleanup()
	os.Exit(0)
	return nil
}

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
	cmd := command("cmd", "/c", "start", "", fmt.Sprintf("steam://rungameid/%d", steamVRAppID))
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
var selfInstall = false

// canUninstall — удаляют через «Параметры → Приложения» (деинсталлятор).
var canUninstall = false

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

// ---------- обновление (update.go) ----------

// isProgramAsset — установщик в релизе: обновление ставит он, как и первую установку.
func isProgramAsset(name string) bool {
	return strings.HasPrefix(name, "PICOtrackers-setup-") && strings.HasSuffix(name, ".exe")
}

// canSelfUpdate — обновляется только программа, поставленная установщиком (рядом его деинсталлятор),
// не переносная из zip: установщик поставил бы вторую копию в другую папку.
func canSelfUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "unins000.exe")); err != nil {
		return errors.New(T("это переносная версия (из zip) — скачайте новую со страницы релиза"))
	}
	return nil
}

// installUpdate сохраняет установщик новой версии. restart запускает его после выхода программы: она держит
// свои файлы и mutex PICOtrackers.Control (AppMutex), из-за которого тихая установка прервалась бы.
// Ждёт выхода PowerShell. Установщик в режиме /UPDATE ставит драйвер и снова запускает программу.
func installUpdate(name string, data []byte, log logFunc) (restart func() error, err error) {
	os.RemoveAll(updateDir())
	if err := os.MkdirAll(updateDir(), 0o755); err != nil {
		return nil, err
	}
	setup := filepath.Join(updateDir(), name)
	if err := os.WriteFile(setup, data, 0o755); err != nil {
		return nil, err
	}
	log(Tf("установщик новой версии: %s", setup))
	return func() error {
		script := fmt.Sprintf("Wait-Process -Id %d -Timeout 60 -ErrorAction SilentlyContinue; "+
			"Start-Process -FilePath '%s' -ArgumentList '/SILENT','/SUPPRESSMSGBOXES','/NORESTART','/UPDATE'",
			os.Getpid(), strings.ReplaceAll(setup, "'", "''"))
		cmd := command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}, nil
}

// waitForExit ждёт выхода процесса, не дольше timeout (--wait-pid; на Windows программу перезапускает установщик).
func waitForExit(pid int, timeout time.Duration) {
	const synchronize = 0x00100000
	handle, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(handle)
	syscall.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
}
