// Общая логика: adb и приложение на шлеме, установка драйвера SteamVR, состояние SteamVR.
// Платформенные различия — в platform_linux.go и platform_windows.go.
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	headsetPackage = "dev.picotrackers.headset"
	headsetService = headsetPackage + "/.TrackingService"
	dataPort       = 7788 // драйвер: данные со шлема и команды программы (common/protocol.h)
	appPort        = 7790 // программа: состояние от драйвера
	adbWifiPort    = 5555
	steamVRAppID   = 250820
	driverName     = "picotrackers"
	appVersion     = "1.6"
	githubRepo     = "Mr1rbis/PICOtrackers" // релизы: обновление программы (update.go)
)

type tracker struct {
	Name, Label string
	HasRole     bool // есть ли роль трекера в SteamVR
}

// Суставы скелета PICO в порядке драйвера (driver/src/driver.cpp, kTrackerSpecs).
var trackers = []tracker{
	{"waist", "Пояс", true},
	{"chest", "Грудь", true},
	{"left_foot", "Левая стопа", true},
	{"right_foot", "Правая стопа", true},
	{"left_knee", "Левое колено", true},
	{"right_knee", "Правое колено", true},
	{"left_elbow", "Левый локоть", true},
	{"right_elbow", "Правый локоть", true},
	{"left_shoulder", "Левое плечо", true},
	{"right_shoulder", "Правое плечо", true},
	{"left_hip", "Левый тазобедренный сустав", false},
	{"right_hip", "Правый тазобедренный сустав", false},
	{"spine1", "Поясница", false},
	{"spine2", "Середина спины", false},
	{"left_toes", "Левый носок", false},
	{"right_toes", "Правый носок", false},
	{"neck", "Шея", false},
	{"left_collar", "Левая ключица", false},
	{"right_collar", "Правая ключица", false},
	{"head", "Голова", false},
	{"left_wrist", "Левое запястье", false},
	{"right_wrist", "Правое запястье", false},
	{"left_hand", "Левая кисть", false},
	{"right_hand", "Правая кисть", false},
}

var legsTrackers = []string{"waist", "left_foot", "right_foot"}

func mainTrackers() (names []string) {
	for _, t := range trackers {
		if t.HasRole {
			names = append(names, t.Name)
		}
	}
	return
}

func allTrackers() (names []string) {
	for _, t := range trackers {
		names = append(names, t.Name)
	}
	return
}

type logFunc func(string)

// ---------- настройки ----------

func home() string       { h, _ := os.UserHomeDir(); return h }
func configPath() string { return filepath.Join(configDir(), "config.json") }

type Config struct {
	AdbWifi    string    `json:"adb_wifi,omitempty"`
	Tracking   *bool     `json:"tracking,omitempty"` // nil — включён
	Trackers   *[]string `json:"trackers,omitempty"` // nil — ещё не известен, берём у драйвера
	Rate       int       `json:"rate,omitempty"`     // снимков/с со шлема; 0 — по умолчанию
	WizardDone bool      `json:"wizard_done,omitempty"`
	ServiceOff bool      `json:"service_off,omitempty"` // сервис на шлеме выключен кнопкой — сами его не включаем
}

const (
	defaultRate = 100
	minRate     = 10
	maxRate     = 250
)

func (c *Config) rate() int {
	if c.Rate == 0 {
		return defaultRate
	}
	return c.Rate
}

func loadConfig() *Config {
	var c Config
	if data, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(data, &c)
	}
	return &c
}

func (c *Config) save() {
	os.MkdirAll(filepath.Dir(configPath()), 0o755)
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(configPath(), data, 0o644)
}

func (c *Config) trackingEnabled() bool { return c.Tracking == nil || *c.Tracking }

// writeIfChanged пишет файл, только если содержимое отличается.
func writeIfChanged(path, content string) {
	if old, err := os.ReadFile(path); err == nil && string(old) == content {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(content), 0o644)
}

// ---------- запуск команд ----------

// run запускает команду и возвращает stdout; при ненулевом коде — ошибку с выводом.
func run(name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	hideConsole(cmd)
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("команда не удалась: %s %s\n%s%s", filepath.Base(name),
			strings.Join(args, " "), stdout.String(), stderr.String())
	}
	return stdout.String(), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ---------- adb ----------

