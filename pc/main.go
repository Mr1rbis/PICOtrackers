// PICO Trackers — программа на ПК: ставит и обновляет драйвер SteamVR и приложение на шлеме,
// держит связь с ними и позволяет на лету включать и выключать трекеры.
//
//	picotrackers          окно программы
//	picotrackers setup    установить драйвер SteamVR и приложение на шлем без мастера
//	picotrackers stop     выключить сервис на шлеме
//
// --dev — своя сборка для проверки (pc/build.sh): всё работает как обычно (драйвер, шлем, мастер), но программа
// не встраивается в систему — не копирует себя в ~/.local/bin, не создаёт пункт меню, не меняет автозапуск и не
// проверяет обновления (иначе предложила бы заменить себя релизом). Установленная версия, её пункт меню и
// автозапуск остаются как были.
//
// Для установщика Windows (windows/installer.iss), без вопросов и вывода в консоль:
//
//	picotrackers install-driver [--message-file=F]  установить драйвер SteamVR
//	picotrackers uninstall [--message-file=F]       снять драйвер с регистрации, удалить данные программы
//
// Сообщение для пользователя (ошибка или просьба перезапустить SteamVR) пишется в F (UTF-8, готовое к показу).
// Коды выхода: 0 — готово, 1 — ошибка, 3 — мешает запущенный SteamVR.
//
// После обновления (update.go) окно запускается с --updated (сообщить о новой версии), на Linux ещё
// с --wait-pid=N — дождаться выхода прошлой версии.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	messageFile string // куда команды для установщика пишут сообщение для пользователя
	waitPID     int    // --wait-pid: процесс прошлой версии программы
	justUpdated bool   // --updated: программу только что обновили
	devMode     bool   // --dev: своя сборка для проверки — в систему не встраивается
)

// devArgs — флаги, с которыми программа перезапускается (смена языка): режим --dev сохраняется.
func devArgs() []string {
	if devMode {
		return []string{"--dev"}
	}
	return nil
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelWarn) // GTK пишет в slog отладочные сообщения Vulkan и т.п.
	setupLanguage()
	command := ""
	for _, arg := range os.Args[1:] {
		switch {
		case strings.HasPrefix(arg, "--message-file="):
			messageFile = strings.TrimPrefix(arg, "--message-file=")
		case strings.HasPrefix(arg, "--wait-pid="):
			waitPID, _ = strconv.Atoi(strings.TrimPrefix(arg, "--wait-pid="))
		case arg == "--updated":
			justUpdated = true
		case arg == "--dev":
			devMode = true
		case command == "":
			command = arg
		}
	}
	if devMode {
		selfInstall = false // нет шага «Программа» в мастере: себя не копирует и пункт меню не создаёт
	}
	if command != "" {
		restoreHostEnv() // окно возвращает его после запуска GTK (runGUI)
	}
	switch command {
	case "":
		os.Exit(runGUI())
	case "setup":
		exit(cliSetup())
	case "stop":
		exit(cliStop())
	case "install-driver":
		err := installDriver(cliLog)
		if err == nil && steamVRNeedsRestart() {
			message(T("SteamVR уже запущен — перезапустите его, чтобы трекеры появились."))
		}
		exit(err)
	case "uninstall":
		exit(uninstall(cliLog))
	default:
		fmt.Fprintln(os.Stderr, T("использование: picotrackers [--dev] [setup|stop|install-driver|uninstall]"))
		os.Exit(2)
	}
}

func message(text string) {
	fmt.Println("!", text)
	writeMessageFile(text)
}

// writeMessageFile — сообщение для установщика: готовое к показу (с заглавной буквы, без перевода строки),
// потому что Trim/Copy/AnsiUppercase в Pascal Script Inno Setup теряют кириллицу.
func writeMessageFile(text string) {
	if messageFile == "" || text == "" {
		return
	}
	os.WriteFile(messageFile, []byte(capitalize(text)), 0o644)
}

// capitalize — текст ошибки для показа: с заглавной буквы.
func capitalize(text string) string {
	first, size := utf8.DecodeRuneInString(text)
	if size == 0 {
		return text
	}
	return string(unicode.ToUpper(first)) + text[size:]
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		writeMessageFile(err.Error())
		if errors.Is(err, errSteamVRRunning) {
			os.Exit(3)
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func cliLog(text string) { fmt.Println("✓", text) }

// cliSetup — то же, что мастер установки, без вопросов: драйвер, затем приложение на шлем (если подключён).
func cliSetup() error {
	if err := installDriver(cliLog); err != nil {
		return err
	}
	if steamVRNeedsRestart() {
		fmt.Println("!", T("SteamVR запущен со старой версией драйвера — перезапустите SteamVR"))
	}
	if adbPath() == "" {
		if err := downloadPlatformTools(cliLog); err != nil {
			return err
		}
	}
	cfg := loadConfig()
	headset, err := connectHeadset(cfg)
	if err != nil {
		fmt.Println("!", T("шлем не найден по adb — приложение на шлем не установлено"))
		return nil
	}
	if appState(headset) != AppInstalled {
		if err := installApp(headset, cliLog); err != nil {
			return err
		}
	}
	allowBackgroundCalibration(headset)
	if !headset.OverWifi() {
		headset = switchToWifi(headset, cfg, cliLog)
	}
	if cfg.ServiceOff {
		fmt.Println("!", T("сервис на шлеме выключен вами — включите его кнопкой «Включить сервис» в программе"))
		return nil
	}
	if err := startService(headset); err != nil {
		return err
	}
	cliLog(T("сервис на шлеме запущен: трекинг включится, когда SteamVR загрузит драйвер"))
	return nil
}

// cliStop — как кнопка «Выключить сервис»: выбор запоминается и в программе, и на шлеме.
func cliStop() error {
	cfg := loadConfig()
	headset, err := connectHeadset(cfg)
	if err != nil {
		return err
	}
	if err := setServiceEnabled(headset, false); err != nil {
		return err
	}
	cfg.ServiceOff = true
	cfg.save()
	cliLog(T("сервис на шлеме выключен — сам он не включится, пока не нажмёте «Включить сервис» в программе"))
	return nil
}
