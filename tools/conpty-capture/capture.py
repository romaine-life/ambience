"""
ConPTY byte-level capture for Windows terminal apps.

Spawns a command under a pseudo-console and records every output byte
(including sixel DCS blocks, which PowerSession misses) with timestamps
into an asciinema-compatible .cast file.

Usage:
    python conpty_capture.py "C:\\path\\to\\app.exe" --duration 5 --out run.cast
"""
import argparse
import json
import os
import sys
import time

from winpty import PTY, Backend


def capture(command: str, duration: float, cols: int, rows: int, out_path: str):
    pty = PTY(cols, rows, backend=Backend.ConPTY)
    if not pty.spawn(command):
        raise RuntimeError(f"Failed to spawn: {command}")

    events = []
    start = time.monotonic()
    while True:
        now = time.monotonic()
        elapsed = now - start
        if elapsed >= duration:
            break
        try:
            chunk = pty.read(False)  # non-blocking; "" if no data
        except Exception as e:
            print(f"read err: {e}", file=sys.stderr)
            chunk = ""
        if chunk:
            events.append((elapsed, chunk))
        else:
            time.sleep(0.01)
        if not pty.isalive():
            # Drain any tail output the process emitted before exiting.
            for _ in range(10):
                try:
                    chunk = pty.read(False)
                except Exception:
                    chunk = ""
                if chunk:
                    events.append((time.monotonic() - start, chunk))
                time.sleep(0.02)
            break

    # Force kill if still alive (TUIs won't exit on their own)
    if pty.isalive():
        try:
            os.kill(pty.pid, 9)
        except Exception:
            pass

    # Write asciinema v2 cast
    header = {
        "version": 2,
        "width": cols,
        "height": rows,
        "timestamp": int(time.time()),
        "env": {"SHELL": "conpty_capture", "TERM": "xterm-256color"},
    }
    with open(out_path, "w", encoding="utf-8", newline="\n") as f:
        f.write(json.dumps(header) + "\n")
        for t, data in events:
            f.write(json.dumps([round(t, 6), "o", data]) + "\n")

    total_chars = sum(len(d) for _, d in events)
    dur = events[-1][0] if events else 0.0
    print(
        f"captured {len(events)} events, {total_chars} chars over {dur:.2f}s -> {out_path}",
        file=sys.stderr,
    )


def main():
    p = argparse.ArgumentParser()
    p.add_argument("command", help="Path to executable (with any args)")
    p.add_argument("--duration", type=float, default=5.0)
    p.add_argument("--cols", type=int, default=120)
    p.add_argument("--rows", type=int, default=30)
    p.add_argument("--out", default="capture.cast")
    args = p.parse_args()
    capture(args.command, args.duration, args.cols, args.rows, args.out)


if __name__ == "__main__":
    main()
