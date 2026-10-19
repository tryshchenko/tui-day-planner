package main

import "strings"

// vline is one visual (soft-wrapped) line: runes [start,end) of logical line row.
type vline struct{ row, start, end int }

// TextArea is a small plain-text editor with soft wrapping.
type TextArea struct {
	lines      [][]rune
	row, col   int
	wantX      int // desired visual column for up/down; -1 = unset
	scroll     int
	single     bool // single logical line (newlines become spaces)
	needEnsure bool // scroll the cursor into view on next draw

	// geometry / state of the last draw
	gx, gy, gw, gh int
	total          int
}

func NewTextArea(single bool) *TextArea {
	return &TextArea{lines: [][]rune{{}}, wantX: -1, single: single, gw: 40, gh: 5}
}

func sanitize(s string, single bool) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\t", "  ")
	if single {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r < 32 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

// SetText replaces the content and puts the cursor at the end.
func (t *TextArea) SetText(s string) {
	parts := strings.Split(sanitize(s, t.single), "\n")
	t.lines = make([][]rune, len(parts))
	for i, p := range parts {
		t.lines[i] = []rune(p)
	}
	t.row = len(t.lines) - 1
	t.col = len(t.lines[t.row])
	t.wantX = -1
	t.scroll = 0
	t.needEnsure = true
}

func (t *TextArea) Text() string {
	parts := make([]string, len(t.lines))
	for i, l := range t.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

func (t *TextArea) Empty() bool { return len(t.lines) == 1 && len(t.lines[0]) == 0 }

// Insert adds text at the cursor.
func (t *TextArea) Insert(s string) bool {
	s = sanitize(s, t.single)
	if s == "" {
		return false
	}
	parts := strings.Split(s, "\n")
	line := t.lines[t.row]
	tail := append([]rune(nil), line[t.col:]...)
	first := append(append([]rune(nil), line[:t.col]...), []rune(parts[0])...)
	if len(parts) == 1 {
		t.lines[t.row] = append(first, tail...)
		t.col = len(first)
	} else {
		ins := [][]rune{first}
		for _, p := range parts[1 : len(parts)-1] {
			ins = append(ins, []rune(p))
		}
		last := []rune(parts[len(parts)-1])
		newCol := len(last)
		ins = append(ins, append(last, tail...))
		out := append([][]rune(nil), t.lines[:t.row]...)
		out = append(out, ins...)
		out = append(out, t.lines[t.row+1:]...)
		t.lines = out
		t.row += len(parts) - 1
		t.col = newCol
	}
	t.wantX = -1
	t.needEnsure = true
	return true
}

func wrapRunes(line []rune, width int) [][2]int {
	if width < 1 {
		width = 1
	}
	var segs [][2]int
	i := 0
	for {
		w, j, lastSpace := 0, i, -1
		for j < len(line) {
			rw := runeWidth(line[j])
			if w+rw > width {
				break
			}
			if line[j] == ' ' {
				lastSpace = j
			}
			w += rw
			j++
		}
		if j >= len(line) {
			return append(segs, [2]int{i, len(line)})
		}
		switch {
		case line[j] == ' ':
			segs = append(segs, [2]int{i, j + 1})
			i = j + 1
		case lastSpace >= i:
			segs = append(segs, [2]int{i, lastSpace + 1})
			i = lastSpace + 1
		default:
			if j == i {
				j = i + 1
			}
			segs = append(segs, [2]int{i, j})
			i = j
		}
		if i >= len(line) {
			return append(segs, [2]int{len(line), len(line)})
		}
	}
}

func (t *TextArea) vlines(width int) []vline {
	ww := width - 1 // keep a column free for the cursor
	if ww < 1 {
		ww = 1
	}
	var out []vline
	for r, l := range t.lines {
		for _, s := range wrapRunes(l, ww) {
			out = append(out, vline{r, s[0], s[1]})
		}
	}
	return out
}

func lastOfRow(vl []vline, i int) bool { return i+1 >= len(vl) || vl[i+1].row != vl[i].row }

// cursorV returns the visual line index and visual x of the cursor.
func (t *TextArea) cursorV(vl []vline) (idx, x int) {
	for i, v := range vl {
		if v.row != t.row {
			continue
		}
		if t.col < v.end || lastOfRow(vl, i) {
			idx = i
			break
		}
	}
	v := vl[idx]
	return idx, runesWidth(t.lines[v.row][v.start:t.col])
}

// colAt returns the column on visual line v nearest to visual x.
func (t *TextArea) colAt(vl []vline, i, x int) int {
	v := vl[i]
	limit := v.end
	if !lastOfRow(vl, i) {
		limit = v.end - 1
		if limit < v.start {
			limit = v.start
		}
	}
	c, w := v.start, 0
	line := t.lines[v.row]
	for c < limit {
		rw := runeWidth(line[c])
		if w+rw > x {
			break
		}
		w += rw
		c++
	}
	return c
}

func (t *TextArea) moveV(dir int) {
	vl := t.vlines(t.gw)
	idx, x := t.cursorV(vl)
	if t.wantX < 0 {
		t.wantX = x
	}
	ni := idx + dir
	if ni < 0 {
		t.col = 0
		return
	}
	if ni >= len(vl) {
		t.col = len(t.lines[t.row])
		return
	}
	t.row = vl[ni].row
	t.col = t.colAt(vl, ni, t.wantX)
}

func (t *TextArea) left() {
	if t.col > 0 {
		t.col--
	} else if t.row > 0 {
		t.row--
		t.col = len(t.lines[t.row])
	}
}

func (t *TextArea) right() {
	if t.col < len(t.lines[t.row]) {
		t.col++
	} else if t.row < len(t.lines)-1 {
		t.row++
		t.col = 0
	}
}

func isSpace(r rune) bool { return r == ' ' }

func (t *TextArea) wordLeft() {
	if t.col == 0 {
		t.left()
		return
	}
	l := t.lines[t.row]
	for t.col > 0 && isSpace(l[t.col-1]) {
		t.col--
	}
	for t.col > 0 && !isSpace(l[t.col-1]) {
		t.col--
	}
}

func (t *TextArea) wordRight() {
	l := t.lines[t.row]
	if t.col >= len(l) {
		t.right()
		return
	}
	for t.col < len(l) && !isSpace(l[t.col]) {
		t.col++
	}
	for t.col < len(l) && isSpace(l[t.col]) {
		t.col++
	}
}

func (t *TextArea) backspace() bool {
	if t.col > 0 {
		l := t.lines[t.row]
		t.lines[t.row] = append(l[:t.col-1], l[t.col:]...)
		t.col--
		return true
	}
	if t.row > 0 {
		prev := t.lines[t.row-1]
		t.col = len(prev)
		t.lines[t.row-1] = append(prev, t.lines[t.row]...)
		t.lines = append(t.lines[:t.row], t.lines[t.row+1:]...)
		t.row--
		return true
	}
	return false
}

func (t *TextArea) del() bool {
	l := t.lines[t.row]
	if t.col < len(l) {
		t.lines[t.row] = append(l[:t.col], l[t.col+1:]...)
		return true
	}
	if t.row < len(t.lines)-1 {
		t.lines[t.row] = append(l, t.lines[t.row+1]...)
		t.lines = append(t.lines[:t.row+1], t.lines[t.row+2:]...)
		return true
	}
	return false
}

func (t *TextArea) deleteRange(from int) bool { // delete [from, col) on the current line
	if from >= t.col {
		return false
	}
	l := t.lines[t.row]
	t.lines[t.row] = append(l[:from], l[t.col:]...)
	t.col = from
	return true
}

func (t *TextArea) deleteWordBack() bool {
	if t.col == 0 {
		return t.backspace()
	}
	end := t.col
	t.wordLeft()
	from := t.col
	t.col = end
	return t.deleteRange(from)
}

// HandleKey applies an editing key; returns true if the text changed.
func (t *TextArea) HandleKey(ev Event) bool {
	t.needEnsure = true
	changed := false
	keepX := false
	switch ev.Key {
	case "rune":
		if ev.Alt {
			switch ev.Rune {
			case 'b':
				t.wordLeft()
			case 'f':
				t.wordRight()
			}
		} else {
			changed = t.Insert(string(ev.Rune))
		}
	case "enter":
		if !t.single {
			changed = t.Insert("\n")
		}
	case "backspace":
		if ev.Alt {
			changed = t.deleteWordBack()
		} else {
			changed = t.backspace()
		}
	case "delete", "ctrl+d":
		changed = t.del()
	case "left":
		if ev.Alt {
			t.wordLeft()
		} else {
			t.left()
		}
	case "right":
		if ev.Alt {
			t.wordRight()
		} else {
			t.right()
		}
	case "ctrl+left":
		t.wordLeft()
	case "ctrl+right":
		t.wordRight()
	case "up":
		t.moveV(-1)
		keepX = true
	case "down":
		t.moveV(1)
		keepX = true
	case "pgup", "pgdn":
		d := 1
		if ev.Key == "pgup" {
			d = -1
		}
		for i := 0; i < max(1, t.gh-1); i++ {
			t.moveV(d)
		}
		keepX = true
	case "home", "ctrl+a":
		vl := t.vlines(t.gw)
		idx, _ := t.cursorV(vl)
		t.col = vl[idx].start
	case "end", "ctrl+e":
		vl := t.vlines(t.gw)
		idx, _ := t.cursorV(vl)
		t.col = vl[idx].end
		if !lastOfRow(vl, idx) && t.col > vl[idx].start {
			t.col--
		}
	case "ctrl+home":
		t.row, t.col = 0, 0
	case "ctrl+end":
		t.row = len(t.lines) - 1
		t.col = len(t.lines[t.row])
	case "ctrl+u":
		changed = t.deleteRange(0)
	case "ctrl+k":
		l := t.lines[t.row]
		if t.col < len(l) {
			t.lines[t.row] = l[:t.col]
			changed = true
		} else {
			changed = t.del()
		}
	case "ctrl+w":
		changed = t.deleteWordBack()
	default:
		t.needEnsure = false
	}
	if !keepX {
		t.wantX = -1
	}
	return changed
}

// Draw renders the (wrapped) text into r; shows the cursor when focus is true.
func (t *TextArea) Draw(s *Screen, r Rect, st Style, focus bool) {
	t.gx, t.gy, t.gw, t.gh = r.X, r.Y, r.W, r.H
	vl := t.vlines(r.W)
	t.total = len(vl)
	idx, cx := t.cursorV(vl)
	if t.needEnsure {
		if idx < t.scroll {
			t.scroll = idx
		}
		if idx >= t.scroll+r.H {
			t.scroll = idx - r.H + 1
		}
		t.needEnsure = false
	}
	t.scroll = clamp(t.scroll, 0, max(0, len(vl)-r.H))
	for i := 0; i < r.H && t.scroll+i < len(vl); i++ {
		v := vl[t.scroll+i]
		s.Text(r.X, r.Y+i, string(t.lines[v.row][v.start:v.end]), st, r.W)
	}
	if focus {
		cy := idx - t.scroll
		if cy >= 0 && cy < r.H {
			s.SetCursor(r.X+cx, r.Y+cy)
		}
	}
}

// ClickAt moves the cursor to a screen position inside the last drawn area.
func (t *TextArea) ClickAt(x, y int) {
	vl := t.vlines(t.gw)
	i := clamp(t.scroll+(y-t.gy), 0, len(vl)-1)
	t.row = vl[i].row
	t.col = t.colAt(vl, i, max(0, x-t.gx))
	t.wantX = -1
}

func (t *TextArea) ScrollBy(d int) {
	t.scroll = clamp(t.scroll+d, 0, max(0, t.total-t.gh))
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
