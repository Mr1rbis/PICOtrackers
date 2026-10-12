// Фоновая работа программы: установка, связь с драйвером SteamVR (UDP 127.0.0.1) и шлемом (adb).
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	driverTimeout       = 3 * time.Second // без состояния от драйвера — связи нет
	headsetStatsTimeout = 3 * time.Second
	headsetRetry        = 5 * time.Second
	calibrationTimeout  = 3 * time.Second // шлем отвечает на просьбу откалибровать трекеры в пакете состояния (раз в 1 с)
	lowBattery          = 20              // %: заряд трекера PICO ниже — предупреждение
)

// DriverStatus — JSON, который драйвер шлёт раз в 0.5 с (driver/src/driver.cpp, status_json).
type DriverStatus struct {
	Type          string `json:"type"`
	App           bool   `json:"app"`
	Tracking      bool   `json:"tracking"`
	Data          bool   `json:"data"`
	HMD           bool   `json:"hmd"`
	PacketsPerS   int    `json:"packets_per_s"`
	HeadsetIP     string `json:"headset_ip"`
	BodyTracked   bool   `json:"body_tracked"`
	TrackersReady bool   `json:"trackers_ready"`
	Rate          int    `json:"rate"`
	Calibration   struct {
		Valid   bool    `json:"valid"`
		Error   float64 `json:"error"`
		Samples int     `json:"samples"`
		Yaw     float64 `json:"yaw"`
	} `json:"calibration"`
	Trackers []struct {
		Name    string `json:"name"`
		Role    string `json:"role"`
		Enabled bool   `json:"enabled"`
		Added   bool   `json:"added"`
		Valid   bool   `json:"valid"`
	} `json:"trackers"`
	Pico *PicoStatus `json:"pico"` // nil — шлем не присылает (приложение на шлеме до 1.7)
}

// PicoStatus — трекеры PICO по словам шлема (common/protocol.h, StatusPacket). -1 — неизвестно.
type PicoStatus struct {
	Status             int `json:"status"`     // 0 нет данных, 1 точные, 2 неточные
	Error              int `json:"error"`      // при status 0: 1 не откалиброваны, 2 мало трекеров…
	Calibrated         int `json:"calibrated"` // 1 да, 0 нет
	Count              int `json:"count"`      // подключено трекеров
	CalibrationRequest int `json:"calibration_request"`
	CalibrationResult  int `json:"calibration_result"`
	Trackers           []struct {
		ID      int `json:"id"`
		Battery int `json:"battery"` // %, -1 — неизвестен
	} `json:"trackers"`
}

const (
	picoErrorNotCalibrated = 1
	picoErrorNotEnough     = 2
	picoNotRequested       = -32768 // CalibrationResult: шлем ещё не получал просьб откалибровать трекеры
)

// NotCalibrated — трекеры подключены, но не откалиброваны: нужна кнопка «Откалибровать».
func (p *PicoStatus) NotCalibrated() bool {
	if p == nil || p.Status == 1 || p.Status == 2 || p.Count == 0 {
		return false
	}
	return (p.Status == 0 && p.Error == picoErrorNotCalibrated) || (p.Calibrated == 0 && p.Count > 0)
}

// Batteries — заряд подключённых трекеров PICO, % (только известный).
func (p *PicoStatus) Batteries() (levels []int) {
	if p == nil {
		return nil
	}
	for _, t := range p.Trackers {
		if t.Battery >= 0 {
			levels = append(levels, t.Battery)
		}
	}
	return
}

// Snapshot — копия состояния для отрисовки интерфейса.
type Snapshot struct {
	DriverState    DriverState
	RestartSteamVR bool
	AppState       AppState
	AppKnown       bool // состояние приложения на шлеме проверено
	HeadsetSerial  string
	HeadsetWifi    bool
	HeadsetError   string
	AdbConnected   bool
	ServiceKnown   bool // состояние сервиса на шлеме проверено (шлем на связи по adb)
	ServiceRunning bool
	ServiceChange  string // "start" / "stop" — сервис сейчас включается / выключается; "" — нет
	ServiceOff     bool   // сервис выключили кнопкой: программа сама его не включает
	Listener       string
	Stats          map[string]string // nil — нет свежей статистики
	Driver         *DriverStatus     // nil — драйвер не на связи
	SteamVRRunning bool
	Tracking       bool
	Rate           int
	Trackers       []string // nil — набор ещё не известен
	Calibrating    bool     // открываем калибровку трекеров PICO на шлеме
}

