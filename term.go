package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const (
	enterSeq = "\x1b[?1049h" + // alternate screen
		"\x1b[?7l" + // no autowrap
		"\x1b[?1000h\x1b[?1006h" + // mouse: press/release + SGR encoding
		"\x1b[?2004h" + // bracketed paste
		"\x1b[5 q" + // blinking bar cursor
		"\x1b]0;Day Planner\x07"
	leaveSeq = "\x1b[?2004l\x1b[?1006l\x1b[?1000l\x1b[?7h\x1b[0 q\x1b[?25h\x1b[0m\x1b[?1049l"
)

// EnterRaw puts the terminal into raw mode and returns a restore function.
func EnterRaw() (restore func(), err error) {
	fd := int(os.Stdin.Fd())
	old, err := unix.IoctlGetTermios(fd, ioctlGet)
	if err != nil {
		return nil, fmt.Errorf("stdin is not a terminal: %w", err)
	}
	raw := *old
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, ioctlSet, &raw); err != nil {
		return nil, err
	}
	os.Stdout.WriteString(enterSeq)
	return func() {
		os.Stdout.WriteString(leaveSeq)
		unix.IoctlSetTermios(fd, ioctlSet, old)
	}, nil
}

func TermSize() (w, h int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}
