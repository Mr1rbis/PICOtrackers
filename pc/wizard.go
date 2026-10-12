// Мастер установки: по шагам с кнопкой «Далее» — сама программа (только Linux: копия в ~/.local/bin, значок,
// пункт меню; на Windows это делает установщик), драйвер SteamVR, приложение на шлем (если шлем подключён
// кабелем и включена отладка по USB), итог. Открывается при первом запуске, когда драйвер не установлен
// или устарел, когда подключён шлем без актуального приложения, и из настроек.
package main

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type wizard struct {
	w      *window
	dialog *adw.Dialog
	nav    *adw.NavigationView

	programInstalled bool
	programMenu      bool
	driverInstalled  bool
	appInstalled     bool
	appSkipped       bool
	polling          bool // идёт фоновая проверка шлема
	closed           bool
}

// wizardStep — страница мастера: значок, заголовок, пояснение, строки состояния и кнопки внизу.
type wizardStep struct {
	page     *adw.NavigationPage
	status   *adw.StatusPage
	rows     *adw.PreferencesGroup
	message  *gtk.Label
	spinner  *adw.Spinner
	buttons  *gtk.Box
	next     *gtk.Button
	secondry *gtk.Button
}

func newWizardStep(title, icon, heading, description string) *wizardStep {
	s := &wizardStep{}
	s.status = adw.NewStatusPage()
	s.status.SetIconName(icon)
	s.status.SetTitle(heading)
	s.status.SetDescription(description)

	content := gtk.NewBox(gtk.OrientationVertical, 18)
	s.rows = adw.NewPreferencesGroup()
	content.Append(s.rows)
	s.message = gtk.NewLabel("")
	s.message.SetWrap(true)
	s.message.SetVisible(false)
	content.Append(s.message)
	s.spinner = adw.NewSpinner()
	s.spinner.SetVisible(false)
	content.Append(s.spinner)

	s.buttons = gtk.NewBox(gtk.OrientationHorizontal, 12)
	s.buttons.SetHAlign(gtk.AlignCenter)
	s.secondry = gtk.NewButtonWithLabel("")
	s.secondry.AddCSSClass("pill")
	s.secondry.SetVisible(false)
	s.next = gtk.NewButtonWithLabel(T("Далее"))
	s.next.AddCSSClass("suggested-action")
	s.next.AddCSSClass("pill")
	s.buttons.Append(s.secondry)
	s.buttons.Append(s.next)
	content.Append(s.buttons)
	s.status.SetChild(content)

	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(adw.NewHeaderBar())
	toolbar.SetContent(s.status)
	s.page = adw.NewNavigationPage(toolbar, title)
	return s
}

func (s *wizardStep) row(title string) (*adw.ActionRow, *gtk.Label) {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitleLines(3)
	dot := newDot()
	row.AddPrefix(dot)
	s.rows.Add(row)
	return row, dot
}

// showMessage — текст под строками: class "error" для ошибок, "dim-label" для подсказок.
func (s *wizardStep) showMessage(text, class string) {
	s.message.SetLabel(text)
	s.message.RemoveCSSClass("error")
	s.message.RemoveCSSClass("dim-label")
	s.message.AddCSSClass(class)
	s.message.SetVisible(text != "")
}

func (s *wizardStep) setBusy(busy bool) {
	s.spinner.SetVisible(busy)
	s.next.SetSensitive(!busy)
	s.secondry.SetSensitive(!busy)
}

func (w *window) openWizard() {
	if w.wizard != nil && !w.wizard.closed {
		return
	}
	z := &wizard{w: w}
	w.wizard = z
	z.nav = adw.NewNavigationView()
	z.dialog = adw.NewDialog()
	z.dialog.SetTitle(T("Мастер установки"))
	z.dialog.SetContentWidth(580)
	z.dialog.SetContentHeight(640)
	z.dialog.SetChild(z.nav)
	z.dialog.ConnectClosed(func() { z.closed = true })
	z.nav.Push(z.welcomeStep())
	z.dialog.Present(w.win)
}

