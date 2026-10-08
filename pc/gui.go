// Окно программы (GTK 4 / libadwaita): вкладки «Трекеры», «3D-вид», «Настройки».
// GTK вызывается только из главного потока: фоновые события приходят через glib.IdleAdd,
// состояние перечитывается раз в 0.5 с, 3D-вид перерисовывается ~30 раз/с.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const appID = "dev.picotrackers.Control"

const css = `
.dot { font-size: 15px; }
.hero { padding: 20px 22px; }
.hero-dot { font-size: 34px; }
.hero-title { font-size: 19pt; font-weight: 800; }
.log { font-family: monospace; font-size: 9pt; }
`

func runGUI() int {
	app := adw.NewApplication(appID, gio.ApplicationDefaultFlags)
	var w *window
	app.ConnectStartup(func() {
		provider := gtk.NewCSSProvider()
		provider.LoadFromString(css)
		gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
		setupIcon()
	})
	app.ConnectActivate(func() {
		if w != nil {
			w.win.Present()
			return
		}
		w = newWindow(app)
	})
	app.ConnectShutdown(func() {
		if w != nil && w.c != nil {
			w.c.Close()
		}
	})
	markRunning()
	return app.Run(os.Args[:1])
}

// setupIcon делает вшитый значок доступным GTK по имени appID (окно, «О программе», мастер) — без установки:
// файл кладётся в кэш, папка добавляется в пути поиска темы значков.
func setupIcon() {
	cache, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir := filepath.Join(cache, "picotrackers", "icons")
	writeIfChanged(filepath.Join(dir, appID+".svg"), appIcon)
	gtk.IconThemeGetForDisplay(gdk.DisplayGetDefault()).AddSearchPath(dir)
	gtk.WindowSetDefaultIconName(appID)
}

// trackerControl — переключатель трекера: строка-список (дополнительные суставы) или строка на схеме.
type trackerControl struct {
	active    func() bool
	setActive func(bool)
	setState  func(text, short, class string)
}

type window struct {
	app   *adw.Application
	c     *Controller
	win   *adw.ApplicationWindow
	stack *adw.ViewStack

	toasts                       *adw.ToastOverlay
	heroDot, heroTitle, heroText *gtk.Label
	heroButton, steamVRButton    *gtk.Button
	headsetRow, steamVRRow       *adw.ActionRow
	calibrationRow               *adw.ActionRow
	headsetDot, steamVRDot       *gtk.Label
	calibrationDot               *gtk.Label
	trackingSwitch               *adw.SwitchRow
	rateRow                      *adw.SpinRow
	body                         *bodyDiagram
	view3d                       *view3D
	controls                     map[string]trackerControl
	wizard                       *wizard
	heroAction                   func()
	logBuffer                    *gtk.TextBuffer
	logLines                     []string
	updating, switchesLoaded     bool
}

