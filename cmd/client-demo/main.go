// client-demo is a headless ambience client that connects to a server,
// subscribes to the atmosphere, runs the local sim, and periodically saves
// grid snapshots as PNGs.
//
// Lets humans (and Claude) inspect what the client is simulating without
// needing a real TTY or sixel-capable terminal. Useful for verifying that:
//   - snapshot + config + trigger commands are correctly applied
//   - the local sim produces the intended grid state
//
// Separate from any wt.exe-specific sixel rendering questions.
//
// Usage:
//
//	go run ./cmd/client-demo --server=http://localhost:8080 --out=./frames --every=5 --duration=20s
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/romaine-life/ambience/terminal"
)

func main() {
	server := flag.String("server", "https://ambience.romaine.life", "ambience server URL")
	out := flag.String("out", "./frames", "directory to save PNG frames into")
	every := flag.Int("every", 5, "save every N ticks (0 to disable)")
	gridW := flag.Int("w", 160, "grid width (pixels)")
	gridH := flag.Int("h", 80, "grid height (pixels)")
	duration := flag.Duration("duration", 15*time.Second, "how long to run before exiting (0 = until Ctrl-C)")
	flag.Parse()

	if *every <= 0 {
		log.Fatal("--every must be > 0")
	}

	c := terminal.New(terminal.Config{
		ServerURL:   *server,
		GridW:       *gridW,
		GridH:       *gridH,
		RecordDir:   *out,
		RecordEvery: *every,
		OnError:     func(err error) { log.Printf("client: %v", err) },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if *duration > 0 {
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		cancel()
	}()

	c.Start(ctx)
	fmt.Fprintf(os.Stderr, "client-demo: saving PNG frames to %s every %d ticks (%dx%d) — server=%s\n",
		*out, *every, *gridW, *gridH, *server)
	<-ctx.Done()
	c.Stop()
	fmt.Fprintln(os.Stderr, "client-demo: done")
}
