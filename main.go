package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	path, err := DefaultPath()
	if err != nil {
		fatal(err)
	}
	db, err := OpenDB(path)
	if err != nil {
		fatal(fmt.Errorf("open %s: %w", path, err))
	}
	defer db.Close()

	restore, err := EnterRaw()
	if err != nil {
		fatal(err)
	}
	defer restore()

	out := bufio.NewWriterSize(os.Stdout, 1<<16)
	w, h := TermSize()
	app := NewApp(db, w, h)

	events := make(chan Event, 256)
	go readInput(os.Stdin, events)

	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	go func() {
		for s := range sig {
			if s == syscall.SIGWINCH {
				events <- Event{Type: EvResize}
			} else {
				events <- Event{Type: EvQuit}
			}
		}
	}()
	go func() {
		for range time.Tick(time.Second) {
			events <- Event{Type: EvTick}
		}
	}()

	step := func(ev Event) {
		if ev.Type == EvResize {
			app.scr.Resize(TermSize())
			return
		}
		app.handle(ev)
	}
	for !app.quitting {
		app.render()
		app.scr.Flush(out)
		step(<-events)
		for drained := false; !drained && !app.quitting; {
			select {
			case ev := <-events:
				step(ev)
			default:
				drained = true
			}
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "day-planner:", err)
	os.Exit(1)
}