func newWindow(app *adw.Application) *window {
	w := &window{app: app, controls: map[string]trackerControl{}}
	w.win = adw.NewApplicationWindow(&app.Application)
	w.win.SetTitle("PICO Trackers")
	w.win.SetDefaultSize(640, 940)

	w.stack = adw.NewViewStack()
	header := adw.NewHeaderBar()
	switcher := adw.NewViewSwitcher()
	switcher.SetStack(w.stack)
	switcher.SetPolicy(adw.ViewSwitcherPolicyWide)
	header.SetTitleWidget(switcher)
	menu := gio.NewMenu()
	menu.Append("Мастер установки", "app.wizard")
	menu.Append("Выключить сервис на шлеме", "app.stop-service")
	menu.Append("О программе", "app.about")
	menuButton := gtk.NewMenuButton()
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetMenuModel(menu)
	menuButton.SetTooltipText("Меню")
	header.PackEnd(menuButton)

	w.toasts = adw.NewToastOverlay()
	w.toasts.SetChild(w.stack)
	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(header)
	toolbar.SetContent(w.toasts)
	w.win.SetContent(toolbar)

	c, err := NewController(w.logFromThread, w.toastFromThread)
	if err != nil {
		dialog := adw.NewAlertDialog("Не удалось запустить", err.Error())
		dialog.AddResponse("close", "Закрыть")
		dialog.ConnectResponse(func(string) { app.Quit() })
		w.win.Present()
		dialog.Present(w.win)
		return w
	}
	w.c = c

	trackersPage := adw.NewPreferencesPage()
	trackersPage.Add(w.buildHero())
	trackersPage.Add(w.buildConnection())
	trackersPage.Add(w.buildTracking())
	trackersPage.Add(w.buildBody())
	trackersPage.Add(w.buildExtra())
	w.stack.AddTitledWithIcon(trackersPage, "trackers", "Трекеры", "avatar-default-symbolic")

	w.view3d = newView3D(c, w.enabledTrackers)
	w.stack.AddTitledWithIcon(w.view3d.widget, "view3d", "3D-вид", "object-rotate-right-symbolic")

	settingsPage := adw.NewPreferencesPage()
	settingsPage.Add(w.buildApp())
	settingsPage.Add(w.buildLog())
	w.stack.AddTitledWithIcon(settingsPage, "settings", "Настройки", "preferences-system-symbolic")
	w.addActions()

	w.win.Present()
	w.refresh()
	go func() {
		if wizardNeeded() {
			glib.IdleAdd(w.openWizard)
		}
	}()
	glib.TimeoutAdd(500, func() bool { w.refresh(); return true })
	glib.TimeoutAdd(1000, func() bool { c.SendCommand(); return true })
	glib.TimeoutAdd(33, func() bool {
		if w.stack.VisibleChildName() == "view3d" {
			w.view3d.area.QueueDraw()
		}
		return true
	})
	return w
}

// ---------- построение ----------

func newDot() *gtk.Label {
	label := gtk.NewLabel("●")
	label.SetVAlign(gtk.AlignCenter)
	label.AddCSSClass("dot")
	setDot(label, "dim-label")
	return label
}

func setDot(label *gtk.Label, class string) {
	for _, c := range []string{"success", "warning", "error", "dim-label"} {
		label.RemoveCSSClass(c)
	}
	label.AddCSSClass(class)
}

func statusRow(group *adw.PreferencesGroup, title string) (*adw.ActionRow, *gtk.Label) {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitleLines(2)
	dot := newDot()
	row.AddPrefix(dot)
	group.Add(row)
	return row, dot
}

func (w *window) buildHero() *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	box := gtk.NewBox(gtk.OrientationHorizontal, 18)
	box.AddCSSClass("card")
	box.AddCSSClass("hero")
	w.heroDot = gtk.NewLabel("●")
	w.heroDot.SetVAlign(gtk.AlignCenter)
	w.heroDot.AddCSSClass("hero-dot")
	box.Append(w.heroDot)

	texts := gtk.NewBox(gtk.OrientationVertical, 4)
	texts.SetHExpand(true)
	texts.SetVAlign(gtk.AlignCenter)
	w.heroTitle = gtk.NewLabel("")
	w.heroTitle.SetXAlign(0)
	w.heroTitle.SetWrap(true)
	w.heroTitle.AddCSSClass("hero-title")
	w.heroText = gtk.NewLabel("")
	w.heroText.SetXAlign(0)
	w.heroText.SetWrap(true)
	w.heroText.AddCSSClass("dim-label")
	texts.Append(w.heroTitle)
	texts.Append(w.heroText)
	box.Append(texts)

	w.heroButton = gtk.NewButtonWithLabel("Запустить SteamVR")
	w.heroButton.SetVAlign(gtk.AlignCenter)
	w.heroButton.AddCSSClass("suggested-action")
	w.heroButton.AddCSSClass("pill")
	w.heroButton.ConnectClicked(func() {
		if w.heroAction != nil {
			w.heroAction()
		}
	})
	box.Append(w.heroButton)
	group.Add(box)
	return group
}

