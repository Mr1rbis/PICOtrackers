#!/usr/bin/env python3
"""Отладка: принимает UDP-пакеты приложения на шлеме и раз в секунду показывает суставы.

Порт тот же, что у драйвера SteamVR (7788): запускать, когда SteamVR выключен, или с другим
портом (тогда и шлем запускать на него). Формат пакета — common/protocol.h.
Флаги сустава — XrSpaceLocationFlags: 3 = ориентация+позиция валидны, f = ещё и отслеживаются.
"""
import argparse
import socket
import struct
import time

HEADER = struct.Struct("<4sIqII")   # magic, seq, xr_time_ns, flags, joint_count
JOINT = struct.Struct("<3f4fI")     # position, orientation xyzw, flags
JOINT_COUNT = 24
PACKET_SIZE = HEADER.size + JOINT_COUNT * JOINT.size
FLAG_ALL_TRACKED, FLAG_TRACKERS_READY = 1, 2
SHOWN = {0: "pelvis", 9: "chest", 4: "l_knee", 5: "r_knee", 7: "l_ankle", 8: "r_ankle", 10: "l_foot",
         11: "r_foot", 15: "head"}


def parse(data):
    if len(data) != PACKET_SIZE:
        return None
    magic, seq, _, flags, count = HEADER.unpack_from(data)
    if magic != b"PTB1":
        return None
    joints = [JOINT.unpack_from(data, HEADER.size + i * JOINT.size) for i in range(min(count, JOINT_COUNT))]
    return seq, flags, joints


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--port", type=int, default=7788)
    args = ap.parse_args()

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind(("", args.port))
    sock.settimeout(0.1)

    sender = last = None
    packets = tracked = lost = 0
    last_seq = last_at = None
    changes = {}
    previous = {}
    next_draw = time.monotonic() + 1
    print(f"Жду пакеты на UDP-порту {args.port}…", flush=True)
    while True:
        try:
            data, addr = sock.recvfrom(4096)
        except socket.timeout:
            data = None
        now = time.monotonic()
        parsed = data and parse(data)
        if parsed:
            seq, flags, joints = parsed
            sender, last, last_at = addr[0], parsed, now
            packets += 1
            tracked += bool(flags & FLAG_ALL_TRACKED)
            if last_seq is not None and seq > last_seq + 1:
                lost += seq - last_seq - 1
            last_seq = seq
            for i in SHOWN:
                if i < len(joints):
                    changes[i] = changes.get(i, 0) + (previous.get(i) not in (None, joints[i][:7]))
                    previous[i] = joints[i][:7]

        if now < next_draw:
            continue
        next_draw += 1
        out = ["\033[H\033[J"]
        if last is None:
            out.append(f"Жду пакеты на UDP-порту {args.port}…")
        else:
            _, flags, joints = last
            state = "ИДУТ" if now - last_at < 1 else f"НЕТ ДАННЫХ уже {now - last_at:.0f} с"
            ready = "подключены" if flags & FLAG_TRACKERS_READY else "НЕ подключены"
            out.append(f"От {sender}: пакетов/с {packets}, скелет {tracked}/{packets}, потеряно {lost}  [{state}]")
            out.append(f"Трекеры: {ready}")
            out.append("")
            out.append(f"{'сустав':<10}{'флаги':>6}  {'x':>7} {'y':>7} {'z':>7}  {'изм/с':>6}")
            for i, name in SHOWN.items():
                if i < len(joints):
                    x, y, z, *_, jflags = joints[i]
                    out.append(f"{name:<10}{jflags:>6x}  {x:+7.3f} {y:+7.3f} {z:+7.3f}  {changes.get(i, 0):>6}")
        print("\n".join(out), flush=True)
        packets = tracked = lost = 0
        changes.clear()


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        pass
