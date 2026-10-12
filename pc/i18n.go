// Язык интерфейса: русский или английский. Тексты в коде — по-русски, T() отдаёт перевод из i18n_en.go.
// Язык — из настроек (Настройки → Язык), по умолчанию как в системе: русский, если язык системы русский,
// иначе английский. i18n_test.go проверяет, что у каждого T("…") есть перевод.
package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	languageSystem  = ""
	languageRussian = "ru"
	languageEnglish = "en"
)

var english bool

// setupLanguage — при запуске: язык из настроек или системы.
func setupLanguage() {
	language := loadConfig().Language
	if language == languageSystem {
		language = systemLanguage()
	}
	english = language != languageRussian
}

// localeLanguage — язык по переменным окружения (LANGUAGE, LC_ALL, LC_MESSAGES, LANG).
func localeLanguage() string {
	for _, name := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.Split(os.Getenv(name), ":")[0]
		if value == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(value), "ru") {
			return languageRussian
		}
		return languageEnglish
	}
	return ""
}

// T — текст на языке интерфейса.
func T(russian string) string {
	if english {
		if text, ok := translations[russian]; ok {
			return text
		}
	}
	return russian
}

// N помечает текст для перевода там, где T вызывается позже (i18n_test.go видит и такие тексты).
func N(russian string) string { return russian }

// Tf — T для строки формата.
func Tf(format string, args ...any) string { return fmt.Sprintf(T(format), args...) }