func (w *window) buildConnection() *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Подключение")
	w.headsetRow, w.headsetDot = statusRow(group, "Шлем PICO")
	w.steamVRRow, w.steamVRDot = statusRow(group, "SteamVR")
	w.steamVRButton = gtk.NewButtonWithLabel("Запустить")
	w.steamVRButton.SetVAlign(gtk.AlignCenter)
	w.steamVRButton.ConnectClicked(w.launchSteamVR)
	w.steamVRRow.AddSuffix(w.steamVRButton)
	w.calibrationRow, w.calibrationDot = statusRow(group, "Калибровка")
	return group
}

func (w *window) buildTracking() *adw.PreferencesGroup {
	s := w.c.Snapshot()
	group := adw.NewPreferencesGroup()
	w.trackingSwitch = adw.NewSwitchRow()
	w.trackingSwitch.SetTitle("Передавать трекинг")
	w.trackingSwitch.SetSubtitle("Выключите, чтобы временно убрать все трекеры — шлем перестанет отправлять данные")
	w.trackingSwitch.SetActive(s.Tracking)
	w.trackingSwitch.NotifyProperty("active", func() {
		if !w.updating {
			w.c.SetTracking(w.trackingSwitch.Active())
		}
	})
	group.Add(w.trackingSwitch)

	w.rateRow = adw.NewSpinRowWithRange(minRate, maxRate, 10)
	w.rateRow.SetTitle("Частота передачи")
	w.rateRow.SetSubtitle("Снимков скелета в секунду: больше — плавнее, но выше нагрузка на Wi-Fi")
	w.rateRow.SetValue(float64(s.Rate))
	w.rateRow.NotifyProperty("value", func() { w.c.SetRate(int(w.rateRow.Value())) })
	group.Add(w.rateRow)
	return group
}

func (w *window) buildBody() *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Трекеры")
	group.SetDescription("Применяется сразу: включённый трекер появляется в SteamVR, выключенный отключается " +
		"и пропадает из игр. Зелёный — отслеживается, оранжевый — нет позиции.")
	presets := gtk.NewBox(gtk.OrientationHorizontal, 0)
	presets.AddCSSClass("linked")
	presets.SetVAlign(gtk.AlignCenter)
	for _, preset := range []struct {
		label string
		names []string
	}{{"Ноги", legsTrackers}, {"Основные", mainTrackers()}, {"Все", allTrackers()}} {
		names := preset.names
		button := gtk.NewButtonWithLabel(preset.label)
		button.ConnectClicked(func() {
			w.setSwitches(names)
			w.c.SetTrackers(names)
		})
		presets.Append(button)
	}
	group.SetHeaderSuffix(presets)

	w.body = newBodyDiagram(w.onTrackerToggled, &w.updating)
	w.body.widget.AddCSSClass("card")
	for name, item := range w.body.items {
		item := item
		w.controls[name] = trackerControl{
			active:    item.sw.Active,
			setActive: item.sw.SetActive,
			setState:  func(_, short, class string) { w.body.setState(item.name, short, class) },
		}
	}
	group.Add(w.body.widget)
	return group
}

func (w *window) buildExtra() *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Дополнительные суставы")
	group.SetDescription("У них нет роли в SteamVR: игры вроде VRChat могут путать их с основными трекерами.")
	expander := adw.NewExpanderRow()
	expander.SetTitle("Ещё 14 суставов")
	expander.SetSubtitle("голова, кисти, шея, позвоночник, носки…")
	for _, t := range trackers {
		if t.HasRole {
			continue
		}
		row := adw.NewSwitchRow()
		row.SetTitle(t.Label)
		dot := newDot()
		row.AddPrefix(dot)
		row.NotifyProperty("active", func() {
			if !w.updating {
				w.onTrackerToggled()
			}
		})
		w.controls[t.Name] = trackerControl{
			active:    row.Active,
			setActive: row.SetActive,
			setState: func(text, _, class string) {
				row.SetSubtitle(text)
				setDot(dot, class)
			},
		}
		expander.AddRow(row)
	}
	group.Add(expander)
	return group
}