func (z *wizard) welcomeStep() *adw.NavigationPage {
	what := T("Мастер установит драйвер SteamVR и приложение на шлем PICO.")
	if selfInstall {
		what = T("Мастер установит программу, драйвер SteamVR и приложение на шлем PICO.")
	}
	s := newWizardStep(T("Добро пожаловать"), appID, T("Установка PICO Trackers"),
		what+" "+T("Потом трекинг будет включаться сам, когда вы подключаетесь к компьютеру через Steam Link."))
	s.next.ConnectClicked(func() {
		if selfInstall {
			z.nav.Push(z.programStep())
		} else {
			z.nav.Push(z.driverStep())
		}
	})
	return s.page
}

// ---------- шаг 0 (Linux): сама программа ----------

func (z *wizard) programStep() *adw.NavigationPage {
	s := newWizardStep(T("Программа"), appID, T("Программа"),
		T("Копия программы в домашней папке и пункт в меню приложений. Права администратора не нужны."))
	row, dot := s.row("PICO Trackers")
	menu := adw.NewSwitchRow()
	menu.SetTitle(T("Пункт в меню приложений"))
	menu.SetSubtitle(T("Со значком — чтобы запускать программу из меню"))
	s.rows.Add(menu)
	s.secondry.SetLabel(T("Пропустить"))
	s.secondry.SetVisible(true)

	state := programState()
	menu.SetActive(state.InMenu || !state.Installed)
	switch {
	case state.Current:
		row.SetSubtitle(Tf("Установлена: %s", installedExecutable()))
		setDot(dot, "success")
		s.next.SetLabel(T("Далее"))
	case state.Installed:
		row.SetSubtitle(Tf("Будет обновлена: %s", installedExecutable()))
		setDot(dot, "warning")
		s.next.SetLabel(T("Обновить и далее"))
	default:
		row.SetSubtitle(Tf("Будет установлена в %s", installedExecutable()))
		setDot(dot, "warning")
		s.next.SetLabel(T("Установить и далее"))
	}

	s.secondry.ConnectClicked(func() { z.nav.Push(z.driverStep()) })
	s.next.ConnectClicked(func() {
		s.setBusy(true)
		s.showMessage("", "dim-label")
		addMenu := menu.Active()
		go func() {
			err := installProgram(addMenu, z.w.logFromThread)
			glib.IdleAdd(func() {
				s.setBusy(false)
				if err != nil {
					s.showMessage(T("Не удалось установить: ")+err.Error(), "error")
					return
				}
				z.programInstalled, z.programMenu = true, addMenu
				z.nav.Push(z.driverStep())
			})
		}()
	})
	return s.page
}

// ---------- шаг 1: драйвер SteamVR ----------

func (z *wizard) driverStep() *adw.NavigationPage {
	s := newWizardStep(T("Драйвер SteamVR"), "preferences-system-symbolic", T("Драйвер SteamVR"),
		T("Через него трекеры со шлема появляются в SteamVR и играх."))
	row, dot := s.row(T("Драйвер picotrackers"))

	refresh := func() {
		state := driverState()
		s.showMessage("", "dim-label")
		s.next.SetSensitive(true)
		switch {
		case !state.SteamVRFound:
			row.SetSubtitle(T("SteamVR не найден: установите SteamVR в Steam и запустите его хотя бы раз"))
			setDot(dot, "error")
			s.next.SetLabel(T("Проверить снова"))
		case state.Ready():
			row.SetSubtitle(T("Уже установлен и зарегистрирован в SteamVR") + "\n" + driverInstallDir())
			setDot(dot, "success")
			s.next.SetLabel(T("Далее"))
		default:
			what := Tf("Будет установлен в %s", driverInstallDir())
			if state.Installed && !state.UpToDate {
				what = T("Будет обновлён до новой версии")
			}
			if len(state.Others) > 0 {
				what += "\n" + T("Старые копии драйвера будут отключены")
			}
			row.SetSubtitle(what)
			setDot(dot, "warning")
			s.next.SetLabel(T("Установить и далее"))
		}
	}
	refresh()

	s.next.ConnectClicked(func() {
		state := driverState()
		switch {
		case !state.SteamVRFound:
			refresh()
			return
		case state.Ready():
			z.driverInstalled = true
			z.nav.Push(z.headsetStep())
			return
		}
		s.setBusy(true)
		go func() {
			err := installDriver(z.w.logFromThread)
			glib.IdleAdd(func() {
				s.setBusy(false)
				refresh()
				if err != nil {
					s.showMessage(T("Не удалось установить: ")+err.Error(), "error")
					return
				}
				z.driverInstalled = true
				z.nav.Push(z.headsetStep())
			})
		}()
	})
	return s.page
}