// Poses — позы всех суставов (STAGE шлема) для 3D-вида.
type Poses [24]struct {
	X, Y, Z float64
	Valid   bool
}

const posesTimeout = 300 * time.Millisecond

type Controller struct {
	onLog, onToast func(string)
	conn           *net.UDPConn
	closing        chan struct{}

	wake           chan struct{} // переподключиться к шлему сейчас
	mu             sync.Mutex
	cfg            *Config
	driverState    DriverState
	restartSteamVR bool
	appState       AppState
	appKnown       bool
	headset        *Headset
	headsetError   string
	adbConnected   bool
	serviceKnown   bool
	serviceRunning bool
	serviceChange  string
	follower       *LogcatFollower
	listener       string
	stats          map[string]string
	statsAt        time.Time
	driver         *DriverStatus
	driverAt       time.Time
	poses          Poses
	posesAt        time.Time
	steamVRRunning bool
	calibration    int       // номер последней просьбы откалибровать трекеры (уходит драйверу в PTA1)
	calibrateUntil time.Time // до этого времени просьба идёт в командах драйверу
	calibrating    bool
}

func NewController(onLog, onToast func(string)) (*Controller, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: appPort})
	if err != nil {
		return nil, fmt.Errorf(T("не удалось открыть UDP-порт %d (программа уже запущена?): %w"), appPort, err)
	}
	c := &Controller{onLog: onLog, onToast: onToast, conn: conn, closing: make(chan struct{}),
		wake: make(chan struct{}, 1), cfg: loadConfig(), driverState: driverState(),
		calibration: 1 + rand.IntN(60000)} // не с 1: драйвер помнит номер прошлой просьбы
	go c.receiveDriver()
	go c.watchSteamVR()
	go c.connectLoop()
	return c, nil
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Snapshot{
		DriverState: c.driverState, RestartSteamVR: c.restartSteamVR, AppState: c.appState, AppKnown: c.appKnown,
		HeadsetError: c.headsetError, AdbConnected: c.adbConnected, Listener: c.listener,
		ServiceKnown: c.serviceKnown, ServiceRunning: c.serviceRunning, ServiceChange: c.serviceChange, ServiceOff: c.cfg.ServiceOff,
		SteamVRRunning: c.steamVRRunning, Tracking: c.cfg.trackingEnabled(), Rate: c.cfg.rate(),
		Calibrating: c.calibrating,
	}
	if c.headset != nil {
		s.HeadsetSerial, s.HeadsetWifi = c.headset.Serial, c.headset.OverWifi()
	}
	if c.stats != nil && time.Since(c.statsAt) < headsetStatsTimeout {
		s.Stats = c.stats
	}
	if c.driver != nil && time.Since(c.driverAt) < driverTimeout {
		s.Driver = c.driver
	}
	if c.cfg.Trackers != nil {
		s.Trackers = append([]string{}, *c.cfg.Trackers...)
	}
	return s
}

// Poses — последние позы суставов; ok=false, если свежих нет.
func (c *Controller) Poses() (Poses, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.poses, time.Since(c.posesAt) < posesTimeout
}

// ---------- команды драйверу ----------

func (c *Controller) SetRate(rate int) {
	c.mu.Lock()
	c.cfg.Rate = min(max(rate, minRate), maxRate)
	c.cfg.save()
	c.mu.Unlock()
	c.SendCommand()
}

func (c *Controller) SetTracking(enabled bool) {
	c.mu.Lock()
	c.cfg.Tracking = &enabled
	c.cfg.save()
	c.mu.Unlock()
	c.SendCommand()
}

// SetLanguage запоминает язык интерфейса (применится после перезапуска); false — он уже такой.
func (c *Controller) SetLanguage(language string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Language == language {
		return false
	}
	c.cfg.Language = language
	c.cfg.save()
	return true
}

func (c *Controller) SetTrackers(names []string) {
	var ordered []string
	for _, t := range trackers {
		if contains(names, t.Name) {
			ordered = append(ordered, t.Name)
		}
	}
	c.mu.Lock()
	c.cfg.Trackers = &ordered
	c.cfg.save()
	c.mu.Unlock()
	c.SendCommand()
}