func (w *window) buildApp() *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Программа")
	autostart := adw.NewSwitchRow()
	autostart.SetTitle("Запускать при входе в систему")
	autostart.SetSubtitle("Программа будет следить за шлемом и драйвером в фоне")
	autostart.SetActive(autostartEnabled())
	autostart.NotifyProperty("active", func() { setAutostart(autostart.Active()) })
	group.Add(autostart)

	wizardRow := adw.NewActionRow()
	wizardRow.SetTitle("Мастер установки")
	wizardRow.SetSubtitle("Установить или обновить драйвер SteamVR и приложение на шлеме")
	button := gtk.NewButtonWithLabel("Открыть")
	button.SetVAlign(gtk.AlignCenter)
	button.ConnectClicked(w.openWizard)
	wizardRow.AddSuffix(button)
	group.Add(wizardRow)

	if selfInstall { // на Windows удаляют через «Параметры → Приложения»
		removeRow := adw.NewActionRow()
		removeRow.SetTitle("Удалить программу")
		removeRow.SetSubtitle("Драйвер SteamVR, программа, пункт меню и настройки. Приложение на шлеме останется")
		remove := gtk.NewButtonWithLabel("Удалить")
		remove.AddCSSClass("destructive-action")
		remove.SetVAlign(gtk.AlignCenter)
		remove.ConnectClicked(w.confirmUninstall)
		removeRow.AddSuffix(remove)
		group.Add(removeRow)
	}
	return group
}

func (w *window) confirmUninstall() {
	dialog := adw.NewAlertDialog("Удалить PICO Trackers?",
		"Драйвер будет снят с регистрации в SteamVR и удалён, программа исчезнет из меню приложений, "+
			"её настройки будут удалены. Приложение на шлеме останется — его можно удалить в настройках шлема.")
	dialog.AddResponse("cancel", "Отмена")
	dialog.AddResponse("remove", "Удалить")
	dialog.SetResponseAppearance("remove", adw.ResponseDestructive)
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response != "remove" {
			return
		}
		go func() {
			err := uninstall(w.logFromThread)
			glib.IdleAdd(func() {
				if err != nil {
					failed := adw.NewAlertDialog("Не удалось удалить", err.Error())
					failed.AddResponse("close", "Закрыть")
					failed.Present(w.win)
					return
				}
				w.app.Quit()
			})
		}()
	})
	dialog.Present(w.win)
}

func (w *window) buildLog() *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Журнал событий")
	view := gtk.NewTextView()
	view.SetEditable(false)
	view.SetCursorVisible(false)
	view.SetWrapMode(gtk.WrapWordChar)
	view.SetTopMargin(10)
	view.SetBottomMargin(10)
	view.SetLeftMargin(12)
	view.SetRightMargin(12)
	view.AddCSSClass("log")
	w.logBuffer = view.Buffer()
	scroll := gtk.NewScrolledWindow()
	scroll.SetMinContentHeight(420)
	scroll.SetChild(view)
	scroll.AddCSSClass("card")
	group.Add(scroll)
	return group
}

func (w *window) addActions() {
	for name, callback := range map[string]func(){
		"wizard":       w.openWizard,
		"stop-service": w.c.StopService,
		"about":        w.showAbout,
	} {
		action := gio.NewSimpleAction(name, nil)
		callback := callback
		action.ConnectActivate(func(*glib.Variant) { callback() })
		w.app.AddAction(action)
	}
}

// ---------- действия ----------

func (w *window) launchSteamVR() {
	launchSteamVR()
	w.toasts.AddToast(adw.NewToast("Запускаю SteamVR…"))
}

func (w *window) enabledTrackers() (names []string) {
	for _, t := range trackers {
		if w.controls[t.Name].active() {
			names = append(names, t.Name)
		}
	}
	return
}

func (w *window) onTrackerToggled() {
	w.c.SetTrackers(w.enabledTrackers())
	w.refresh()
}

func (w *window) setSwitches(names []string) {
	w.updating = true
	for name, control := range w.controls {
		control.setActive(contains(names, name))
	}
	w.updating = false
	w.refresh()
}