// ---------- шаг 2: приложение на шлем ----------

func (z *wizard) headsetStep() *adw.NavigationPage {
	s := newWizardStep(T("Шлем"), "drive-removable-media-symbolic", T("Приложение на шлем"),
		T("Подключите шлем к компьютеру USB-кабелем. В шлеме включите режим разработчика и «Отладку по USB», "+
			"затем наденьте шлем и разрешите отладку для этого компьютера."))
	adbRow, adbDot := s.row("adb (Android Debug Bridge)")
	headsetRow, headsetDot := s.row(T("Шлем PICO"))
	s.secondry.SetLabel(T("Пропустить"))
	s.secondry.SetVisible(true)

	var serial string
	var state AppState
	busy := false

	apply := func(adbFound bool, foundSerial, adbState string, app AppState) {
		if busy || z.closed {
			return
		}
		serial, state = foundSerial, app
		if adbFound {
			adbRow.SetSubtitle(Tf("Найден: %s", adbPath()))
			setDot(adbDot, "success")
		} else {
			adbRow.SetSubtitle(T("Не найден — нажмите «Скачать adb» (Android platform-tools от Google, ~10 МБ)"))
			setDot(adbDot, "warning")
		}
		s.next.SetSensitive(true)
		switch {
		case !adbFound:
			headsetRow.SetSubtitle(T("Сначала нужен adb"))
			setDot(headsetDot, "dim-label")
			s.next.SetLabel(T("Скачать adb"))
		case foundSerial == "":
			headsetRow.SetSubtitle(T("Не найден — подключите кабель и включите отладку по USB"))
			setDot(headsetDot, "dim-label")
			s.next.SetLabel(T("Установить и далее"))
			s.next.SetSensitive(false)
		case adbState == "unauthorized":
			headsetRow.SetSubtitle(T("Наденьте шлем и разрешите отладку по USB для этого компьютера"))
			setDot(headsetDot, "warning")
			s.next.SetLabel(T("Установить и далее"))
			s.next.SetSensitive(false)
		case adbState != "device":
			headsetRow.SetSubtitle(Tf("Шлем %s: %s — переподключите кабель", foundSerial, adbState))
			setDot(headsetDot, "warning")
			s.next.SetSensitive(false)
		case app == AppInstalled:
			headsetRow.SetSubtitle(Tf("Подключён (%s) · приложение уже установлено", foundSerial))
			setDot(headsetDot, "success")
			s.next.SetLabel(T("Далее"))
		default:
			what := T("приложение будет установлено")
			if app == AppOutdated {
				what = T("приложение будет обновлено")
			}
			headsetRow.SetSubtitle(Tf("Подключён (%s) · %s", foundSerial, what))
			setDot(headsetDot, "success")
			s.next.SetLabel(T("Установить и далее"))
		}
	}

	// Проверка шлема раз в 2 с, пока открыт этот шаг.
	check := func() {
		if z.polling || busy {
			return
		}
		z.polling = true
		go func() {
			adbFound := adbPath() != ""
			var found, devState string
			app := AppMissing
			if adbFound {
				found, devState = usbHeadset()
				if found == "" {
					cfg := loadConfig()
					if cfg.AdbWifi != "" && adbDevices()[cfg.AdbWifi] == "device" {
						found, devState = cfg.AdbWifi, "device"
					}
				}
				if devState == "device" {
					app = appState(&Headset{found})
				}
			}
			glib.IdleAdd(func() {
				z.polling = false
				apply(adbFound, found, devState, app)
			})
		}()
	}
	check()
	glib.TimeoutAdd(2000, func() bool {
		if z.closed || z.appInstalled || z.appSkipped {
			return false
		}
		check()
		return true
	})

	s.secondry.ConnectClicked(func() {
		z.appSkipped = true
		z.nav.Push(z.doneStep())
	})
	s.next.ConnectClicked(func() {
		if adbPath() == "" {
			busy = true
			s.setBusy(true)
			go func() {
				err := downloadPlatformTools(z.w.logFromThread)
				glib.IdleAdd(func() {
					busy = false
					s.setBusy(false)
					if err != nil {
						s.showMessage(T("Не удалось скачать adb: ")+err.Error(), "error")
					}
					check()
				})
			}()
			return
		}
		if state == AppInstalled {
			z.appInstalled = true
			z.nav.Push(z.doneStep())
			return
		}
		busy = true
		s.setBusy(true)
		s.showMessage("", "dim-label")
		headset := &Headset{serial}
		go func() {
			cfg := loadConfig()
			err := installApp(headset, z.w.logFromThread)
			if err == nil {
				target := headset
				if !headset.OverWifi() {
					target = switchToWifi(headset, cfg, z.w.logFromThread)
				}
				if !cfg.ServiceOff { // выключен кнопкой — не включаем
					err = startService(target)
				}
			}
			glib.IdleAdd(func() {
				busy = false
				s.setBusy(false)
				if err != nil {
					s.showMessage(T("Не удалось установить: ")+err.Error(), "error")
					return
				}
				z.appInstalled = true
				z.nav.Push(z.doneStep())
			})
		}()
	})
	return s.page
}