// adbPath — adb из PATH или скачанный программой; "" — adb нет.
func adbPath() string {
	if path, err := exec.LookPath(adbName); err == nil {
		return path
	}
	path := filepath.Join(dataDir(), "platform-tools", adbName)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func adb(args ...string) (string, error) {
	path := adbPath()
	if path == "" {
		return "", errors.New("не найден adb")
	}
	return run(path, args...)
}

// downloadPlatformTools скачивает Android platform-tools (adb) от Google в папку программы.
func downloadPlatformTools(log logFunc) error {
	log("скачиваю Android platform-tools (adb)…")
	response, err := http.Get(platformToolsURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("загрузка adb: %s", response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range archive.File {
		target := filepath.Join(dataDir(), filepath.FromSlash(file.Name))
		if !strings.HasPrefix(target, filepath.Join(dataDir(), "platform-tools")) {
			continue
		}
		if file.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		src, err := file.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, content, file.Mode()|0o600); err != nil {
			return err
		}
	}
	log("adb установлен: " + filepath.Join(dataDir(), "platform-tools"))
	return nil
}

type Headset struct{ Serial string }

func (h *Headset) OverWifi() bool { return strings.Contains(h.Serial, ":") }

func (h *Headset) Adb(args ...string) (string, error) {
	return adb(append([]string{"-s", h.Serial}, args...)...)
}

func (h *Headset) Shell(command string) (string, error) {
	out, err := h.Adb("shell", command)
	return strings.TrimSpace(out), err
}

var inetRe = regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+)`)

func (h *Headset) WifiIP() string {
	out, _ := h.Shell("ip -4 -o addr show wlan0")
	if m := inetRe.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// adbDevices — устройства adb и их состояние: device, unauthorized, offline…
func adbDevices() map[string]string {
	out, _ := adb("devices")
	devices := map[string]string{}
	for _, line := range strings.Split(out, "\n")[1:] {
		if serial, state, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
			devices[serial] = state
		}
	}
	return devices
}

// usbHeadset — шлем, подключённый кабелем: serial и состояние ("" — не найден).
func usbHeadset() (serial, state string) {
	for s, st := range adbDevices() {
		if !strings.Contains(s, ":") && (serial == "" || st == "device") {
			serial, state = s, st
		}
	}
	return
}

// connectHeadset находит шлем без установки чего-либо: сохранённый Wi-Fi адрес, иначе USB.
func connectHeadset(cfg *Config) (*Headset, error) {
	if adbPath() == "" {
		return nil, errors.New("не найден adb — установите его в мастере установки")
	}
	devices := adbDevices()
	if cfg.AdbWifi != "" && devices[cfg.AdbWifi] != "device" {
		adb("connect", cfg.AdbWifi)
		devices = adbDevices()
	}
	if cfg.AdbWifi != "" && devices[cfg.AdbWifi] == "device" {
		return &Headset{cfg.AdbWifi}, nil
	}
	if serial, state := usbHeadset(); state == "device" {
		return &Headset{serial}, nil
	}
	return nil, errors.New("шлем не найден по adb")
}

// switchToWifi: adb tcpip — после этого кабель можно отключить (до перезагрузки шлема).
func switchToWifi(h *Headset, cfg *Config, log logFunc) *Headset {
	ip := h.WifiIP()
	if ip == "" {
		return h
	}
	address := fmt.Sprintf("%s:%d", ip, adbWifiPort)
	h.Adb("tcpip", strconv.Itoa(adbWifiPort))
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		adb("connect", address)
		if adbDevices()[address] == "device" {
			cfg.AdbWifi = address
			cfg.save()
			log("adb по Wi-Fi: " + address + " — USB-кабель можно отключить")
			return &Headset{address}
		}
	}
	log("не удалось переключить adb на Wi-Fi, работаю по USB")
	return h
}

type AppState int

const (
	AppMissing AppState = iota
	AppOutdated
	AppInstalled
)

// installedAPKHash — sha256 APK приложения на шлеме; "" — приложение не установлено.
func installedAPKHash(h *Headset) string {
	path, _ := h.Shell("pm path " + headsetPackage)
	path = strings.TrimPrefix(path, "package:")
	if path == "" {
		return ""
	}
	out, _ := h.Shell("sha256sum " + path)
	if fields := strings.Fields(out); len(fields) > 0 {
		return fields[0]
	}
	return "?" // установлено, но хеш не прочитать — считаем, что версия другая
}

// appState сравнивает установленное на шлеме приложение со вшитым в программу.
func appState(h *Headset) AppState {
	switch hash := installedAPKHash(h); hash {
	case "":
		return AppMissing
	case apkHash():
		return AppInstalled
	}
	return AppOutdated
}

// installApp ставит на шлем приложение, вшитое в программу.
func installApp(h *Headset, log logFunc) error {
	data, err := apkData()
	if err != nil {
		return errors.New("в программу не вшито приложение для шлема — пересоберите её через build.sh")
	}
	return installAPK(h, data, log)
}

// installAPK ставит на шлем приложение из data (вшитое или скачанное с обновлением).
func installAPK(h *Headset, data []byte, log logFunc) error {
	file, err := os.CreateTemp("", "PicoTrackers-*.apk")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	file.Write(data)
	file.Close()
	log("устанавливаю приложение на шлем…")
	if _, err := h.Adb("install", "-r", file.Name()); err != nil {
		// На шлеме приложение, подписанное другим ключом (собрано у себя, а программа — из релиза на GitHub,
		// или наоборот): поверх его не обновить, только удалить и поставить заново. Хранимых настроек у него нет.
		if !strings.Contains(err.Error(), "INSTALL_FAILED_UPDATE_INCOMPATIBLE") {
			return err
		}
		log("приложение на шлеме подписано другим ключом — переустанавливаю")
		if _, err := h.Adb("uninstall", headsetPackage); err != nil {
			return err
		}
		if _, err := h.Adb("install", file.Name()); err != nil {
			return err
		}
	}
	log("приложение на шлеме установлено")
	return nil
}

// startService запускает сервис в авторежиме: он ждёт просьбу драйвера SteamVR и сам включает трекинг.
// Если сервис выключили кнопкой на шлеме, приложение (1.6+) не запустится — выбор пользователя важнее.
func startService(h *Headset) error {
	_, err := h.Shell("am start-foreground-service -n " + headsetService)
	return err
}

func stopService(h *Headset) {
	h.Shell("am stopservice -n " + headsetService) // am возвращает 255 даже при успехе
}

// setServiceEnabled — выбор пользователя «сервис включён/выключен» (кнопка в программе, picotrackers stop).
// Приложение на шлеме (1.6+) его запоминает: выключенный сервис не запускается сам ни при включении шлема,
// ни при открытии приложения. Старое приложение «enabled» не знает — его сервис останавливаем как раньше.
func setServiceEnabled(h *Headset, enabled bool) error {
	if _, err := h.Shell(fmt.Sprintf("am start-foreground-service -n %s --ez enabled %t", headsetService, enabled)); err != nil {
		return err
	}
	if enabled {
		return nil
	}
	for i := 0; i < 10; i++ { // новое приложение останавливается само; старое — нет
		time.Sleep(200 * time.Millisecond)
		if running, err := serviceRunning(h); err == nil && !running {
			return nil
		}
	}
	stopService(h)
	return nil
}

// serviceRunning — запущен ли сервис на шлеме: dumpsys выводит «ServiceRecord{… <пакет>/.TrackingService}»,
// а если сервиса нет — «(nothing)».
func serviceRunning(h *Headset) (bool, error) {
	out, err := h.Shell("dumpsys activity services " + headsetService)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, headsetService), nil
}

// LogcatFollower следит за логом приложения на шлеме: строки «stats …» и «listener: …».
type LogcatFollower struct {
	headset      *Headset
	OnStats      func(map[string]string)
	OnListener   func(string)
	OnMessage    func(string)
	OnConnection func(bool)

	stop chan struct{}
	mu   sync.Mutex
	cmd  *exec.Cmd
}

var statsRe = regexp.MustCompile(`(\w+)=(\S+)`)

func (f *LogcatFollower) Start() {
	f.stop = make(chan struct{})
	go f.run()
}

func (f *LogcatFollower) Stop() {
	close(f.stop)
	f.mu.Lock()
	if f.cmd != nil && f.cmd.Process != nil {
		f.cmd.Process.Kill()
	}
	f.mu.Unlock()
}

func (f *LogcatFollower) stopped() bool {
	select {
	case <-f.stop:
		return true
	default:
		return false
	}
}

func (f *LogcatFollower) run() {
	for !f.stopped() {
		cmd := exec.Command(adbPath(), "-s", f.headset.Serial, "logcat", "-T", "1", "-v", "brief",
			"PicoTrackers:I", "*:S")
		hideConsole(cmd)
		out, err := cmd.StdoutPipe()
		if err == nil && cmd.Start() == nil {
			f.mu.Lock()
			f.cmd = cmd
			f.mu.Unlock()
			f.OnConnection(true)
			scanner := bufio.NewScanner(out)
			for scanner.Scan() {
				line := strings.TrimRight(scanner.Text(), "\r")
				switch {
				case strings.Contains(line, " stats "):
					stats := map[string]string{}
					for _, m := range statsRe.FindAllStringSubmatch(strings.SplitN(line, " stats ", 2)[1], -1) {
						stats[m[1]] = m[2]
					}
					f.OnStats(stats)
				case strings.Contains(line, "listener: "):
					f.OnListener(strings.TrimSpace(strings.SplitN(line, "listener: ", 2)[1]))
				case strings.HasPrefix(line, "W/PicoTrackers"), strings.HasPrefix(line, "E/PicoTrackers"):
					if _, msg, ok := strings.Cut(line, ":"); ok {
						f.OnMessage(strings.TrimSpace(msg))
					}
				}
			}
			cmd.Process.Kill()
			cmd.Wait()
		}
		if f.stopped() {
			return
		}
		f.OnConnection(false)
		for {
			select {
			case <-f.stop:
				return
			case <-time.After(3 * time.Second):
			}
			if adbDevices()[f.headset.Serial] == "device" {
				break
			}
			if f.headset.OverWifi() {
				adb("connect", f.headset.Serial)
			}
		}
	}
}

// ---------- драйвер SteamVR ----------

func driverInstallDir() string { return filepath.Join(dataDir(), "driver", driverName) }

type openVRPaths map[string]any

func readOpenVRPaths() (openVRPaths, error) {
	data, err := os.ReadFile(openVRPathsFile())
	if err != nil {
		return nil, fmt.Errorf("SteamVR не найден (нет %s) — установите SteamVR и запустите его хотя бы раз",
			openVRPathsFile())
	}
	var p openVRPaths
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return p, nil
}

func (p openVRPaths) list(key string) (values []string) {
	if items, ok := p[key].([]any); ok {
		for _, item := range items {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
	}
	return
}

// DriverState — что с драйвером: установлен ли вшитой версии и зарегистрирован ли в SteamVR.
type DriverState struct {
	SteamVRFound bool
	Installed    bool // файлы на месте
	UpToDate     bool // версия совпадает со вшитой
	Registered   bool // папка в external_drivers
	Others       []string
}

func (s DriverState) Ready() bool {
	return s.Installed && s.UpToDate && s.Registered && len(s.Others) == 0
}

func driverVersionFile() string { return filepath.Join(driverInstallDir(), "version.txt") }

// isOurDriver — папка зарегистрированного драйвера с именем picotrackers (старые установки, сборки из исходников).
func isOurDriver(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "driver.vrdrivermanifest"))
	return err == nil && strings.Contains(string(data), `"name": "`+driverName+`"`)
}

func driverState() DriverState {
	var s DriverState
	paths, err := readOpenVRPaths()
	s.SteamVRFound = err == nil
	if _, err := os.Stat(filepath.Join(driverInstallDir(), driverBinarySubdir)); err == nil {
		s.Installed = true
	}
	if version, err := os.ReadFile(driverVersionFile()); err == nil {
		s.UpToDate = strings.TrimSpace(string(version)) == driverAssetsVersion()
	}
	for _, dir := range paths.list("external_drivers") {
		switch {
		case filepath.Clean(dir) == filepath.Clean(driverInstallDir()):
			s.Registered = true
		case isOurDriver(dir):
			s.Others = append(s.Others, dir)
		}
	}
	return s
}

// installDriver раскладывает вшитый драйвер в папку программы и регистрирует его в SteamVR
// (убирая регистрации других копий picotrackers).
func installDriver(log logFunc) error {
	if _, err := readOpenVRPaths(); err != nil {
		return err
	}
	if _, running := steamVRStartedAt(); running && runtime.GOOS == "windows" && driverState().Installed {
		return errSteamVRRunning
	}

	log("копирую драйвер в " + driverInstallDir())
	err := fs.WalkDir(assets, assetsDriver, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(driverInstallDir(), filepath.FromSlash(strings.TrimPrefix(path, assetsDriver)))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := assets.ReadFile(path)
		if err != nil {
			return err
		}
		return replaceFile(target, data, 0o755)
	})
	if err != nil {
		return fmt.Errorf("копирование драйвера: %w", err)
	}
	if err := os.WriteFile(driverVersionFile(), []byte(driverAssetsVersion()+"\n"), 0o644); err != nil {
		return err
	}

	if err := registerDriver(driverInstallDir(), log); err != nil {
		return err
	}
	log("драйвер зарегистрирован в SteamVR")
	return nil
}

// registerDriver оставляет в SteamVR из всех копий picotrackers только keep (пусто — ни одной):
// лишние регистрации снимает через vrpathreg; если он не сработал (нет файла, другая версия) —
// правит openvrpaths.vrpath сам.
func registerDriver(keep string, log logFunc) error {
	paths, err := readOpenVRPaths()
	if err != nil {
		return err
	}
	vrpathreg := ""
	if runtimes := paths.list("runtime"); len(runtimes) > 0 {
		vrpathreg = vrpathregPath(runtimes[0])
	}
	isKeep := func(dir string) bool { return keep != "" && filepath.Clean(dir) == filepath.Clean(keep) }
	stale := func(dir string) bool {
		return !isKeep(dir) && (isOurDriver(dir) || filepath.Clean(dir) == filepath.Clean(driverInstallDir()))
	}

	registered := false
	for _, dir := range paths.list("external_drivers") {
		registered = registered || isKeep(dir)
		if stale(dir) {
			log("снимаю регистрацию драйвера: " + dir)
			if vrpathreg != "" {
				run(vrpathreg, "removedriver", dir)
			}
		}
	}
	if keep != "" && !registered && vrpathreg != "" {
		run(vrpathreg, "adddriver", keep)
	}

	if paths, err = readOpenVRPaths(); err != nil {
		return err
	}
	drivers, changed, registered := []string{}, false, false
	for _, dir := range paths.list("external_drivers") {
		if stale(dir) {
			changed = true
			continue
		}
		registered = registered || isKeep(dir)
		drivers = append(drivers, dir)
	}
	if keep != "" && !registered {
		drivers, changed = append(drivers, keep), true
	}
	if !changed {
		return nil
	}
	paths["external_drivers"] = drivers
	data, _ := json.MarshalIndent(paths, "", "\t")
	return os.WriteFile(openVRPathsFile(), data, 0o644)
}

// ProgramState — установлена ли сама программа (Linux: копия в ~/.local/bin и пункт меню).
type ProgramState struct {
	Installed bool // копия на месте
	Current   bool // и совпадает с запущенной
	Running   bool // запущена именно установленная копия
	InMenu    bool // есть пункт в меню приложений
}

// errSteamVRRunning — на Windows загруженную SteamVR DLL драйвера нельзя ни заменить, ни удалить.
var errSteamVRRunning = errors.New("закройте SteamVR: Windows не даёт заменить или удалить загруженный драйвер")

// uninstall — для деинсталлятора и кнопки «Удалить программу» (Linux): снимает драйвер с регистрации
// в SteamVR и удаляет его, выключает автозапуск, удаляет программу из меню и ~/.local/bin, её данные
// и настройки. Приложение на шлеме не трогает.
func uninstall(log logFunc) error {
	if _, running := steamVRStartedAt(); running && runtime.GOOS == "windows" {
		return errSteamVRRunning
	}
	if _, err := readOpenVRPaths(); err == nil {
		if err := registerDriver("", log); err != nil {
			return err
		}
	}
	setAutostart(false)
	removeProgramFiles()
	// Свой adb-сервер держит файлы platform-tools открытыми — на Windows их иначе не удалить.
	if adb := adbPath(); strings.HasPrefix(adb, dataDir()) {
		run(adb, "kill-server")
	}
	log("удаляю " + dataDir())
	if err := os.RemoveAll(dataDir()); err != nil {
		return err
	}
	return os.RemoveAll(configDir())
}

// replaceFile пишет файл рядом и подменяет старый переименованием: запущенный SteamVR держит загруженную
// .so — перезапись на месте поменяла бы её код у него под ногами (и уронила бы vrserver).
func replaceFile(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), perm)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// steamVRNeedsRestart — SteamVR запущен раньше, чем установлен текущий драйвер.
func steamVRNeedsRestart() bool {
	started, running := steamVRStartedAt()
	if !running {
		return false
	}
	info, err := os.Stat(driverVersionFile())
	return err == nil && info.ModTime().After(started)
}
