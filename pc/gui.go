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
	if waitPID > 0 {
		waitForExit(waitPID, 15*time.Second) // прошлая версия ещё держит имя на D-Bus и UDP-порт
	}
	app := adw.NewApplication(appID, gio.ApplicationDefaultFlags)
	var w *window
	app.ConnectStartup(func() {
		initGTKFromEnv()
		restoreHostEnv()
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
	theme := gtk.IconThemeGetForDisplay(gdk.DisplayGetDefault())
	theme.AddSearchPath(dir)
	gtk.WindowSetDefaultIconName(appID)
	// Тема значков из настроек системы может не знать значков окна или её может не быть (AppImage на системе
	// без этой темы) — тогда Adwaita: она есть всегда (у AppImage — своя).
	if !theme.HasIcon("avatar-default-symbolic") {
		gtk.SettingsGetDefault().SetObjectProperty("gtk-icon-theme-name", "Adwaita")
	}
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
	serviceButton                *gtk.Button
	menu                         *gio.Menu
	serviceMenuLabel             string
	serviceAction                *gio.SimpleAction
	headsetRow, steamVRRow       *adw.ActionRow
	picoRow, calibrationRow      *adw.ActionRow
	headsetDot, steamVRDot       *gtk.Label
	picoDot, calibrationDot      *gtk.Label
	calibrateButton              *gtk.Button
	trackingSwitch               *adw.SwitchRow
	rateRow                      *adw.SpinRow
	body                         *bodyDiagram
	view3d                       *view3D
	controls                     map[string]trackerControl
	wizard                       *wizard
	update                       *Release // версия новее, nil — нет
	updateOpen                   bool     // открыто окно обновления
	updateRow                    *adw.ActionRow
	heroAction                   func()
	logBuffer                    *gtk.TextBuffer
	logLines                     []string
	updating, switchesLoaded     bool
}

func newWindow(app *adw.Application) *window {
	w := &window{app: app, controls: map[string]trackerControl{}, serviceMenuLabel: T(serviceMenuUnknown)}
	w.win = adw.NewApplicationWindow(&app.Application)
	w.win.SetTitle("PICO Trackers")
	if devMode {
		w.win.SetTitle("PICO Trackers (dev)") // не перепутать с установленной версией
	}
	w.win.SetDefaultSize(640, 940)

	w.stack = adw.NewViewStack()
	header := adw.NewHeaderBar()
	switcher := adw.NewViewSwitcher()
	switcher.SetStack(w.stack)
	switcher.SetPolicy(adw.ViewSwitcherPolicyWide)
	header.SetTitleWidget(switcher)
	menu := gio.NewMenu()
	menu.Append(T("Мастер установки"), "app.wizard")
	menu.Append(w.serviceMenuLabel, "app.toggle-service") // надпись меняет refreshService
	menu.Append(T("О программе"), "app.about")
	w.menu = menu
	menuButton := gtk.NewMenuButton()
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetMenuModel(menu)
	menuButton.SetTooltipText(T("Меню"))
	header.PackEnd(menuButton)

	w.toasts = adw.NewToastOverlay()
	w.toasts.SetChild(w.stack)
	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(header)
	toolbar.SetContent(w.toasts)
	w.win.SetContent(toolbar)

	c, err := NewController(w.logFromThread, w.toastFromThread)
	if err != nil {
		dialog := adw.NewAlertDialog(T("Не удалось запустить"), err.Error())
		dialog.AddResponse("close", T("Закрыть"))
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
	w.stack.AddTitledWithIcon(trackersPage, "trackers", T("Трекеры"), "avatar-default-symbolic")

	w.view3d = newView3D(c, w.enabledTrackers)
	w.stack.AddTitledWithIcon(w.view3d.widget, "view3d", T("3D-вид"), "object-rotate-right-symbolic")

	settingsPage := adw.NewPreferencesPage()
	settingsPage.Add(w.buildApp())
	settingsPage.Add(w.buildLog())
	w.stack.AddTitledWithIcon(settingsPage, "settings", T("Настройки"), "preferences-system-symbolic")
	w.addActions()

	w.win.Present()
	w.refresh()
	if justUpdated {
		w.toasts.AddToast(adw.NewToast(Tf("PICO Trackers обновлена до версии %s", appVersion)))
		w.logFromThread(Tf("программа обновлена до версии %s", appVersion))
	}
	// При каждом запуске: есть новая версия — окно обновления (оно обновит и драйвер, и приложение на шлеме),
	// иначе, если нужно, мастер установки. Своя сборка (--dev) обновления не ищет.
	go func() {
		var release *Release
		if devMode {
			w.logFromThread(T("режим --dev: обновления не проверяются, в меню, автозапуск и ~/.local/bin программа себя не ставит"))
		} else {
			os.RemoveAll(updateDir())
			var err error
			if release, err = checkForUpdate(); err != nil {
				w.logFromThread(T("не удалось проверить обновления: ") + err.Error())
			}
		}
		switch {
		case release != nil:
			w.logFromThread(Tf("доступна новая версия %s", release.Version))
			glib.IdleAdd(func() { w.setUpdate(release) })
		case wizardNeeded():
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
	row.SetSubtitleLines(3)
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

	w.heroButton = gtk.NewButtonWithLabel(T("Запустить SteamVR"))
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
	group.SetTitle(T("Подключение"))
	w.headsetRow, w.headsetDot = statusRow(group, T("Шлем PICO"))
	w.serviceButton = gtk.NewButtonWithLabel(T("Включить сервис"))
	w.serviceButton.SetVAlign(gtk.AlignCenter)
	w.serviceButton.ConnectClicked(w.c.ToggleService)
	w.serviceButton.SetVisible(false)
	w.headsetRow.AddSuffix(w.serviceButton)
	w.steamVRRow, w.steamVRDot = statusRow(group, "SteamVR")
	w.steamVRButton = gtk.NewButtonWithLabel(T("Запустить"))
	w.steamVRButton.SetVAlign(gtk.AlignCenter)
	w.steamVRButton.ConnectClicked(w.launchSteamVR)
	w.steamVRRow.AddSuffix(w.steamVRButton)
	w.picoRow, w.picoDot = statusRow(group, T("Трекеры PICO"))
	w.calibrateButton = gtk.NewButtonWithLabel(T("Откалибровать"))
	w.calibrateButton.SetVAlign(gtk.AlignCenter)
	w.calibrateButton.AddCSSClass("suggested-action")
	w.calibrateButton.ConnectClicked(w.c.CalibrateTrackers)
	w.calibrateButton.SetVisible(false)
	w.picoRow.AddSuffix(w.calibrateButton)
	w.calibrationRow, w.calibrationDot = statusRow(group, T("Калибровка"))
	return group
}

func (w *window) buildTracking() *adw.PreferencesGroup {
	s := w.c.Snapshot()
	group := adw.NewPreferencesGroup()
	w.trackingSwitch = adw.NewSwitchRow()
	w.trackingSwitch.SetTitle(T("Передавать трекинг"))
	w.trackingSwitch.SetSubtitle(T("Выключите, чтобы временно убрать все трекеры — шлем перестанет отправлять данные. " +
		"Драйвер помнит это и без программы"))
	w.trackingSwitch.SetActive(s.Tracking)
	w.trackingSwitch.NotifyProperty("active", func() {
		if !w.updating {
			w.c.SetTracking(w.trackingSwitch.Active())
		}
	})
	group.Add(w.trackingSwitch)

	w.rateRow = adw.NewSpinRowWithRange(minRate, maxRate, 10)
	w.rateRow.SetTitle(T("Частота передачи"))
	w.rateRow.SetSubtitle(T("Снимков скелета в секунду: больше — плавнее, но выше нагрузка на Wi-Fi"))
	w.rateRow.SetValue(float64(s.Rate))
	w.rateRow.NotifyProperty("value", func() { w.c.SetRate(int(w.rateRow.Value())) })
	group.Add(w.rateRow)
	return group
}

func (w *window) buildBody() *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle(T("Трекеры"))
	group.SetDescription(T("Применяется сразу: включённый трекер появляется в SteamVR, выключенный отключается " +
		"и пропадает из игр. Зелёный — отслеживается, оранжевый — нет позиции."))
	presets := gtk.NewBox(gtk.OrientationHorizontal, 0)
	presets.AddCSSClass("linked")
	presets.SetVAlign(gtk.AlignCenter)
	for _, preset := range []struct {
		label string
		names []string
	}{{N("Ноги"), legsTrackers}, {N("Основные"), mainTrackers()}, {N("Все"), allTrackers()}} {
		names := preset.names
		button := gtk.NewButtonWithLabel(T(preset.label))
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
	group.SetTitle(T("Дополнительные суставы"))
	group.SetDescription(T("У них нет роли в SteamVR: игры вроде VRChat могут путать их с основными трекерами."))
	expander := adw.NewExpanderRow()
	expander.SetTitle(T("Ещё 14 суставов"))
	expander.SetSubtitle(T("голова, кисти, шея, позвоночник, носки…"))
	for _, t := range trackers {
		if t.HasRole {
			continue
		}
		row := adw.NewSwitchRow()
		row.SetTitle(T(t.Label))
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
	group.SetTitle(T("Программа"))
	group.Add(w.buildLanguage())
	if devMode {
		dev := adw.NewActionRow()
		dev.SetTitle(T("Своя сборка (--dev)"))
		dev.SetSubtitle(T("Программа не встраивается в систему: не ставит себя в меню и ~/.local/bin, не меняет " +
			"автозапуск и не проверяет обновления. Драйвер и приложение на шлем мастер ставит как обычно"))
		dev.SetSubtitleLines(4)
		group.Add(dev)
	} else {
		autostart := adw.NewSwitchRow()
		autostart.SetTitle(T("Запускать при входе в систему"))
		autostart.SetSubtitle(T("Программа будет следить за шлемом и драйвером в фоне"))
		autostart.SetActive(autostartEnabled())
		autostart.NotifyProperty("active", func() { setAutostart(autostart.Active()) })
		group.Add(autostart)
	}

	w.updateRow = adw.NewActionRow()
	w.updateRow.SetTitle(T("Обновление"))
	update := gtk.NewButtonWithLabel(T("Обновить"))
	update.AddCSSClass("suggested-action")
	update.SetVAlign(gtk.AlignCenter)
	update.ConnectClicked(w.openUpdate)
	w.updateRow.AddSuffix(update)
	w.updateRow.SetVisible(false)
	group.Add(w.updateRow)

	wizardRow := adw.NewActionRow()
	wizardRow.SetTitle(T("Мастер установки"))
	wizardRow.SetSubtitle(T("Установить или обновить драйвер SteamVR и приложение на шлеме"))
	button := gtk.NewButtonWithLabel(T("Открыть"))
	button.SetVAlign(gtk.AlignCenter)
	button.ConnectClicked(w.openWizard)
	wizardRow.AddSuffix(button)
	group.Add(wizardRow)

	if canUninstall { // на Windows удаляют через «Параметры → Приложения»
		removeRow := adw.NewActionRow()
		removeRow.SetTitle(T("Удалить программу"))
		switch {
		case devMode:
			removeRow.SetSubtitle(T("Драйвер SteamVR и настройки. Установленную программу, пункт меню и автозапуск режим --dev не трогает"))
		case selfInstall:
			removeRow.SetSubtitle(T("Драйвер SteamVR, программа, пункт меню и настройки. Приложение на шлеме останется"))
		default:
			removeRow.SetSubtitle(Tf("Драйвер SteamVR и настройки; саму программу удалите командой flatpak uninstall %s", appID))
		}
		remove := gtk.NewButtonWithLabel(T("Удалить"))
		remove.AddCSSClass("destructive-action")
		remove.SetVAlign(gtk.AlignCenter)
		remove.ConnectClicked(w.confirmUninstall)
		removeRow.AddSuffix(remove)
		group.Add(removeRow)
	}
	return group
}

// buildLanguage — язык интерфейса; меняется перезапуском программы.
func (w *window) buildLanguage() *adw.ComboRow {
	row := adw.NewComboRow()
	row.SetTitle(T("Язык · Language"))
	values := []string{languageSystem, languageRussian, languageEnglish}
	row.SetModel(gtk.NewStringList([]string{T("Как в системе"), "Русский", "English"}))
	current := w.c.Config().Language
	for i, value := range values {
		if value == current {
			row.SetSelected(uint(i))
		}
	}
	row.NotifyProperty("selected", func() {
		if !w.c.SetLanguage(values[row.Selected()]) {
			return
		}
		toast := adw.NewToast(T("Язык сменится после перезапуска программы"))
		toast.SetButtonLabel(T("Перезапустить"))
		toast.ConnectButtonClicked(w.restart)
		toast.SetTimeout(0)
		w.toasts.AddToast(toast)
	})
	return row
}

// restart — перезапуск программы (смена языка).
func (w *window) restart() {
	err := restartProgram(func() {
		w.c.Close()
		w.win.Close()
	})
	if err != nil {
		w.toasts.AddToast(adw.NewToast(T("Не удалось перезапустить: ") + err.Error()))
	}
}

func (w *window) confirmUninstall() {
	text := T("Драйвер будет снят с регистрации в SteamVR и удалён, программа исчезнет из меню приложений, " +
		"её настройки будут удалены. Приложение на шлеме останется — его можно удалить в настройках шлема.")
	if devMode {
		text = T("Драйвер будет снят с регистрации в SteamVR и удалён, настройки программы — удалены. Установленная " +
			"программа, её пункт меню и автозапуск останутся (режим --dev). Приложение на шлеме тоже останется.")
	}
	dialog := adw.NewAlertDialog(T("Удалить PICO Trackers?"), text)
	dialog.AddResponse("cancel", T("Отмена"))
	dialog.AddResponse("remove", T("Удалить"))
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
					failed := adw.NewAlertDialog(T("Не удалось удалить"), err.Error())
					failed.AddResponse("close", T("Закрыть"))
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
	group.SetTitle(T("Журнал событий"))
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
		"wizard":         w.openWizard,
		"toggle-service": w.c.ToggleService,
		"start-service":  w.c.StartService, // без пункта меню — для D-Bus (org.gtk.Actions.Activate)
		"stop-service":   w.c.StopService,
		"about":          w.showAbout,
	} {
		action := gio.NewSimpleAction(name, nil)
		callback := callback
		action.ConnectActivate(func(*glib.Variant) { callback() })
		w.app.AddAction(action)
		if name == "toggle-service" {
			w.serviceAction = action
			action.SetEnabled(false) // до первой проверки сервиса
		}
	}
}

// ---------- действия ----------

func (w *window) launchSteamVR() {
	launchSteamVR()
	w.toasts.AddToast(adw.NewToast(T("Запускаю SteamVR…")))
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
	about.SetComments(T("PICO Motion Trackers в SteamVR через Steam Link"))
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
	w.refreshPico(s)
	w.refreshCalibration(s)
	w.refreshTrackers(s)
	w.refreshHero(s)
}

func (w *window) refreshHeadset(s Snapshot) {
	var parts []string
	state := "dim-label"
	switch d := s.Driver; {
	case d != nil && d.Data && d.Pico.NotCalibrated():
		parts = append(parts, T("на связи · трекеры не откалиброваны — в SteamVR ничего не передаётся"))
		state = "warning"
	case d != nil && d.Data && !d.TrackersReady:
		parts = append(parts, T("на связи · трекеры НЕ подключены — в SteamVR ничего не передаётся"))
		state = "warning"
	case d != nil && d.Data:
		parts = append(parts, Tf("передаёт %d из %d пак/с · трекеры подключены", d.PacketsPerS, d.Rate))
		state = "success"
	case s.ServiceKnown && !s.ServiceRunning:
		parts = append(parts, T("сервис на шлеме выключен"))
		state = "warning"
	case s.Stats != nil:
		parts = append(parts, Tf("передаёт %s пак/с", s.Stats["sent"]))
		state = "success"
	case s.Listener != "" && !strings.HasPrefix(s.Listener, "tracking ->"):
		parts = append(parts, T("сервис ждёт SteamVR"))
	default:
		parts = append(parts, T("нет данных"))
		if s.SteamVRRunning {
			state = "warning"
		}
	}
	switch {
	case s.AppKnown && s.AppState == AppMissing:
		parts = append(parts, T("приложение не установлено — откройте мастер установки"))
		state = "warning"
	case s.AppKnown && s.AppState == AppOutdated && (s.Driver == nil || !s.Driver.Data):
		parts = append(parts, T("приложение устарело — обновите в мастере установки"))
		state = "warning"
	}
	switch {
	case s.HeadsetSerial != "":
		via := T("adb по USB %s")
		if s.HeadsetWifi {
			via = T("adb по Wi-Fi %s")
		}
		link := fmt.Sprintf(via, strings.Split(s.HeadsetSerial, ":")[0])
		if !s.AdbConnected {
			link += " " + T("(нет связи)")
		}
		parts = append(parts, link)
	case s.HeadsetError != "":
		parts = append(parts, T("adb: шлем не найден"))
		if s.Driver == nil || !s.Driver.Data {
			state = "error"
		}
	}
	w.headsetRow.SetSubtitle(strings.Join(parts, " · "))
	setDot(w.headsetDot, state)
	w.refreshService(s)
}

// serviceMenuUnknown — по-русски: T при выводе (переменная вычисляется до выбора языка).
var serviceMenuUnknown = N("Сервис на шлеме — нет связи по adb")

// refreshService — одна кнопка (строка «Шлем PICO») и один пункт меню на включение и выключение сервиса;
// надпись — по его состоянию на шлеме (adb), независимо от SteamVR.
func (w *window) refreshService(s Snapshot) {
	label, menuLabel := N("Включить сервис"), N("Включить сервис на шлеме")
	switch {
	case !s.ServiceKnown:
		menuLabel = serviceMenuUnknown
	case s.ServiceChange == "start":
		label, menuLabel = N("Включаю…"), N("Сервис на шлеме включается…")
	case s.ServiceChange == "stop":
		label, menuLabel = N("Выключаю…"), N("Сервис на шлеме выключается…")
	case s.ServiceRunning:
		label, menuLabel = N("Выключить сервис"), N("Выключить сервис на шлеме")
	}
	label, menuLabel = T(label), T(menuLabel)
	w.serviceButton.SetLabel(label)
	w.serviceButton.SetVisible(s.ServiceKnown)
	w.serviceButton.SetSensitive(s.ServiceChange == "")
	if s.ServiceRunning {
		w.serviceButton.RemoveCSSClass("suggested-action")
	} else {
		w.serviceButton.AddCSSClass("suggested-action")
	}
	w.serviceAction.SetEnabled(s.ServiceKnown && s.ServiceChange == "")
	if menuLabel != w.serviceMenuLabel {
		w.serviceMenuLabel = menuLabel
		w.menu.Remove(1)
		w.menu.Insert(1, menuLabel, "app.toggle-service")
	}
}

func (w *window) refreshSteamVR(s Snapshot) {
	w.steamVRButton.SetVisible(!s.SteamVRRunning)
	var text, state string
	switch d := s.Driver; {
	case !s.SteamVRRunning:
		text, state = T("не запущен"), "dim-label"
	case d == nil:
		text, state = T("запущен, драйвер не отвечает"), "warning"
		if s.RestartSteamVR {
			text += " — " + T("перезапустите SteamVR")
		}
	default:
		hmd, hmdState := T("шлем не подключён (Steam Link)"), "warning"
		if d.HMD {
			hmd, hmdState = T("шлем подключён"), "success"
		}
		text, state = T("драйвер на связи")+" · "+hmd, hmdState
		if !d.Tracking {
			text += " · " + T("трекинг выключен")
		}
	}
	w.steamVRRow.SetSubtitle(text)
	setDot(w.steamVRDot, state)
}

// refreshPico — трекеры PICO по словам шлема: подключены ли, откалиброваны ли, заряд. Кнопка «Откалибровать» —
// только когда трекеры подключены, но не откалиброваны.
func (w *window) refreshPico(s Snapshot) {
	d := s.Driver
	var p *PicoStatus
	if d != nil && d.Data {
		p = d.Pico
	}
	var parts []string
	state := "dim-label"
	switch {
	case d == nil || !d.Data:
		parts = append(parts, "—")
	case p == nil && d.TrackersReady:
		parts, state = append(parts, T("подключены")), "success"
	case p == nil:
		parts, state = append(parts, T("не подключены")), "warning"
	case p.NotCalibrated():
		parts, state = append(parts, T("подключены, но не откалиброваны")), "warning"
	case p.Status == 1 || p.Status == 2 || d.TrackersReady:
		parts, state = append(parts, T("подключены и откалиброваны")), "success"
		if p.Status == 2 {
			parts, state = append(parts, T("данные неточные")), "warning"
		}
	case p.Count == 0 || p.Error == picoErrorNotEnough:
		parts, state = append(parts, T("не подключены — включите трекеры")), "warning"
	default:
		parts, state = append(parts, Tf("нет данных от трекеров (код %d)", p.Error)), "warning"
	}
	if p != nil && p.Count > 0 {
		parts = append(parts, Tf("подключено: %d", p.Count))
	}
	if levels := p.Batteries(); len(levels) > 0 {
		var texts []string
		low := false
		for _, level := range levels {
			texts = append(texts, Tf("%d %%", level))
			low = low || level < lowBattery
		}
		battery := Tf("заряд %s", strings.Join(texts, ", "))
		if low {
			battery += " — " + T("зарядите трекеры")
			if state == "success" {
				state = "warning"
			}
		}
		parts = append(parts, battery)
	} else if d != nil && d.Data && p == nil && s.AppKnown && s.AppState == AppOutdated {
		parts = append(parts, T("заряд — после обновления приложения на шлеме"))
	}
	w.picoRow.SetSubtitle(strings.Join(parts, " · "))
	setDot(w.picoDot, state)
	w.calibrateButton.SetVisible(p.NotCalibrated() || s.Calibrating)
	w.calibrateButton.SetSensitive(!s.Calibrating)
	if s.Calibrating {
		w.calibrateButton.SetLabel(T("Открываю…"))
	} else {
		w.calibrateButton.SetLabel(T("Откалибровать"))
	}
}

func (w *window) refreshCalibration(s Snapshot) {
	var text, state string
	switch d := s.Driver; {
	case d == nil:
		text, state = "—", "dim-label"
	case d.Data && !d.TrackersReady && !d.Calibration.Valid:
		text, state = T("ждёт трекеры PICO"), "dim-label"
	case d.Calibration.Valid:
		mm := d.Calibration.Error * 1000
		text = Tf("точность %.0f мм · %d замеров · поворот %.1f°", mm, d.Calibration.Samples, d.Calibration.Yaw)
		state = "success"
		if mm >= 30 {
			state = "warning"
		}
	case !d.HMD:
		text, state = T("ждёт позу шлема из Steam Link"), "dim-label"
	default:
		text, state = T("идёт: постойте спокойно и медленно повертите головой"), "warning"
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
			control.setState(T("выключен"), T("выключен"), "dim-label")
		case s.Driver == nil:
			control.setState(T("включён · SteamVR не на связи"), T("нет связи со SteamVR"), "dim-label")
		case s.Driver.Data && s.Driver.Pico.NotCalibrated():
			control.setState(T("трекеры PICO не откалиброваны — в SteamVR не передаётся"), T("трекеры не откалиброваны"), "warning")
		case s.Driver.Data && !s.Driver.TrackersReady:
			control.setState(T("трекеры PICO не подключены — в SteamVR не передаётся"), T("трекеры не подключены"), "warning")
		case !known || !s.Driver.Trackers[i].Enabled:
			control.setState(T("применяется…"), T("применяется…"), "dim-label")
		case s.Driver.Trackers[i].Valid:
			control.setState(T("в SteamVR · отслеживается"), T("отслеживается"), "success")
		case !s.Driver.Trackers[i].Added:
			control.setState(T("появится в SteamVR, когда пойдут данные"), T("ждёт данные"), "dim-label")
		default:
			control.setState(T("в SteamVR · нет позиции"), T("нет позиции"), "warning")
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
		title, text, state = T("Нужна установка"), T("Драйвер SteamVR не установлен или устарел"), "warning"
		w.heroButton.SetLabel(T("Открыть мастер"))
		w.heroAction = w.openWizard
	case s.ServiceKnown && !s.ServiceRunning:
		title, text, state = T("Сервис на шлеме выключен"), T("Без него шлем не передаёт трекинг"), "warning"
		if s.ServiceOff {
			text = T("Выключен вами: сам он не включится, пока не нажмёте кнопку")
		}
		w.heroButton.SetLabel(T("Включить сервис"))
		w.heroAction = w.c.StartService
	case !s.SteamVRRunning:
		title, text, state = T("Ждём SteamVR"), T("Подключитесь со шлема через Steam Link или запустите SteamVR"), "dim-label"
		w.heroButton.SetLabel(T("Запустить SteamVR"))
		w.heroAction = w.launchSteamVR
	case d == nil:
		title, text, state = T("Драйвер не отвечает"), T("SteamVR запускается или драйвер не загружен"), "warning"
		if s.RestartSteamVR {
			text = T("Перезапустите SteamVR, чтобы загрузить новую версию драйвера")
		}
	case !s.Tracking:
		title, text, state = T("Трекинг выключен"), T("Включите переключатель «Передавать трекинг»"), "dim-label"
	case !d.Data:
		title, text, state = T("Ждём данные со шлема"), T("Шлем включён и надет? Сервис PICO Trackers запущен?"), "warning"
	case d.Pico.NotCalibrated():
		title, text, state = T("Трекеры не откалиброваны"),
			T("Нажмите «Откалибровать»: на шлеме откроется калибровка трекеров PICO. Пока её нет, в SteamVR ничего не передаётся"), "warning"
		w.heroButton.SetLabel(T("Откалибровать"))
		if !s.Calibrating {
			w.heroAction = w.c.CalibrateTrackers
		}
	case !d.TrackersReady:
		title, text, state = T("Трекеры не подключены"),
			T("Включите трекеры PICO: пока они не подключены к шлему, в SteamVR ничего не передаётся"), "warning"
	case !d.HMD:
		title, text, state = T("Шлем не подключён к SteamVR"), T("Подключитесь через Steam Link"), "warning"
	case !d.Calibration.Valid:
		title, text, state = T("Калибровка"), T("Постойте спокойно и медленно повертите головой"), "warning"
	case active == 0:
		title, text, state = T("Нет активных трекеров"), T("Включите трекеры на схеме ниже"), "warning"
	default:
		title, state = T("Трекинг работает"), "success"
		text = Tf("%d трекеров в SteamVR · %d пак/с · точность %.0f мм", active, d.PacketsPerS,
			d.Calibration.Error*1000)
	}
	w.heroTitle.SetLabel(title)
	w.heroText.SetLabel(text)
	setDot(w.heroDot, state)
	w.heroButton.SetVisible(w.heroAction != nil)
}