// ---------- итог ----------

func (z *wizard) doneStep() *adw.NavigationPage {
	s := newWizardStep(T("Готово"), "object-select-symbolic", T("Готово"),
		T("Подключитесь со шлема через Steam Link — трекинг включится сам."))
	if selfInstall {
		programRow, programDot := s.row(T("Программа"))
		switch {
		case !z.programInstalled:
			programRow.SetSubtitle(T("Не установлена — запускайте этот файл (мастер можно открыть позже)"))
			setDot(programDot, "dim-label")
		default:
			text := Tf("Установлена: %s", installedExecutable())
			if z.programMenu {
				text += "\n" + T("Есть в меню приложений: «PICO Trackers»")
			}
			if !programState().Running {
				text += "\n" + T("Скачанный файл больше не нужен — его можно удалить")
			}
			programRow.SetSubtitle(text)
			setDot(programDot, "success")
		}
	}
	driverRow, driverDot := s.row(T("Драйвер SteamVR"))
	if steamVRNeedsRestart() {
		driverRow.SetSubtitle(T("Установлен. SteamVR уже запущен — перезапустите его, чтобы загрузить драйвер"))
		setDot(driverDot, "warning")
	} else {
		driverRow.SetSubtitle(T("Установлен и зарегистрирован"))
		setDot(driverDot, "success")
	}
	appRow, appDot := s.row(T("Приложение на шлеме"))
	if z.appInstalled {
		appRow.SetSubtitle(T("Установлено, сервис запущен. USB-кабель можно отключить"))
		setDot(appDot, "success")
	} else {
		appRow.SetSubtitle(T("Пропущено — мастер можно открыть позже: Настройки → Мастер установки"))
		setDot(appDot, "dim-label")
	}
	s.next.SetLabel(T("Начать"))
	s.next.ConnectClicked(func() {
		cfg := loadConfig()
		cfg.WizardDone = true
		cfg.save()
		z.w.c.Reconnect()
		z.dialog.Close()
	})
	return s.page
}

// wizardNeeded — нужно ли открыть мастер при запуске (вызывается не из главного потока).
func wizardNeeded() bool {
	if !loadConfig().WizardDone || !driverState().Ready() {
		return true
	}
	if adbPath() == "" {
		return false
	}
	serial, devState := usbHeadset()
	return serial != "" && devState == "device" && appState(&Headset{serial}) != AppInstalled
}
