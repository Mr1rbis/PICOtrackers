#!/usr/bin/env python3
"""Sysroot Debian 11 (bullseye, glibc 2.31) для сборки Linux-драйвера → third_party/sysroot-bullseye.

SteamVR на Linux грузит драйверы в контейнере Steam Runtime «sniper» (Debian 11): там glibc 2.31, если у хоста
не новее. Собранный системным компилятором Arch драйвер требовал glibc 2.43 и не грузился бы на Ubuntu/Debian.
С этим sysroot (заголовки glibc 2.31, libstdc++/libgcc от GCC 10 — статически) драйвер требует не больше 2.31.

Индекс пакетов закреплён sha256 из Release архива bullseye 11.11 (выпуск заморожен), каждый .deb проверяется
по sha256 из индекса. Абсолютные симлинки внутри пакетов переписываются в относительные — внутрь sysroot,
чтобы компоновщик не ушёл в библиотеки хоста.
"""
import hashlib
import io
import lzma
import os
import shutil
import sys
import tarfile
import urllib.request
from pathlib import Path

MIRROR = "https://archive.debian.org/debian"
PACKAGES_INDEX = "dists/bullseye/main/binary-amd64/Packages.xz"
PACKAGES_SHA256 = "1c3405444297ead864711c18e64bce4810db63b48340a2f9d81182d9f235ad71"
PACKAGES = ["libc6", "libc6-dev", "linux-libc-dev", "libgcc-s1", "libgcc-10-dev", "libstdc++-10-dev"]

ROOT = Path(__file__).resolve().parent
DEST = ROOT / "third_party" / "sysroot-bullseye"


def download(path, sha256):
    with urllib.request.urlopen(f"{MIRROR}/{path}") as response:
        data = response.read()
    digest = hashlib.sha256(data).hexdigest()
    if digest != sha256:
        sys.exit(f"sha256 не совпал: {path}: {digest}")
    return data


def parse_index(text):
    packages = {}
    for stanza in text.split("\n\n"):
        fields = {}
        for line in stanza.splitlines():
            if line and not line[0].isspace() and ":" in line:
                key, value = line.split(":", 1)
                fields[key] = value.strip()
        if "Package" in fields:
            packages[fields["Package"]] = fields
    return packages


def deb_data(deb):
    """data.tar.* из .deb (формат ar)."""
    assert deb[:8] == b"!<arch>\n"
    pos = 8
    while pos < len(deb):
        header = deb[pos:pos + 60]
        name = header[:16].decode().strip().rstrip("/")
        size = int(header[48:58])
        if name.startswith("data.tar"):
            return name, deb[pos + 60:pos + 60 + size]
        pos += 60 + size + size % 2
    sys.exit("в .deb нет data.tar")


def extract(name, data, dest):
    mode = {"data.tar.xz": "r:xz", "data.tar.gz": "r:gz", "data.tar": "r:"}[name]
    with tarfile.open(fileobj=io.BytesIO(data), mode=mode) as tar:
        members = []
        for member in tar.getmembers():
            if member.issym() and member.linkname.startswith("/"):
                link_dir = os.path.dirname("/" + member.name.lstrip("./"))
                member.linkname = os.path.relpath(member.linkname, link_dir)
            members.append(member)
        tar.extractall(dest, members=members, filter="data")


def main():
    index = parse_index(lzma.decompress(download(PACKAGES_INDEX, PACKAGES_SHA256)).decode())
    if DEST.exists():
        shutil.rmtree(DEST)
    DEST.mkdir(parents=True)
    for package in PACKAGES:
        fields = index[package]
        print(f"{package} {fields['Version']}")
        name, data = deb_data(download(fields["Filename"], fields["SHA256"]))
        extract(name, data, DEST)
    print(f"sysroot → {DEST}")


if __name__ == "__main__":
    main()
