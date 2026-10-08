// Вшитые в программу файлы: драйвер SteamVR (Linux и Windows), приложение для шлема, значок.
// Папку assets/ заполняет build.sh перед сборкой.
package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"sort"
)

//go:embed all:assets
var assets embed.FS

// appIcon — значок программы (SVG): окно, «О программе», меню приложений Linux.
//
//go:embed icon.svg
var appIcon string

const (
	assetsDriver = "assets/driver/picotrackers"
	assetsAPK    = "assets/PicoTrackers.apk"
)

// driverAssetsVersion — хеш всех файлов вшитого драйвера: по нему видно, что установленный устарел.
func driverAssetsVersion() string {
	var paths []string
	fs.WalkDir(assets, assetsDriver, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		data, _ := assets.ReadFile(path)
		hash.Write([]byte(path))
		hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil))[:16]
}

func apkData() ([]byte, error) { return assets.ReadFile(assetsAPK) }

func apkHash() string {
	data, err := apkData()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
