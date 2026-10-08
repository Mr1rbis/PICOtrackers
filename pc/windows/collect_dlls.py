#!/usr/bin/env python3
"""Копирует в папку программы все DLL, от которых (рекурсивно) зависят указанные .exe/.dll.

DLL ищутся в bin/ sysroot MSYS2; системные DLL Windows (KERNEL32, USER32, api-ms-win-crt-…) там
не лежат и пропускаются.

  collect_dlls.py <sysroot/ucrt64/bin> <куда копировать> <файл.exe|.dll>…
"""
import re
import shutil
import subprocess
import sys
from pathlib import Path


def imports(path):
    out = subprocess.run(["x86_64-w64-mingw32-objdump", "-p", str(path)], capture_output=True, text=True).stdout
    return re.findall(r"DLL Name: (\S+)", out)


def main():
    bin_dir, dest = Path(sys.argv[1]), Path(sys.argv[2])
    available = {p.name.lower(): p for p in bin_dir.glob("*.dll")}
    queue, copied = [Path(p) for p in sys.argv[3:]], set()
    while queue:
        for name in imports(queue.pop()):
            source = available.get(name.lower())
            if source and name.lower() not in copied:
                copied.add(name.lower())
                shutil.copy2(source, dest / source.name)
                queue.append(source)
    print(f"DLL: {len(copied)}")


if __name__ == "__main__":
    main()
