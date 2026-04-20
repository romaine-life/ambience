# conpty-capture

Windows-native byte-level capture for terminal applications that emit sixel
graphics (e.g. anything using `ambience/terminal`).

## Why this exists

`PowerSession`, `asciinema`, and other tape-based recorders on Windows
capture through Console API hooks that don't include the byte stream of
sixel DCS blocks written to `CONOUT$`. That makes it impossible to
diagnose sixel-related rendering issues (transparency flags, raster
dimensions, interleaving with tcell frame emissions) after the fact.

This tool spawns the target under a pseudo-console (ConPTY) via
`pywinpty` and captures the full output byte stream — both ANSI cell
writes *and* sixel blocks — into an asciinema-compatible `.cast` file.

## Requirements

- Windows 10+ (ConPTY)
- Python 3.8+
- `pip install pywinpty`

## Usage

```
python capture.py "C:\path\to\app.exe" --duration 5 --out run.cast
```

Options:
- `--duration <seconds>` — how long to record (default 5)
- `--cols <N>` / `--rows <N>` — PTY size (default 120x30)
- `--out <path>` — output cast path (default capture.cast)

The output is asciinema v2 format, so any tool that reads `.cast` files
(asciinema player, agg, custom analyzers) works — as long as they
tolerate sixel DCS sequences in the stream.

## Gap this closes

- ✅ Captures sixel DCS blocks (PowerSession misses them entirely)
- ✅ Preserves timing → flicker diagnosis
- ✅ Works on Windows without WSL / Docker / VM

Does NOT capture pixel-level rendered output — that's a WT-renderer
concern. Use this for byte-level diagnosis (what's being sent),
complement with visual checks (what the terminal actually shows).