func (w *window) showAbout() {
	about := adw.NewAboutDialog()
	about.SetApplicationName("PICO Trackers")
	about.SetApplicationIcon(appID)
	about.SetVersion(appVersion)
	about.SetComments("PICO Motion Trackers в SteamVR через Steam Link")
	about.SetDeveloperName("PICOtrackers")
	about.Present(w.win)
}

func (w *window) logFromThread(text string) {
	glib.IdleAdd(func() {
		w.logLines = append(w.logLines, time.Now().Format("15:04:05")+"  "+text)
		if len(w.logLines) > 300 {
			w.logLines = w.logLines[len(w.logLines)-300:]
		}
		reversed := make([]string, len(w.logLines))
		for i, line := range w.logLines {
			reversed[len(w.logLines)-1-i] = line
		}
		if w.logBuffer != nil {
			w.logBuffer.SetText(strings.Join(reversed, "\n"))
		}
	})
}

func (w *window) toastFromThread(text string) {
	glib.IdleAdd(func() { w.toasts.AddToast(adw.NewToast(text)) })
}

// ---------- обновление раз в 0.5 с ----------

func (w *window) refresh() {
	if w.c == nil {
		return
	}
	s := w.c.Snapshot()
	if s.Trackers != nil && !w.switchesLoaded {
		w.switchesLoaded = true
		w.setSwitches(s.Trackers)
		return
	}
	w.refreshHeadset(s)
	w.refreshSteamVR(s)
	w.refreshCalibration(s)
	w.refreshTrackers(s)
	w.refreshHero(s)
}

func (w *window) refreshHeadset(s Snapshot) {
	var parts []string
	state := "dim-label"
	switch d := s.Driver; {
	case d != nil && d.Data:
		ready := "трекеры НЕ подключены"
		state = "warning"
		if d.TrackersReady {
			ready, state = "трекеры подключены", "success"
		}
		parts = append(parts, fmt.Sprintf("передаёт %d из %d пак/с · %s", d.PacketsPerS, d.Rate, ready))
	case s.Stats != nil:
		parts = append(parts, "передаёт "+s.Stats["sent"]+" пак/с")
		state = "success"
	case s.Listener != "" && !strings.HasPrefix(s.Listener, "tracking ->"):
		parts = append(parts, "сервис ждёт SteamVR")
	default:
		parts = append(parts, "нет данных")
		if s.SteamVRRunning {
			state = "warning"
		}
	}
	switch {
	case s.AppKnown && s.AppState == AppMissing:
		parts = append(parts, "приложение не установлено — откройте мастер установки")
		state = "warning"
	case s.AppKnown && s.AppState == AppOutdated && (s.Driver == nil || !s.Driver.Data):
		parts = append(parts, "приложение устарело — обновите в мастере установки")
		state = "warning"
	}
	switch {
	case s.HeadsetSerial != "":
		via := "adb по USB "
		if s.HeadsetWifi {
			via = "adb по Wi-Fi "
		}
		link := via + strings.Split(s.HeadsetSerial, ":")[0]
		if !s.AdbConnected {
			link += " (нет связи)"
		}
		parts = append(parts, link)
	case s.HeadsetError != "":
		parts = append(parts, "adb: шлем не найден")
		if s.Driver == nil || !s.Driver.Data {
			state = "error"
		}
	}
	w.headsetRow.SetSubtitle(strings.Join(parts, " · "))
	setDot(w.headsetDot, state)
}

func (w *window) refreshSteamVR(s Snapshot) {
	w.steamVRButton.SetVisible(!s.SteamVRRunning)
	var text, state string
	switch d := s.Driver; {
	case !s.SteamVRRunning:
		text, state = "не запущен", "dim-label"
	case d == nil:
		text, state = "запущен, драйвер не отвечает", "warning"
		if s.RestartSteamVR {
			text += " — перезапустите SteamVR"
		}
	default:
		hmd, hmdState := "шлем не подключён (Steam Link)", "warning"
		if d.HMD {
			hmd, hmdState = "шлем подключён", "success"
		}
		text, state = "драйвер на связи · "+hmd, hmdState
		if !d.Tracking {
			text += " · трекинг выключен"
		}
	}
	w.steamVRRow.SetSubtitle(text)
	setDot(w.steamVRDot, state)
}

