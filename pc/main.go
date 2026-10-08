// PICO Trackers — программа на ПК: ставит и обновляет драйвер SteamVR и приложение на шлеме,
// держит связь с ними и позволяет на лету включать и выключать трекеры.
//
//	picotrackers          окно программы
//	picotrackers setup    установить драйвер SteamVR и приложение на шлем без мастера
//	picotrackers stop     выключить сервис на шлеме
//
// Для установщика Windows (windows/installer.iss), без вопросов и вывода в консоль:
//
//	picotrackers install-driver [--message-file=F]  установить драйвер SteamVR
//	picotrackers uninstall [--message-file=F]       снять драйвер с регистрации, удалить данные программы
//
// Сообщение для пользователя (ошибка или просьба перезапустить SteamVR) пишется в F (UTF-8, готовое к показу).
// Коды выхода: 0 — готово, 1 — ошибка, 3 — мешает запущенный SteamVR.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// messageFile — куда команды для установщика пишут сообщение для пользователя.
var messageFile string

func main() {
	slog.SetLogLoggerLevel(slog.LevelWarn) // GTK пишет в slog отладочные сообщения Vulkan и т.п.
	if len(os.Args) > 1 {
		for _, arg := range os.Args[2:] {
			if file, ok := strings.CutPrefix(arg, "--message-file="); ok {
				messageFile = file
			}
		}
		switch os.Args[1] {
		case "setup":
			exit(cliSetup())
		case "stop":
			exit(cliStop())
		case "install-driver":
			err := installDriver(cliLog)
			if err == nil && steamVRNeedsRestart() {
				message("SteamVR уже запущен — перезапустите его, чтобы трекеры появились.")
			}
			exit(err)
		case "uninstall":
			exit(uninstall(cliLog))
		default:
			fmt.Fprintln(os.Stderr, "использование: picotrackers [setup|stop|install-driver|uninstall]")
			os.Exit(2)
		}
	}
	os.Exit(runGUI())
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
	first, size := utf8.DecodeRuneInString(text)
	os.WriteFile(messageFile, []byte(string(unicode.ToUpper(first))+text[size:]), 0o644)
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
		fmt.Println("! SteamVR запущен со старой версией драйвера — перезапустите SteamVR")
	}
	if adbPath() == "" {
		if err := downloadPlatformTools(cliLog); err != nil {
			return err
		}
	}
	cfg := loadConfig()
	headset, err := connectHeadset(cfg)
	if err != nil {
		fmt.Println("! шлем не найден по adb — приложение на шлем не установлено")
		return nil
	}
	if appState(headset) != AppInstalled {
		if err := installApp(headset, cliLog); err != nil {
			return err
		}
	}
	if !headset.OverWifi() {
		headset = switchToWifi(headset, cfg, cliLog)
	}
	if err := startService(headset); err != nil {
		return err
	}
	cliLog("сервис на шлеме запущен: трекинг включится, когда SteamVR загрузит драйвер")
	return nil
}

func cliStop() error {
	headset, err := connectHeadset(loadConfig())
	if err != nil {
		return err
	}
	stopService(headset)
	cliLog("сервис на шлеме выключен")
	return nil
}