// SendCommand — раз в секунду и при каждом изменении: драйвер считает программу подключённой, пока команды идут.
func (c *Controller) SendCommand() {
	c.mu.Lock()
	if c.cfg.Trackers == nil {
		c.mu.Unlock()
		return // набор ещё не известен — сначала возьмём его у драйвера
	}
	list := strings.Join(*c.cfg.Trackers, ",")
	if list == "" {
		list = "none"
	}
	tracking := 0
	if c.cfg.trackingEnabled() {
		tracking = 1
	}
	rate := c.cfg.rate()
	calibrate := ""
	if time.Now().Before(c.calibrateUntil) {
		calibrate = fmt.Sprintf(" calibrate=%d", c.calibration)
	}
	c.mu.Unlock()
	message := fmt.Sprintf("PTA1 tracking=%d rate=%d trackers=%s%s", tracking, rate, list, calibrate)
	c.conn.WriteToUDP([]byte(message), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: dataPort})
}

func (c *Controller) receiveDriver() {
	buffer := make([]byte, 16384)
	for {
		n, _, err := c.conn.ReadFromUDP(buffer)
		if errors.Is(err, net.ErrClosed) {
			return
		}
		var kind struct {
			Type   string       `json:"type"`
			Joints [][4]float64 `json:"joints"`
		}
		if err != nil || json.Unmarshal(buffer[:n], &kind) != nil {
			continue
		}
		if kind.Type == "poses" {
			c.mu.Lock()
			for i := 0; i < len(kind.Joints) && i < len(c.poses); i++ {
				j := kind.Joints[i]
				c.poses[i].X, c.poses[i].Y, c.poses[i].Z, c.poses[i].Valid = j[0], j[1], j[2], j[3] != 0
			}
			c.posesAt = time.Now()
			c.mu.Unlock()
			continue
		}
		var status DriverStatus
		if kind.Type != "status" || json.Unmarshal(buffer[:n], &status) != nil {
			continue
		}
		c.mu.Lock()
		reconnected := c.driver == nil || time.Since(c.driverAt) >= driverTimeout
		c.driver, c.driverAt = &status, time.Now()
		if c.cfg.Trackers == nil {
			// Первый запуск программы: берём набор, который сейчас у драйвера.
			var enabled []string
			for _, t := range status.Trackers {
				if t.Enabled {
					enabled = append(enabled, t.Name)
				}
			}
			c.cfg.Trackers = &enabled
			c.cfg.save()
		}
		c.mu.Unlock()
		if reconnected {
			c.onLog(T("драйвер SteamVR на связи"))
		}
	}
}

func (c *Controller) watchSteamVR() {
	for {
		select {
		case <-c.closing:
			return
		case <-time.After(2 * time.Second):
		}
		_, running := steamVRStartedAt()
		state, restart := driverState(), steamVRNeedsRestart()
		c.mu.Lock()
		changed := running != c.steamVRRunning
		c.steamVRRunning, c.driverState, c.restartSteamVR = running, state, restart
		c.mu.Unlock()
		if changed {
			c.onLog(map[bool]string{true: T("SteamVR запущен"), false: T("SteamVR закрыт")}[running])
		}
	}
}

// ---------- шлем ----------

// Config — копия настроек для мастера установки.
func (c *Controller) Config() *Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// Reconnect — переподключиться к шлему (например, после установки приложения мастером).
func (c *Controller) Reconnect() {
	c.mu.Lock()
	if c.follower != nil {
		c.follower.Stop()
		c.follower = nil
	}
	c.headset, c.appKnown, c.serviceKnown = nil, false, false
	c.cfg = loadConfig()
	c.mu.Unlock()
	c.wakeUp()
}

func (c *Controller) wakeUp() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// connectLoop держит связь со шлемом по adb, ничего не устанавливая: сохранённый Wi-Fi адрес или USB.
// Если приложение уже стоит — запускает сервис (авторежим) и следит за его логом; потом раз в 5 с
// проверяет, не выключили ли сервис (на шлеме, из меню программы, после переустановки приложения).
func (c *Controller) connectLoop() {
	for {
		c.mu.Lock()
		connected, cfg, headset := c.follower != nil, c.cfg, c.headset
		c.mu.Unlock()
		if !connected {
			c.tryConnect(cfg)
		} else {
			c.checkService(headset)
		}
		select {
		case <-c.closing:
			return
		case <-c.wake:
		case <-time.After(headsetRetry):
		}
	}
}

