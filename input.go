package main

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

type EventType int

const (
	EvKey EventType = iota
	EvMouse
	EvPaste
	EvResize
	EvTick
	EvQuit
)

type Event struct {
	Type  EventType
	Key   string // "rune", "up", "enter", "ctrl+s", ...
	Rune  rune
	Alt   bool
	X, Y  int
	Btn   int // 0 left, 1 middle, 2 right, 64 wheel up, 65 wheel down
	Press bool
	Text  string
}

func runeEv(r rune) Event { return Event{Type: EvKey, Key: "rune", Rune: r} }

// kev builds a key event from a short spec ("p", "esc", "ctrl+s", " ").
func kev(spec string) Event {
	if utf8.RuneCountInString(spec) == 1 {
		r, _ := utf8.DecodeRuneInString(spec)
		return runeEv(r)
	}
	return Event{Type: EvKey, Key: spec}
}

// readInput reads stdin forever, parsing escape sequences into events.
func readInput(f *os.File, out chan<- Event) {
	buf := make([]byte, 0, 8192)
	tmp := make([]byte, 4096)
	for {
		n, err := f.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			evs, rest := parseInput(buf)
			for _, e := range evs {
				out <- e
			}
			buf = append(buf[:0], rest...)
			if len(buf) > 1<<20 {
				buf = buf[:0]
			}
		}
		if err != nil {
			out <- Event{Type: EvQuit}
			return
		}
	}
}

var csiKeys = map[byte]string{'A': "up", 'B': "down", 'C': "right", 'D': "left", 'H': "home", 'F': "end", 'Z': "shift+tab"}
var tildeKeys = map[int]string{1: "home", 2: "insert", 3: "delete", 4: "end", 5: "pgup", 6: "pgdn", 7: "home", 8: "end"}

// parseInput converts raw bytes to events. rest holds an incomplete trailing sequence.
func parseInput(buf []byte) (evs []Event, rest []byte) {
	i := 0
	for i < len(buf) {
		b := buf[i]
		switch {
		case b == 0x1b:
			if i+1 >= len(buf) {
				evs = append(evs, Event{Type: EvKey, Key: "esc"})
				i++
				continue
			}
			switch buf[i+1] {
			case '[':
				fin := -1
				for j := i + 2; j < len(buf); j++ {
					if buf[j] >= 0x40 && buf[j] <= 0x7e {
						fin = j
						break
					}
				}
				if fin < 0 {
					return evs, buf[i:]
				}
				params := string(buf[i+2 : fin])
				final := buf[fin]
				if params == "200" && final == '~' {
					end := bytes.Index(buf[fin+1:], []byte("\x1b[201~"))
					if end < 0 {
						return evs, buf[i:]
					}
					evs = append(evs, Event{Type: EvPaste, Text: string(buf[fin+1 : fin+1+end])})
					i = fin + 1 + end + 6
					continue
				}
				if ev, ok := parseCSI(params, final); ok {
					evs = append(evs, ev)
				}
				i = fin + 1
			case 'O':
				if i+2 >= len(buf) {
					return evs, buf[i:]
				}
				c := buf[i+2]
				if k, ok := csiKeys[c]; ok {
					evs = append(evs, Event{Type: EvKey, Key: k})
				} else if c >= 'P' && c <= 'S' {
					evs = append(evs, Event{Type: EvKey, Key: "f" + string(rune('1'+c-'P'))})
				}
				i += 3
			case 0x1b:
				evs = append(evs, Event{Type: EvKey, Key: "esc"})
				i++
			default:
				// Alt+key
				if !utf8.FullRune(buf[i+1:]) {
					return evs, buf[i:]
				}
				r, size := utf8.DecodeRune(buf[i+1:])
				ev := runeEv(r)
				switch r {
				case 0x7f:
					ev = Event{Type: EvKey, Key: "backspace"}
				case '\r', '\n':
					ev = Event{Type: EvKey, Key: "enter"}
				}
				ev.Alt = true
				evs = append(evs, ev)
				i += 1 + size
			}
		case b == '\r' || b == '\n':
			evs = append(evs, Event{Type: EvKey, Key: "enter"})
			i++
		case b == '\t':
			evs = append(evs, Event{Type: EvKey, Key: "tab"})
			i++
		case b == 0x7f || b == 0x08:
			evs = append(evs, Event{Type: EvKey, Key: "backspace"})
			i++
		case b < 0x20:
			evs = append(evs, Event{Type: EvKey, Key: "ctrl+" + string(rune(b+0x60))})
			i++
		default:
			if !utf8.FullRune(buf[i:]) {
				return evs, buf[i:]
			}
			r, size := utf8.DecodeRune(buf[i:])
			evs = append(evs, runeEv(r))
			i += size
		}
	}
	return evs, nil
}

func parseCSI(params string, final byte) (Event, bool) {
	if strings.HasPrefix(params, "<") && (final == 'M' || final == 'm') {
		p := strings.Split(params[1:], ";")
		if len(p) != 3 {
			return Event{}, false
		}
		b, _ := strconv.Atoi(p[0])
		x, _ := strconv.Atoi(p[1])
		y, _ := strconv.Atoi(p[2])
		if b&32 != 0 { // motion
			return Event{}, false
		}
		btn := b & 3
		if b&64 != 0 {
			btn = 64 + (b & 3)
		}
		return Event{Type: EvMouse, X: x - 1, Y: y - 1, Btn: btn, Press: final == 'M'}, true
	}
	parts := strings.Split(params, ";")
	mod := 1
	if len(parts) > 1 {
		mod, _ = strconv.Atoi(parts[1])
	}
	var key string
	if final == '~' {
		n, _ := strconv.Atoi(parts[0])
		k, ok := tildeKeys[n]
		if !ok {
			if n == 12 {
				key = "f2"
			} else {
				return Event{}, false
			}
		} else {
			key = k
		}
	} else if k, ok := csiKeys[final]; ok {
		key = k
	} else {
		return Event{}, false
	}
	m := mod - 1
	if m&4 != 0 {
		key = "ctrl+" + key
	}
	ev := Event{Type: EvKey, Key: key}
	if m&2 != 0 {
		ev.Alt = true
	}
	return ev, true
}
