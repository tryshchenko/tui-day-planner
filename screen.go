package main

import (
	"bufio"
	"fmt"
	"strings"
	"unicode"
)

// Color is 0xRRGGBB, or Default (-1) for the terminal default.
type Color int32

const Default Color = -1

type Attr uint8

const (
	Bold Attr = 1 << iota
	Dim
	Italic
	Underline
	Strike
	Reverse
)

type Style struct {
	FG, BG Color
	A      Attr
}

func sty(fg, bg Color, a ...Attr) Style {
	s := Style{FG: fg, BG: bg}
	for _, x := range a {
		s.A |= x
	}
	return s
}

type Rect struct{ X, Y, W, H int }

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

type Cell struct {
	R rune
	W uint8 // 1, 2, or 0 for the right half of a wide rune
	S Style
}

type Screen struct {
	W, H        int
	cells, prev []Cell
	curX, curY  int
	curVisible  bool
}

func NewScreen(w, h int) *Screen {
	s := &Screen{}
	s.Resize(w, h)
	return s
}

func (s *Screen) Resize(w, h int) {
	s.W, s.H = w, h
	s.cells = make([]Cell, w*h)
	s.prev = make([]Cell, w*h) // zero cells never equal a drawn cell: forces full repaint
}

func (s *Screen) Clear(st Style) {
	for i := range s.cells {
		s.cells[i] = Cell{R: ' ', W: 1, S: st}
	}
	s.curVisible = false
}

func (s *Screen) SetCursor(x, y int) { s.curX, s.curY, s.curVisible = x, y, true }

func (s *Screen) set(x, y int, r rune, w int, st Style) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	if w == 2 && x+1 >= s.W {
		r, w = ' ', 1
	}
	i := y*s.W + x
	// Overwriting half of a wide rune: blank the other half.
	if s.cells[i].W == 0 && x > 0 {
		p := &s.cells[i-1]
		*p = Cell{R: ' ', W: 1, S: p.S}
	}
	if s.cells[i].W == 2 && x+1 < s.W {
		n := &s.cells[i+1]
		*n = Cell{R: ' ', W: 1, S: n.S}
	}
	if w == 2 {
		if s.cells[i+1].W == 2 && x+2 < s.W {
			n := &s.cells[i+2]
			*n = Cell{R: ' ', W: 1, S: n.S}
		}
		s.cells[i+1] = Cell{R: 0, W: 0, S: st}
	}
	s.cells[i] = Cell{R: r, W: uint8(w), S: st}
}

// Text writes s starting at x, clipped to maxW columns. Returns columns used.
func (s *Screen) Text(x, y int, str string, st Style, maxW int) int {
	used := 0
	for _, r := range str {
		rw := runeWidth(r)
		if rw == 0 {
			continue
		}
		if used+rw > maxW {
			break
		}
		s.set(x+used, y, r, rw, st)
		used += rw
	}
	return used
}

func (s *Screen) Fill(r Rect, st Style) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			s.set(x, y, ' ', 1, st)
		}
	}
}

// Box draws a rounded border and fills the interior with bg.
func (s *Screen) Box(r Rect, border Style, bg Color) {
	if r.W < 2 || r.H < 2 {
		return
	}
	s.Fill(r, sty(Default, bg))
	b := border
	b.BG = bg
	s.Text(r.X, r.Y, "╭"+strings.Repeat("─", r.W-2)+"╮", b, r.W)
	for y := 1; y < r.H-1; y++ {
		s.Text(r.X, r.Y+y, "│", b, 1)
		s.Text(r.X+r.W-1, r.Y+y, "│", b, 1)
	}
	s.Text(r.X, r.Y+r.H-1, "╰"+strings.Repeat("─", r.W-2)+"╯", b, r.W)
}