func (c *Controller) tryConnect(cfg *Config) {
	headset, err := connectHeadset(cfg)
	if err != nil {
		c.mu.Lock()
		changed := c.headsetError != err.Error()
		c.headsetError = err.Error()
		c.mu.Unlock()
		if changed {
			c.onLog(err.Error())
		}
		return
	}
	state := appState(headset)
	c.mu.Lock()
	c.headset, c.headsetError, c.appState, c.appKnown = headset, "", state, true
	c.mu.Unlock()
	if state == AppMissing {
		c.onLog(T("приложение на шлеме не установлено — откройте мастер установки"))
		return
	}
	allowBackgroundCalibration(headset)
	if cfg.ServiceOff {
		// Выключен кнопкой — не включаем, только следим за состоянием.
		c.checkService(headset)
	} else if err := startService(headset); err != nil {
		c.onLog(T("не удалось запустить сервис на шлеме: ") + err.Error())
		return
	} else {
		// dumpsys видит сервис не сразу. Если его выключили на самом шлеме (приложение 1.6+), он не запустится —
		// это покажет проверка через 5 с.
		c.setService(true)
	}
	follower := &LogcatFollower{
		headset:      headset,
		OnStats:      c.onStats,
		OnListener:   c.onListener,
		OnMessage:    func(text string) { c.onLog(T("шлем: ") + text) },
		OnConnection: c.onAdb,
	}
	follower.Start()
	c.mu.Lock()
	c.follower = follower
	c.mu.Unlock()
	if cfg.ServiceOff {
		c.onLog(Tf("шлем на связи (%s), сервис выключен вами — сама программа его не включает", headset.Serial))
	} else {
		c.onLog(Tf("шлем на связи (%s), сервис запущен", headset.Serial))
	}
}

func (c *Controller) onStats(stats map[string]string) {
	c.mu.Lock()
	c.stats, c.statsAt = stats, time.Now()
	c.mu.Unlock()
}

func (c *Controller) onListener(text string) {
	c.mu.Lock()
	c.listener = text
	c.mu.Unlock()
	c.onLog(T("шлем: ") + text)
	switch {
	case strings.HasPrefix(text, "waiting"): // сервис только что запустился
		c.setService(true)
	case text == "service stopped":
		c.setService(false)
	}
}

// checkService — запущен ли сервис на шлеме (ошибка adb — состояние неизвестно).
func (c *Controller) checkService(headset *Headset) {
	if headset == nil {
		return
	}
	running, err := serviceRunning(headset)
	if err != nil {
		c.mu.Lock()
		c.serviceKnown = false
		c.mu.Unlock()
		return
	}
	c.setService(running)
}

func (c *Controller) setService(running bool) {
	c.mu.Lock()
	changed := !c.serviceKnown || running != c.serviceRunning
	c.serviceKnown, c.serviceRunning = true, running
	c.mu.Unlock()
	if changed && !running {
		c.onLog(T("сервис на шлеме выключен — без него трекинг не передаётся; включить: кнопка «Включить сервис»"))
	}
}

func (c *Controller) onAdb(connected bool) {
	c.mu.Lock()
	changed := connected != c.adbConnected
	c.adbConnected = connected
	c.mu.Unlock()
	if changed && connected {
		c.onLog(T("adb: шлем на связи"))
	} else if changed {
		c.onLog(T("adb: связь со шлемом потеряна (трекингу это не мешает)"))
	}
}

// ToggleService включает сервис на шлеме, если он выключен, и выключает, если включён.
func (c *Controller) ToggleService() {
	c.mu.Lock()
	known, running := c.serviceKnown, c.serviceRunning
	c.mu.Unlock()
	switch {
	case !known:
		c.onToast(T("Шлем не на связи по adb"))
	case running:
		c.StopService()
	default:
		c.StartService()
	}
}

func (c *Controller) StopService() {
	headset := c.beginServiceChange("stop")
	if headset == nil {
		return
	}
	go func() {
		defer c.endServiceChange()
		c.setServiceOff(true)
		if err := setServiceEnabled(headset, false); err != nil {
			c.onLog(T("не удалось выключить сервис на шлеме: ") + err.Error())
		}
		c.checkService(headset)
	}()
}

// StartService включает сервис на шлеме (авторежим: трекинг пойдёт, когда SteamVR попросит данные).
func (c *Controller) StartService() {
	headset := c.beginServiceChange("start")
	if headset == nil {
		return
	}
	go func() {
		defer c.endServiceChange()
		c.setServiceOff(false)
		if err := setServiceEnabled(headset, true); err != nil {
			c.onLog(T("не удалось запустить сервис на шлеме: ") + err.Error())
			c.onToast(T("Не удалось включить сервис на шлеме"))
			return
		}
		c.setService(true) // dumpsys видит сервис не сразу; следующая проверка (через 5 с) уточнит
		c.onLog(T("сервис на шлеме включён"))
	}()
}

