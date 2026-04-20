// sixel-demo — same as client-demo, but instead of saving PNGs it writes
// sixel bytes to stdout at a fixed rate. Lets us capture a stream of
// actual sixel output via a recorder (PowerSession, asciinema, etc.)
// without involving tcell or fzt.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nelsong6/ambience/terminal"
)

func main() {
	server := flag.String("server", "https://ambience.romaine.life", "ambience server URL")
	every := flag.Duration("every", 500*time.Millisecond, "emit a sixel every duration")
	gridW := flag.Int("w", 400, "grid width (pixels)")
	gridH := flag.Int("h", 10, "grid height (pixels)")
	duration := flag.Duration("duration", 10*time.Second, "total run duration")
	flag.Parse()

	c := terminal.New(terminal.Config{
		ServerURL: *server,
		GridW:     *gridW,
		GridH:     *gridH,
		OnError:   func(err error) { log.Printf("client: %v", err) },
	})
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	c.Start(ctx)

	fmt.Fprintf(os.Stderr, "sixel-demo: server=%s grid=%dx%d every=%s duration=%s\n",
		*server, *gridW, *gridH, *every, *duration)

	t := time.NewTicker(*every)
	defer t.Stop()
	emissions := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "sixel-demo: done (%d emissions attempted)\n", emissions)
			return
		case <-t.C:
			// Write sixel to stdout (no cursor positioning, we just want the bytes).
			if err := c.Render(os.Stdout, 1, 1); err != nil {
				fmt.Fprintf(os.Stderr, "render err: %v\n", err)
			}
			emissions++
		}
	}
}
