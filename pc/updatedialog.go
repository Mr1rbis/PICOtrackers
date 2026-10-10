// Окно обновления: открывается при каждом запуске, пока на GitHub есть версия новее (update.go). Обновление
// скачивается и ставится, только когда SteamVR закрыт и шлем подключён кабелем с разрешённой отладкой по USB —
// условия проверяются раз в 2 с. Окно можно закрыть («Позже»), при следующем запуске оно откроется снова;
// открыть его можно и из настроек.
package main

import (
	"context"
	"fmt"
	"html"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// setUpdate — найдена версия новее: строка в настройках и окно обновления.
func (w *window) setUpdate(r *Release) {
	w.update = r
	w.updateRow.SetSubtitle(fmt.Sprintf("Доступна версия %s, установлена %s", r.Version, appVersion))
	w.updateRow.SetVisible(true)
	w.openUpdate()
}

func (w *window) openUpdate() {
	r := w.update
	if r == nil || w.updateOpen {
		return
	}
	w.updateOpen = true

	selfUpdateErr := canSelfUpdate()
	what := "Обновятся программа, драйвер SteamVR и приложение на шлеме. Программа перезапустится."
	switch {
	case selfUpdateErr != nil:
		what = ""
	case !selfInstall:
		what = "Обновятся программа, драйвер SteamVR и приложение на шлеме. Программа закроется, " +
			"установщик обновит её и запустит снова."
	}
	s := newWizardStep("Обновление", "software-update-available-symbolic", "Доступна версия "+r.Version,
		fmt.Sprintf("Установлена %s. %s <a href=\"%s\">Что нового</a>", appVersion, what, html.EscapeString(r.PageURL)))
	nav := adw.NewNavigationView()
	nav.Push(s.page)
	dialog := adw.NewDialog()
	dialog.SetTitle("Обновление")
	dialog.SetContentWidth(580)
	dialog.SetContentHeight(680)
	dialog.SetChild(nav)
	closed := false
	dialog.ConnectClosed(func() { closed, w.updateOpen = true, false })

	s.secondry.SetLabel("Позже")
	s.secondry.SetVisible(true)
	s.secondry.ConnectClicked(func() { dialog.Close() })

	if selfUpdateErr != nil {
		s.showMessage(capitalize(selfUpdateErr.Error()), "dim-label")
		s.next.SetLabel("Страница загрузки")
		s.next.ConnectClicked(func() {
			gtk.NewURILauncher(r.PageURL).Launch(context.Background(), &w.win.Window, nil)
		})
		dialog.Present(w.win)
		return
	}

	steamVRRow, steamVRDot := s.row("SteamVR закрыт")
	headsetRow, headsetDot := s.row("Шлем подключён USB-кабелем")
	s.next.SetLabel("Обновить")
	s.next.SetSensitive(false)

	var conditions UpdateConditions
	busy, polling := false, false
	apply := func(c UpdateConditions) {
		if busy || closed {
			return
		}
		conditions = c
		if c.SteamVRRunning {
			steamVRRow.SetSubtitle("SteamVR запущен — закройте его: драйвер нельзя обновить, пока он загружен")
			setDot(steamVRDot, "error")
		} else {
			steamVRRow.SetSubtitle("Закрыт")
			setDot(steamVRDot, "success")
		}
		switch {
		case !c.AdbFound:
			headsetRow.SetSubtitle("Не найден adb — нажмите «Скачать adb» (Android platform-tools от Google, ~10 МБ)")
			setDot(headsetDot, "warning")
		case c.Serial == "":
			headsetRow.SetSubtitle("Не найден — подключите шлем кабелем и включите в нём «Отладку по USB»")
			setDot(headsetDot, "error")
		case c.State == "unauthorized":
			headsetRow.SetSubtitle("Наденьте шлем и разрешите отладку по USB для этого компьютера")
			setDot(headsetDot, "warning")
		case c.State != "device":
			headsetRow.SetSubtitle(fmt.Sprintf("Шлем %s: %s — переподключите кабель", c.Serial, c.State))
			setDot(headsetDot, "warning")
		default:
			headsetRow.SetSubtitle(fmt.Sprintf("Подключён (%s)", c.Serial))
			setDot(headsetDot, "success")
		}
		if !c.AdbFound {
			s.next.SetLabel("Скачать adb")
			s.next.SetSensitive(true)
		} else {
			s.next.SetLabel("Обновить")
			s.next.SetSensitive(c.Met())
		}
	}
	check := func() {
		if polling || busy {
			return
		}
		polling = true
		go func() {
			c := updateConditions()
			glib.IdleAdd(func() {
				polling = false
				apply(c)
			})
		}()
	}
	check()
	glib.TimeoutAdd(2000, func() bool {
		if closed {
			return false
		}
		check()
		return true
	})

	s.next.ConnectClicked(func() {
		busy = true
		s.setBusy(true)
		s.showMessage("", "dim-label")
		if !conditions.AdbFound {
			go func() {
				err := downloadPlatformTools(w.logFromThread)
				glib.IdleAdd(func() {
					busy = false
					s.setBusy(false)
					if err != nil {
						s.showMessage("Не удалось скачать adb: "+err.Error(), "error")
					}
					check()
				})
			}()
			return
		}
		dialog.SetCanClose(false) // не прерывать установку на полпути
		status := func(text string) { glib.IdleAdd(func() { s.showMessage(text, "dim-label") }) }
		log := func(text string) {
			w.logFromThread(text)
			status(text)
		}
		go func() {
			restart, err := r.Install(log, status)
			installed := err == nil
			if installed {
				err = restart()
			}
			glib.IdleAdd(func() {
				if err == nil {
					w.app.Quit()
					return
				}
				busy = false
				s.setBusy(false)
				dialog.SetCanClose(true)
				if installed {
					s.showMessage("Обновление установлено, но перезапустить программу не удалось — запустите её снова: "+
						err.Error(), "error")
				} else {
					s.showMessage("Не удалось обновить: "+err.Error(), "error")
				}
				check()
			})
		}()
	})
	dialog.Present(w.win)
}