// beginServiceChange — шлем для включения ("start") или выключения ("stop") сервиса; nil — шлема нет
// или операция уже идёт.
func (c *Controller) beginServiceChange(change string) *Headset {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.headset == nil {
		c.onToast(T("Шлем не на связи по adb"))
		return nil
	}
	if c.serviceChange != "" {
		return nil
	}
	c.serviceChange = change
	return c.headset
}

// setServiceOff запоминает выбор пользователя: выключенный кнопкой сервис программа сама не включает.
func (c *Controller) setServiceOff(off bool) {
	c.mu.Lock()
	c.cfg.ServiceOff = off
	c.cfg.save()
	c.mu.Unlock()
}

func (c *Controller) endServiceChange() {
	c.mu.Lock()
	c.serviceChange = ""
	c.mu.Unlock()
}

// CalibrateTrackers открывает на шлеме калибровку трекеров PICO: просьба уходит через драйвер SteamVR
// (так работает и без adb), шлем открывает калибровку через рантайм или запуском приложения калибровки
// и отвечает в пакете состояния. Не вышло или нет ответа — открываем приложение калибровки по adb.
func (c *Controller) CalibrateTrackers() {
	c.mu.Lock()
	if c.calibrating {
		c.mu.Unlock()
		return
	}
	c.calibrating = true
	c.calibration = c.calibration%65535 + 1
	request := c.calibration
	viaDriver := c.driver != nil && time.Since(c.driverAt) < driverTimeout && c.driver.Data
	if viaDriver {
		c.calibrateUntil = time.Now().Add(calibrationTimeout)
	}
	c.mu.Unlock()
	c.onLog(T("открываю калибровку трекеров PICO на шлеме"))
	if viaDriver {
		c.SendCommand()
	}
	go func() {
		defer func() {
			c.mu.Lock()
			c.calibrating, c.calibrateUntil = false, time.Time{}
			c.mu.Unlock()
		}()
		result, answered := 0, false
		for deadline := time.Now().Add(calibrationTimeout); viaDriver && time.Now().Before(deadline); {
			time.Sleep(100 * time.Millisecond)
			c.mu.Lock()
			if d := c.driver; d != nil && d.Pico != nil && d.Pico.CalibrationRequest == request &&
				d.Pico.CalibrationResult != picoNotRequested {
				result, answered = d.Pico.CalibrationResult, true
			}
			c.mu.Unlock()
			if answered {
				break
			}
		}
		// 0 — открыл рантайм, 1 — запущено приложение калибровки, 2 — запущено, но Android мог не дать
		// фоновому сервису показать окно; < 0 — не получилось (common/protocol.h, TrackingService.java).
		if answered && (result == 0 || result == 1) {
			c.onLog(Tf("шлем открыл калибровку трекеров (%d)", result))
			c.onToast(T("Калибровка открыта на шлеме — наденьте его"))
			return
		}
		c.mu.Lock()
		headset := c.headset
		c.mu.Unlock()
		if answered {
			c.onLog(Tf("шлем не смог открыть калибровку сам (%d)", result))
		} else if viaDriver {
			c.onLog(T("шлем не ответил на просьбу открыть калибровку"))
		}
		if headset == nil {
			if answered && result == 2 {
				c.onToast(T("Калибровка запущена — если её не видно, откройте «Motion Tracker» на шлеме"))
			} else {
				c.onToast(T("Не удалось открыть калибровку — откройте «Motion Tracker» на шлеме"))
			}
			return
		}
		if err := launchCalibrationApp(headset); err != nil {
			c.onLog(T("не удалось открыть калибровку по adb: ") + err.Error())
			c.onToast(T("Не удалось открыть калибровку — откройте «Motion Tracker» на шлеме"))
			return
		}
		c.onLog(T("калибровка трекеров открыта на шлеме по adb"))
		c.onToast(T("Калибровка открыта на шлеме — наденьте его"))
	}()
}

func (c *Controller) Close() {
	close(c.closing)
	c.mu.Lock()
	if c.follower != nil {
		c.follower.Stop()
	}
	c.mu.Unlock()
	c.conn.Close()
}
