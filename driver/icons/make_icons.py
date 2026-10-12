#!/usr/bin/env python3
"""Иконки трекера для окна состояния SteamVR (vrmonitor) из силуэта pico_tracker.svg → driver/package/resources/icons.

Набор — как у Vive Tracker (драйвер htc): 32×32 и 64×64 (суффикс _2x, его ищет vrmonitor), «ищется» — анимированный GIF.
Цвета SteamVR: выключен #44454A, готов #85B717; значки поверх (с прозрачным зазором вокруг): «i» — предупреждение,
«!» — ошибка, «молния» — низкий заряд. Нужны rsvg-convert (librsvg) и magick (ImageMagick). Готовые файлы лежат
в репозитории — запускать, только если поменялся рисунок: python3 driver/icons/make_icons.py
"""
import os
import re
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "..", "package", "resources", "icons")
OFF, GREEN, ERROR, LOW, GLYPH = "#44454A", "#85B717", "#E7105C", "#D73822", "#FFFFFF"

source = open(os.path.join(HERE, "pico_tracker.svg")).read()
body = re.search(r"<svg[^>]*>(.*)</svg>", source, re.S).group(1)
width, height = (float(v) for v in re.search(r'viewBox="0 0 ([\d.]+) ([\d.]+)"', source).groups())
SCALE = 30 / height  # трекер — во всю высоту иконки 32×32 (по пикселю сверху и снизу)
LEFT = 15 - width * SCALE / 2

BADGE_X, BADGE_Y, BADGE_R, GAP = 25, 25, 6.5, 1.6
GLYPHS = {
    "info": f'<rect x="{BADGE_X - 0.9}" y="{BADGE_Y - 1}" width="1.8" height="4.6" rx="0.4" fill="#2B2B2B"/>'
            f'<circle cx="{BADGE_X}" cy="{BADGE_Y - 3}" r="1.1" fill="#2B2B2B"/>',
    "error": f'<rect x="{BADGE_X - 0.9}" y="{BADGE_Y - 4.2}" width="1.8" height="5" rx="0.4" fill="{GLYPH}"/>'
             f'<circle cx="{BADGE_X}" cy="{BADGE_Y + 2.9}" r="1.1" fill="{GLYPH}"/>',
    "low": f'<path d="M{BADGE_X + 0.8} {BADGE_Y - 4.6} L{BADGE_X - 2.6} {BADGE_Y + 0.6} H{BADGE_X - 0.2} '
           f'L{BADGE_X - 0.9} {BADGE_Y + 4.6} L{BADGE_X + 2.6} {BADGE_Y - 0.8} H{BADGE_X + 0.2} Z" fill="{GLYPH}"/>',
}
BADGE_COLORS = {"info": GREEN, "error": ERROR, "low": LOW}


def icon(color, opacity=1.0, badge=None):
    """SVG 32×32: трекер цветом color; badge — значок в правом нижнем углу (вокруг него трекер вырезан)."""
    tracker = re.sub(r'fill="#[0-9A-Fa-f]{6}"', f'fill="{color}"', body)
    mask = ""
    if badge:
        mask = ('<mask id="gap"><rect width="32" height="32" fill="white"/>'
                f'<circle cx="{BADGE_X}" cy="{BADGE_Y}" r="{BADGE_R + GAP}" fill="black"/></mask>')
    group = (f'<g {"mask=" + chr(34) + "url(#gap)" + chr(34) if badge else ""} opacity="{opacity}">'
             f'<g transform="translate({LEFT:.3f} 1) scale({SCALE:.5f})">{tracker}</g></g>')
    extra = ""
    if badge:
        extra = f'<circle cx="{BADGE_X}" cy="{BADGE_Y}" r="{BADGE_R}" fill="{BADGE_COLORS[badge]}"/>' + GLYPHS[badge]
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">{mask}{group}{extra}</svg>'


def mix(a, b, t):
    """Цвет между a и b (t = 0 → a, 1 → b)."""
    ca, cb = [int(a[i:i + 2], 16) for i in (1, 3, 5)], [int(b[i:i + 2], 16) for i in (1, 3, 5)]
    return "#" + "".join(f"{round(x + (y - x) * t):02X}" for x, y in zip(ca, cb))


def render(svg, path, size):
    subprocess.run(["rsvg-convert", "-w", str(size), "-h", str(size), "-o", path], input=svg.encode(), check=True)


STILL = {
    "off": icon(OFF),
    "ready": icon(GREEN),
    "ready_alert": icon(GREEN, badge="info"),
    "error": icon(GREEN, badge="error"),
    "standby": icon(GREEN, opacity=0.45),
    "ready_low": icon(GREEN, badge="low"),
}
# «Ищется» — плавное мигание зелёный ↔ серый, 16 кадров по 80 мс.
PULSE = [abs(1 - 2 * i / 16) for i in range(16)]
ANIMATED = {
    "searching": [icon(mix(OFF, GREEN, t)) for t in PULSE],
    "searching_alert": [icon(mix(OFF, GREEN, t), badge="info") for t in PULSE],
}

for tool in ("rsvg-convert", "magick"):
    if not shutil.which(tool):
        sys.exit(f"нужен {tool}")
os.makedirs(OUT, exist_ok=True)
for name, svg in STILL.items():
    for size, suffix in ((32, ""), (64, "_2x")):
        render(svg, os.path.join(OUT, f"pico_tracker_{name}{suffix}.png"), size)
with tempfile.TemporaryDirectory() as tmp:
    for name, frames in ANIMATED.items():
        for size, suffix in ((32, ""), (64, "_2x")):
            files = []
            for i, svg in enumerate(frames):
                files.append(os.path.join(tmp, f"{name}{suffix}-{i:02d}.png"))
                render(svg, files[-1], size)
            subprocess.run(["magick", "-delay", "8", "-loop", "0", "-dispose", "Background", *files,
                            os.path.join(OUT, f"pico_tracker_{name}{suffix}.gif")], check=True)
print("иконки:", os.path.normpath(OUT))
