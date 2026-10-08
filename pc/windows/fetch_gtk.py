#!/usr/bin/env python3
"""Скачивает GTK 4 + libadwaita для Windows (пакеты MSYS2, окружение ucrt64) со всеми зависимостями
и распаковывает в sysroot для кросс-сборки программы: pc/third_party/msys2/ucrt64/…

Пакеты проверяются по SHA-256 из базы репозитория. Заголовки и CRT самого mingw-w64 не берём:
для сборки используется системный x86_64-w64-mingw32-gcc (тоже UCRT).
"""
import hashlib
import io
import sys
import tarfile
import urllib.request
from pathlib import Path

REPO = "https://repo.msys2.org/mingw/ucrt64"
PREFIX = "mingw-w64-ucrt-x86_64-"
ROOTS = ["gtk4", "libadwaita", "adwaita-icon-theme", "librsvg", "gobject-introspection"]
SKIP = {"headers-git", "crt-git", "binutils", "gcc", "cc", "python", "perl", "tcl", "tk"}
DEST = Path(__file__).resolve().parent.parent / "third_party/msys2"


def fetch(url):
    with urllib.request.urlopen(url, timeout=300) as response:
        return response.read()


def parse_db(data):
    """Пакеты из базы: имя → {filename, sha256, depends}; плюс provides → имя."""
    packages, provides = {}, {}
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:zst") as db:
        for member in db.getmembers():
            if not member.name.endswith("/desc"):
                continue
            fields, key = {}, None
            for line in db.extractfile(member).read().decode().splitlines():
                if line.startswith("%") and line.endswith("%"):
                    key = line.strip("%")
                    fields[key] = []
                elif line and key:
                    fields[key].append(line)
            name = fields["NAME"][0]
            packages[name] = {"filename": fields["FILENAME"][0], "sha256": fields["SHA256SUM"][0],
                              "depends": fields.get("DEPENDS", [])}
            for provided in fields.get("PROVIDES", []):
                provides[strip_version(provided)] = name
    return packages, provides


def strip_version(dep):
    for op in (">=", "<=", "=", ">", "<"):
        dep = dep.split(op)[0]
    return dep.strip()


def resolve(packages, provides):
    wanted, queue = set(), [PREFIX + r for r in ROOTS]
    while queue:
        name = strip_version(queue.pop())
        name = name if name in packages else provides.get(name, name)
        if name in wanted or not name.startswith(PREFIX) or name[len(PREFIX):] in SKIP:
            continue
        if name not in packages:
            print(f"  нет в репозитории: {name}", file=sys.stderr)
            continue
        wanted.add(name)
        queue.extend(packages[name]["depends"])
    return sorted(wanted)


def writable(member, path):
    """Как filter="tar", но файлы остаются доступны на запись — иначе повторная распаковка падает."""
    member = tarfile.tar_filter(member, path)
    return member.replace(mode=(member.mode or 0o644) | 0o200, deep=False)


def main():
    print("база пакетов MSYS2 ucrt64…")
    packages, provides = parse_db(fetch(f"{REPO}/ucrt64.db"))
    names = resolve(packages, provides)
    print(f"пакетов: {len(names)}")

    DEST.mkdir(parents=True, exist_ok=True)
    cache = DEST / "cache"
    cache.mkdir(exist_ok=True)
    for name in names:
        info = packages[name]
        path = cache / info["filename"]
        if not path.exists() or hashlib.sha256(path.read_bytes()).hexdigest() != info["sha256"]:
            print(f"  ↓ {info['filename']}")
            data = fetch(f"{REPO}/{info['filename']}")
            if hashlib.sha256(data).hexdigest() != info["sha256"]:
                sys.exit(f"SHA-256 не совпал: {info['filename']}")
            path.write_bytes(data)
        with tarfile.open(path, mode="r:zst") as package:
            members = [m for m in package.getmembers() if m.name.startswith("ucrt64/")]
            package.extractall(DEST, members=members, filter=writable)
    (DEST / "packages.txt").write_text("\n".join(packages[n]["filename"] for n in names) + "\n")
    print(f"готово: {DEST}")


if __name__ == "__main__":
    main()