// DimAll fades everything toward the background (used behind modals).
func (s *Screen) DimAll(toward Color) {
	for i := range s.cells {
		c := &s.cells[i]
		fg := c.S.FG
		if fg < 0 {
			fg = toward
		}
		c.S.FG = blend(fg, toward, 0.6)
		if c.S.BG >= 0 {
			c.S.BG = blend(c.S.BG, toward, 0.35)
		}
	}
}

func blend(a, b Color, t float64) Color {
	ar, ag, ab := int(a>>16)&255, int(a>>8)&255, int(a)&255
	br, bg, bb := int(b>>16)&255, int(b>>8)&255, int(b)&255
	m := func(x, y int) int { return int(float64(x)*(1-t) + float64(y)*t) }
	return Color(m(ar, br)<<16 | m(ag, bg)<<8 | m(ab, bb))
}

func sgr(st Style) string {
	var b strings.Builder
	b.WriteString("\x1b[0")
	if st.A&Bold != 0 {
		b.WriteString(";1")
	}
	if st.A&Dim != 0 {
		b.WriteString(";2")
	}
	if st.A&Italic != 0 {
		b.WriteString(";3")
	}
	if st.A&Underline != 0 {
		b.WriteString(";4")
	}
	if st.A&Reverse != 0 {
		b.WriteString(";7")
	}
	if st.A&Strike != 0 {
		b.WriteString(";9")
	}
	if st.FG >= 0 {
		fmt.Fprintf(&b, ";38;2;%d;%d;%d", (st.FG>>16)&255, (st.FG>>8)&255, st.FG&255)
	}
	if st.BG >= 0 {
		fmt.Fprintf(&b, ";48;2;%d;%d;%d", (st.BG>>16)&255, (st.BG>>8)&255, st.BG&255)
	}
	b.WriteByte('m')
	return b.String()
}

// Flush writes only the rows that changed since the previous frame.
func (s *Screen) Flush(w *bufio.Writer) {
	var b strings.Builder
	b.WriteString("\x1b[?2026h\x1b[?25l")
	for y := 0; y < s.H; y++ {
		row := s.cells[y*s.W : (y+1)*s.W]
		old := s.prev[y*s.W : (y+1)*s.W]
		same := true
		for x := range row {
			if row[x] != old[x] {
				same = false
				break
			}
		}
		if same {
			continue
		}
		fmt.Fprintf(&b, "\x1b[%d;1H", y+1)
		var cur Style
		first := true
		for x := 0; x < s.W; x++ {
			c := row[x]
			if c.W == 0 {
				continue
			}
			if first || c.S != cur {
				b.WriteString(sgr(c.S))
				cur, first = c.S, false
			}
			b.WriteRune(c.R)
		}
		copy(old, row)
	}
	b.WriteString("\x1b[0m")
	if s.curVisible {
		fmt.Fprintf(&b, "\x1b[%d;%dH\x1b[?25h", s.curY+1, s.curX+1)
	}
	b.WriteString("\x1b[?2026l")
	w.WriteString(b.String())
	w.Flush()
}

// Dump renders the grid as plain text (tests / debugging).
func (s *Screen) Dump() string {
	var b strings.Builder
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			c := s.cells[y*s.W+x]
			if c.W == 0 {
				continue
			}
			b.WriteRune(c.R)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// ---- text metrics ----

func runeWidth(r rune) int {
	if r == 0 || r < 32 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	if r < 0x300 {
		return 1
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1f64f) ||
		(r >= 0x1f680 && r <= 0x1f6ff) ||
		(r >= 0x1f900 && r <= 0x1f9ff) ||
		(r >= 0x1fa70 && r <= 0x1faff) ||
		(r >= 0x20000 && r <= 0x3fffd)) {
		return 2
	}
	return 1
}

func strWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

func runesWidth(rs []rune) int {
	w := 0
	for _, r := range rs {
		w += runeWidth(r)
	}
	return w
}

// truncate shortens s to at most w columns, ending with an ellipsis.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if strWidth(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runeWidth(r)
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteString("…")
	return b.String()
}
