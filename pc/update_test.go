package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVersionNewer(t *testing.T) {
	for _, c := range []struct {
		version, than string
		newer         bool
	}{
		{"1.4", "1.3", true},
		{"1.10", "1.9", true},
		{"2.0", "1.99", true},
		{"1.3.1", "1.3", true},
		{"1.3", "1.3", false},
		{"1.4.0", "1.4", false},
		{"1.2", "1.3", false},
		{"1.4-rc1", "1.3", false}, // не версия из чисел — не обновление
		{"", "1.3", false},
	} {
		if got := versionNewer(c.version, c.than); got != c.newer {
			t.Errorf("versionNewer(%q, %q) = %v", c.version, c.than, got)
		}
	}
}

func TestParseSums(t *testing.T) {
	sums := parseSums([]byte("ABC123  picotrackers-linux-x86_64\ndef456 *PicoTrackers.apk\n\nмусор\n"))
	if sums["picotrackers-linux-x86_64"] != "abc123" || sums["PicoTrackers.apk"] != "def456" || len(sums) != 2 {
		t.Fatalf("%v", sums)
	}
}

// programAssetName — имя программы в релизе для системы, на которой идёт тест.
func programAssetName() string {
	if isProgramAsset("picotrackers-linux-x86_64") {
		return "picotrackers-linux-x86_64"
	}
	return "PICOtrackers-setup-9.9.exe"
}

// fakeRelease — сервер с последним релизом tag и файлами files (имя → содержимое) и SHA256SUMS.txt к ним.
func fakeRelease(t *testing.T, tag string, files map[string]string) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			type asset struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
				Size int    `json:"size"`
			}
			release := struct {
				Tag    string  `json:"tag_name"`
				Page   string  `json:"html_url"`
				Assets []asset `json:"assets"`
			}{Tag: tag, Page: server.URL + "/page"}
			for name, content := range files {
				release.Assets = append(release.Assets, asset{name, server.URL + "/files/" + name, len(content)})
			}
			json.NewEncoder(w).Encode(release)
			return
		}
		content, ok := files[strings.TrimPrefix(r.URL.Path, "/files/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(content))
	}))
	t.Cleanup(server.Close)
	t.Setenv("PICOTRACKERS_UPDATE_URL", server.URL+"/latest")
}

func sumsFor(files map[string]string) string {
	var lines []string
	for name, content := range files {
		sum := sha256.Sum256([]byte(content))
		lines = append(lines, hex.EncodeToString(sum[:])+"  "+name)
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestCheckForUpdate(t *testing.T) {
	files := map[string]string{programAssetName(): "новая программа", apkAsset: "apk", "PICOtrackers-windows.zip": "zip"}
	files[sumsAsset] = sumsFor(files)

	fakeRelease(t, "v99.0", files)
	r, err := checkForUpdate()
	if err != nil || r == nil {
		t.Fatalf("новая версия не найдена: %v, %v", r, err)
	}
	if r.Version != "99.0" || r.program.Name != programAssetName() || !strings.HasSuffix(r.PageURL, "/page") {
		t.Fatalf("%+v", r)
	}

	// Файлы скачиваются и сверяются с SHA256SUMS.txt.
	sumsData, err := download(r.sums, nil)
	if err != nil {
		t.Fatal(err)
	}
	sums := parseSums(sumsData)
	var reported int64
	data, err := downloadVerified(r.program, sums, "программа", func(string) { reported++ })
	if err != nil || string(data) != "новая программа" || reported == 0 {
		t.Fatalf("загрузка: %q, %v, отчётов о ходе %d", data, err, reported)
	}
	sums[r.program.Name] = strings.Repeat("0", 64)
	if _, err := downloadVerified(r.program, sums, "программа", func(string) {}); err == nil {
		t.Fatal("файл с чужой контрольной суммой принят")
	}

	// Та же или старая версия — обновления нет.
	for _, tag := range []string{"v" + appVersion, "v0.1"} {
		fakeRelease(t, tag, files)
		if r, err := checkForUpdate(); r != nil || err != nil {
			t.Fatalf("%s: %v, %v", tag, r, err)
		}
	}

	// В релизе нет программы для этой системы — обновление не предлагается.
	delete(files, programAssetName())
	fakeRelease(t, "v99.0", files)
	if r, err := checkForUpdate(); r != nil || err == nil {
		t.Fatalf("релиз без программы: %v, %v", r, err)
	}
}

// Новая версия после обновления ждёт выхода прошлой: и живой, и уже завершённой, но не «забранной» родителем.
func TestWaitForExit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("проверяется на Linux")
	}
	running := exec.Command("sleep", "0.5")
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	go running.Wait()
	start := time.Now()
	waitForExit(running.Process.Pid, 5*time.Second)
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("ждал %v", elapsed)
	}

	zombie := exec.Command("true")
	if err := zombie.Start(); err != nil {
		t.Fatal(err)
	}
	defer zombie.Wait()
	time.Sleep(200 * time.Millisecond)
	start = time.Now()
	waitForExit(zombie.Process.Pid, 5*time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("завершённый процесс ждал %v", elapsed)
	}
}