func (w *window) refreshCalibration(s Snapshot) {
	var text, state string
	switch d := s.Driver; {
	case d == nil:
		text, state = "—", "dim-label"
	case d.Calibration.Valid:
		mm := d.Calibration.Error * 1000
		text = fmt.Sprintf("точность %.0f мм · %d замеров · поворот %.1f°", mm, d.Calibration.Samples, d.Calibration.Yaw)
		state = "success"
		if mm >= 30 {
			state = "warning"
		}
	case !d.HMD:
		text, state = "ждёт позу шлема из Steam Link", "dim-label"
	default:
		text, state = "идёт: постойте спокойно и медленно повертите головой", "warning"
	}
	w.calibrationRow.SetSubtitle(text)
	setDot(w.calibrationDot, state)
}

func (w *window) refreshTrackers(s Snapshot) {
	live := map[string]int{}
	if s.Driver != nil {
		for i, t := range s.Driver.Trackers {
			live[t.Name] = i
		}
	}
	for name, control := range w.controls {
		i, known := live[name]
		switch {
		case !control.active():
			control.setState("выключен", "выключен", "dim-label")
		case s.Driver == nil:
			control.setState("включён · SteamVR не на связи", "нет связи со SteamVR", "dim-label")
		case !known || !s.Driver.Trackers[i].Enabled:
			control.setState("применяется…", "применяется…", "dim-label")
		case s.Driver.Trackers[i].Valid:
			control.setState("в SteamVR · отслеживается", "отслеживается", "success")
		case !s.Driver.Trackers[i].Added:
			control.setState("появится в SteamVR, когда пойдут данные", "ждёт данные", "dim-label")
		default:
			control.setState("в SteamVR · нет позиции", "нет позиции", "warning")
		}
	}
}

func (w *window) refreshHero(s Snapshot) {
	d := s.Driver
	active := 0
	if d != nil {
		for _, t := range d.Trackers {
			if t.Valid {
				active++
			}
		}
	}
	var title, text, state string
	w.heroAction = nil
	switch {
	case !s.DriverState.Ready():
		title, text, state = "Нужна установка", "Драйвер SteamVR не установлен или устарел", "warning"
		w.heroButton.SetLabel("Открыть мастер")
		w.heroAction = w.openWizard
	case !s.SteamVRRunning:
		title, text, state = "Ждём SteamVR", "Подключитесь со шлема через Steam Link или запустите SteamVR", "dim-label"
		w.heroButton.SetLabel("Запустить SteamVR")
		w.heroAction = w.launchSteamVR
	case d == nil:
		title, text, state = "Драйвер не отвечает", "SteamVR запускается или драйвер не загружен", "warning"
		if s.RestartSteamVR {
			text = "Перезапустите SteamVR, чтобы загрузить новую версию драйвера"
		}
	case !s.Tracking:
		title, text, state = "Трекинг выключен", "Включите переключатель «Передавать трекинг»", "dim-label"
	case !d.Data:
		title, text, state = "Ждём данные со шлема", "Шлем включён и надет? Сервис PICO Trackers запущен?", "warning"
	case !d.HMD:
		title, text, state = "Шлем не подключён к SteamVR", "Подключитесь через Steam Link", "warning"
	case !d.Calibration.Valid:
		title, text, state = "Калибровка", "Постойте спокойно и медленно повертите головой", "warning"
	case active == 0:
		title, text, state = "Нет активных трекеров", "Включите трекеры на схеме ниже", "warning"
	default:
		title, state = "Трекинг работает", "success"
		text = fmt.Sprintf("%d трекеров в SteamVR · %d пак/с · точность %.0f мм", active, d.PacketsPerS,
			d.Calibration.Error*1000)
	}
	w.heroTitle.SetLabel(title)
	w.heroText.SetLabel(text)
	setDot(w.heroDot, state)
	w.heroButton.SetVisible(w.heroAction != nil)
}
