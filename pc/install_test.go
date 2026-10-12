package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Установка драйвера во временный HOME: файлы на месте, зарегистрирован, старая копия picotrackers
// отключена, чужие драйверы не тронуты. SteamVR в HOME нет — проверяется правка openvrpaths.vrpath напрямую.
func TestInstallDriverReplacesOldRegistration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	old := filepath.Join(home, "src", "driver", "build", "picotrackers")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "driver.vrdrivermanifest"), []byte(`{ "name": "picotrackers" }`), 0o644)
	other := filepath.Join(home, "other-driver")
	os.MkdirAll(other, 0o755)

	paths := map[string]any{
		"jsonid":           "vrpathreg",
		"version":          1,
		"runtime":          []string{filepath.Join(home, "no-steamvr")},
		"external_drivers": []string{old, other},
	}
	data, _ := json.Marshal(paths)
	os.MkdirAll(filepath.Dir(openVRPathsFile()), 0o755)
	os.WriteFile(openVRPathsFile(), data, 0o644)

	if driverState().Ready() {
		t.Fatal("до установки драйвер не должен считаться готовым")
	}
	if err := installDriver(func(string) {}); err != nil {
		t.Fatal(err)
	}
	state := driverState()
	if !state.Ready() {
		t.Fatalf("после установки: %+v", state)
	}
	got, _ := readOpenVRPaths()
	drivers := got.list("external_drivers")
	if contains(drivers, old) || !contains(drivers, other) || !contains(drivers, driverInstallDir()) {
		t.Fatalf("external_drivers: %v", drivers)
	}
	binary := filepath.Join(driverInstallDir(), driverBinarySubdir)
	before, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}

	// Повторная установка (обновление при запущенном SteamVR): библиотека подменяется новым файлом,
	// а не перезаписывается на месте; временных файлов не остаётся.
	if err := installDriver(func(string) {}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("драйвер перезаписан на месте, а не подменён")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(binary), ".*")); len(leftovers) > 0 {
		t.Fatalf("временные файлы: %v", leftovers)
	}
}

// Удаление (деинсталлятор): регистрация снята, файлы драйвера и настройки удалены, чужие драйверы на месте.
func TestUninstallRemovesDriver(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	other := filepath.Join(home, "other-driver")
	os.MkdirAll(other, 0o755)
	data, _ := json.Marshal(map[string]any{
		"runtime":          []string{filepath.Join(home, "no-steamvr")},
		"external_drivers": []string{other},
	})
	os.MkdirAll(filepath.Dir(openVRPathsFile()), 0o755)
	os.WriteFile(openVRPathsFile(), data, 0o644)

	if err := installDriver(func(string) {}); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig()
	cfg.save()
	if err := uninstall(func(string) {}); err != nil {
		t.Fatal(err)
	}
	got, _ := readOpenVRPaths()
	if drivers := got.list("external_drivers"); len(drivers) != 1 || drivers[0] != other {
		t.Fatalf("external_drivers: %v", drivers)
	}
	for _, dir := range []string{dataDir(), configDir()} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s не удалён: %v", dir, err)
		}
	}
}

// Самоустановка (Linux, шаг «Программа»): копия в ~/.local/bin, значок, пункт меню; удаление убирает всё.
func TestInstallProgram(t *testing.T) {
	if !selfInstall {
		t.Skip("на этой платформе программу ставит установщик")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	if state := programState(); state.Installed || state.InMenu {
		t.Fatalf("до установки: %+v", state)
	}
	if err := installProgram(true, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if state := programState(); !state.Installed || !state.Current || state.Running || !state.InMenu {
		t.Fatalf("после установки: %+v", state)
	}
	entry, _ := os.ReadFile(menuEntryPath())
	if !strings.Contains(string(entry), "Exec="+installedExecutable()+"\n") || !strings.Contains(string(entry), "Icon="+appID+"\n") {
		t.Fatalf("пункт меню:\n%s", entry)
	}
	if _, err := os.Stat(iconPath()); err != nil {
		t.Fatal(err)
	}

	// Без пункта меню: пункт убирается, программа остаётся.
	if err := installProgram(false, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if state := programState(); !state.Installed || state.InMenu {
		t.Fatalf("без пункта меню: %+v", state)
	}

	if err := uninstall(func(string) {}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{installedExecutable(), iconPath(), menuEntryPath()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s не удалён: %v", path, err)
		}
	}
}

// --dev: удаление снимает драйвер и стирает данные, но установленную программу, пункт меню и автозапуск не трогает.
func TestUninstallDevKeepsProgram(t *testing.T) {
	if !selfInstall {
		t.Skip("на этой платформе программу ставит установщик")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := installProgram(true, func(string) {}); err != nil {
		t.Fatal(err)
	}
	setAutostart(true)
	defer func(was bool) { devMode = was }(devMode)
	devMode = true
	if err := uninstall(func(string) {}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{installedExecutable(), iconPath(), menuEntryPath(), autostartPath()} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("режим --dev удалил %s: %v", path, err)
		}
	}
	if _, err := os.Stat(dataDir()); !os.IsNotExist(err) {
		t.Fatalf("%s не удалён: %v", dataDir(), err)
	}
}
