// Обновление программы. При каждом запуске программа спрашивает у GitHub последний релиз; если он новее,
// окно (updatedialog.go) предлагает обновиться. Обновление скачивается и ставится, только когда SteamVR
// закрыт, а шлем подключён USB-кабелем с разрешённой отладкой: вместе с программой обновляются драйвер
// SteamVR и приложение на шлеме. Файлы сверяются с SHA256SUMS.txt релиза.
// Ставит новую программу platform_*.go (installUpdate): Linux — подмена файла и перезапуск,
// Windows — тихий запуск установщика после выхода программы.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	sumsAsset = "SHA256SUMS.txt"
	apkAsset  = "PicoTrackers.apk"
)

// releaseURL — последний релиз на GitHub; PICOTRACKERS_UPDATE_URL подменяет адрес (проверка на своём сервере).
func releaseURL() string {
	if url := os.Getenv("PICOTRACKERS_UPDATE_URL"); url != "" {
		return url
	}
	return "https://api.github.com/repos/" + githubRepo + "/releases/latest"
}

// updateDir — куда Windows-версия кладёт скачанный установщик; чистится при запуске программы.
func updateDir() string { return filepath.Join(dataDir(), "update") }

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release — релиз новее запущенной программы и его файлы для этой системы.
type Release struct {
	Version string
	PageURL string

	program, apk, sums releaseAsset
}

// checkForUpdate — последний релиз, если он новее; nil, nil — обновлений нет.
func checkForUpdate() (*Release, error) {
	request, err := http.NewRequest(http.MethodGet, releaseURL(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "picotrackers/"+appVersion)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, nil // релизов ещё нет
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(T("GitHub ответил %s"), response.Status)
	}
	var latest struct {
		Tag    string         `json:"tag_name"`
		Page   string         `json:"html_url"`
		Assets []releaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(response.Body).Decode(&latest); err != nil {
		return nil, err
	}
	version := strings.TrimPrefix(latest.Tag, "v")
	if !versionNewer(version, appVersion) {
		return nil, nil
	}
	r := &Release{Version: version, PageURL: latest.Page}
	for _, a := range latest.Assets {
		switch {
		case a.Name == sumsAsset:
			r.sums = a
		case a.Name == apkAsset:
			r.apk = a
		case isProgramAsset(a.Name):
			r.program = a
		}
	}
	if r.program.URL == "" || r.apk.URL == "" || r.sums.URL == "" {
		return nil, fmt.Errorf(T("в релизе %s нет файлов для этой системы"), latest.Tag)
	}
	return r, nil
}

// parseVersion — «1.4.2» → [1 4 2]; nil, если это не версия из чисел.
func parseVersion(version string) []int {
	var parts []int
	for _, field := range strings.Split(version, ".") {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return nil
		}
		parts = append(parts, n)
	}
	return parts
}

// versionNewer — version новее than («1.10» новее «1.9», «1.4.0» = «1.4»).
func versionNewer(version, than string) bool {
	a, b := parseVersion(version), parseVersion(than)
	if a == nil || b == nil {
		return false
	}
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// UpdateConditions — без чего обновление не скачивается и не ставится: SteamVR закрыт (иначе драйвер
// не заменить), шлем подключён кабелем и разрешил отладку по USB (обновляется и приложение на шлеме).
type UpdateConditions struct {
	SteamVRRunning bool
	AdbFound       bool
	Serial, State  string // шлем по USB и его состояние в adb; "" — не найден
}

func updateConditions() UpdateConditions {
	_, running := steamVRStartedAt()
	c := UpdateConditions{SteamVRRunning: running, AdbFound: adbPath() != ""}
	if c.AdbFound {
		c.Serial, c.State = usbHeadset()
	}
	return c
}

func (c UpdateConditions) Met() bool { return !c.SteamVRRunning && c.State == "device" }

func (c UpdateConditions) err() error {
	switch {
	case c.SteamVRRunning:
		return errors.New(T("SteamVR запущен — закройте его"))
	case c.State != "device":
		return errors.New(T("шлем не подключён кабелем — подключите его и разрешите отладку по USB"))
	}
	return nil
}

// Install скачивает обновление, сверяет файлы и ставит: приложение на шлем (если там другое), затем программу
// с драйвером. Условия проверяются перед загрузкой и ещё раз перед установкой. log — события для журнала,
// progress — ход загрузки. restart запускает новую версию (на Windows — установщик): вызвать перед выходом.
func (r *Release) Install(log, progress func(string)) (restart func() error, err error) {
	if err := canSelfUpdate(); err != nil {
		return nil, err
	}
	if err := updateConditions().err(); err != nil {
		return nil, err
	}
	log(Tf("скачиваю обновление %s", r.Version))
	sumsData, err := download(r.sums, nil)
	if err != nil {
		return nil, err
	}
	sums := parseSums(sumsData)
	program, err := downloadVerified(r.program, sums, T("программа"), progress)
	if err != nil {
		return nil, err
	}
	apk, err := downloadVerified(r.apk, sums, T("приложение для шлема"), progress)
	if err != nil {
		return nil, err
	}

	conditions := updateConditions()
	if err := conditions.err(); err != nil {
		return nil, err
	}
	headset := &Headset{conditions.Serial}
	if installedAPKHash(headset) != sums[r.apk.Name] {
		if err := installAPK(headset, apk, log); err != nil {
			return nil, err
		}
	} else {
		log(T("приложение на шлеме уже этой версии"))
	}
	return installUpdate(r.program.Name, program, log)
}

// downloadVerified скачивает файл релиза и сверяет его sha256 с SHA256SUMS.txt.
func downloadVerified(a releaseAsset, sums map[string]string, what string, progress func(string)) ([]byte, error) {
	data, err := download(a, func(done, total int64) {
		progress(Tf("Скачиваю: %s — %.1f из %.1f МБ", what, float64(done)/1e6, float64(total)/1e6))
	})
	if err != nil {
		return nil, err
	}
	want := sums[a.Name]
	if want == "" {
		return nil, fmt.Errorf(T("%s нет в %s"), a.Name, sumsAsset)
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != want {
		return nil, fmt.Errorf(T("%s скачан с ошибкой: не совпала контрольная сумма"), a.Name)
	}
	return data, nil
}

// parseSums разбирает вывод sha256sum: «<хеш>  <имя>».
func parseSums(data []byte) map[string]string {
	sums := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 {
			sums[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
		}
	}
	return sums
}

// download скачивает файл в память; progress (может быть nil) — не чаще 5 раз в секунду и в конце.
func download(a releaseAsset, progress func(done, total int64)) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, a.URL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "picotrackers/"+appVersion)
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(T("загрузка %s: %s"), a.Name, response.Status)
	}
	total := response.ContentLength
	if total <= 0 {
		total = a.Size
	}
	var data bytes.Buffer
	chunk := make([]byte, 64*1024)
	var reported time.Time
	for {
		n, err := response.Body.Read(chunk)
		data.Write(chunk[:n])
		if progress != nil && (err != nil || time.Since(reported) > 200*time.Millisecond) {
			progress(int64(data.Len()), total)
			reported = time.Now()
		}
		if err == io.EOF {
			return data.Bytes(), nil
		}
		if err != nil {
			return nil, fmt.Errorf(T("загрузка %s: %w"), a.Name, err)
		}
	}
}
