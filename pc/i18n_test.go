package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// literal — строка из литералов: "a" или "a" + "b" (длинные тексты в коде разбиты на части).
func literal(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		a, ok1 := literal(e.X)
		b, ok2 := literal(e.Y)
		return a + b, ok1 && ok2
	case *ast.ParenExpr:
		return literal(e.X)
	}
	return "", false
}

// translatable — тексты из T("…"), Tf("…") и N("…") во всех файлах программы (кроме тестов).
func translatable(t *testing.T) map[string]string {
	files, _ := filepath.Glob("*.go")
	keys := map[string]string{}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || (ident.Name != "T" && ident.Name != "Tf" && ident.Name != "N") {
				return true
			}
			text, ok := literal(call.Args[0])
			if !ok {
				return true // T(переменная): сам текст помечен N("…") там, где задан
			}
			keys[text] = fset.Position(call.Pos()).String()
			return true
		})
	}
	return keys
}

var verbRe = regexp.MustCompile(`%[-+# 0]*[0-9]*(\.[0-9]+)?[a-zA-Z%]`)

func TestTranslations(t *testing.T) {
	keys := translatable(t)
	var missing []string
	for key, where := range keys {
		en, ok := translations[key]
		if !ok {
			missing = append(missing, strconv.Quote(key)+": \"\", // "+where)
			continue
		}
		if a, b := verbRe.FindAllString(key, -1), verbRe.FindAllString(en, -1); strings.Join(a, " ") != strings.Join(b, " ") {
			t.Errorf("%s: разные спецификаторы формата %v и %v в переводе %q", where, a, b, en)
		}
		if en == "" {
			t.Errorf("%s: пустой перевод %q", where, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("нет перевода (%d):\n%s", len(missing), strings.Join(missing, "\n"))
	}
	for key := range translations {
		if _, ok := keys[key]; !ok {
			t.Errorf("перевод не используется: %q", key)
		}
	}
}

func TestLanguageSwitch(t *testing.T) {
	defer func(was bool) { english = was }(english)
	english = false
	if T("Трекеры") != "Трекеры" {
		t.Fatal("русский текст изменился")
	}
	english = true
	if T("Трекеры") != "Trackers" || Tf("Найден: %s", "/usr/bin/adb") != "Found: /usr/bin/adb" {
		t.Fatalf("английский: %q, %q", T("Трекеры"), Tf("Найден: %s", "/usr/bin/adb"))
	}
	if T("нет такого текста") != "нет такого текста" {
		t.Fatal("текст без перевода должен остаться как есть")
	}
}

func TestLocaleLanguage(t *testing.T) {
	for _, c := range []struct{ language, lcAll, lang, want string }{
		{"", "", "ru_RU.UTF-8", languageRussian},
		{"", "", "en_US.UTF-8", languageEnglish},
		{"", "C", "ru_RU.UTF-8", languageEnglish},
		{"ru:en", "", "en_US.UTF-8", languageRussian},
		{"", "", "", ""},
	} {
		t.Setenv("LANGUAGE", c.language)
		t.Setenv("LC_ALL", c.lcAll)
		t.Setenv("LC_MESSAGES", "")
		t.Setenv("LANG", c.lang)
		if got := localeLanguage(); got != c.want {
			t.Errorf("LANGUAGE=%q LC_ALL=%q LANG=%q: %q, ждал %q", c.language, c.lcAll, c.lang, got, c.want)
		}
	}
}
