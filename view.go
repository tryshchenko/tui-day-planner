package main

import (
	"fmt"
	"strings"
	"time"
)

func (a *App) hit(r Rect, fn func(x, y int)) { a.hits = append(a.hits, Hit{r, fn}) }

func (a *App) layout() {
	W, H := a.scr.W, a.scr.H
	body := Rect{0, 1, W, max(0, H-2)}
	a.listR, a.detR = body, Rect{}
	if a.showDetail {
		if W >= 72 {
			lw := W * 46 / 100
			lw = clamp(lw, 34, W-30)
			a.listR = Rect{0, 1, lw, body.H}
			a.detR = Rect{lw, 1, W - lw, body.H}
		} else if a.focus == FocusDetail {
			a.listR, a.detR = Rect{}, body
		}
	}
	a.listH = max(0, a.listR.H-2)
}

func (a *App) render() {
	s := a.scr
	s.Clear(sty(cText, cBg))
	a.hits = a.hits[:0]
	if s.W < 24 || s.H < 7 {
		msg := "Terminal too small"
		s.Text(max(0, (s.W-strWidth(msg))/2), s.H/2, msg, sty(cDim, cBg), s.W)
		return
	}
	a.layout()
	a.drawHeader()
	if a.listR.W > 0 {
		a.drawList()
	}
	if a.detR.W > 0 {
		a.drawDetail(a.detR)
	}
	a.drawFooter()
	if a.input != nil || a.confirm != nil || a.help {
		s.DimAll(cBg)
		a.hit(Rect{0, 0, s.W, s.H}, nil) // modal blocks clicks behind it
		switch {
		case a.help:
			a.drawHelp()
		case a.confirm != nil:
			a.drawConfirm()
		default:
			a.drawInput()
		}
	}
}

// ---- header / footer ----

func (a *App) chip(x, y int, key, label string, active bool, ev Event) int {
	s := a.scr
	ks := sty(cAccent, cSel, Bold)
	ls := sty(cDim, cBg)
	if active {
		ks = sty(cBg, cAccent, Bold)
		ls = sty(cText, cBg)
	}
	w := s.Text(x, y, " "+key+" ", ks, 40)
	w += s.Text(x+w, y, " "+label, ls, 40)
	a.hit(Rect{x, y, w + 1, 1}, func(int, int) { a.onKey(ev) })
	return w
}

func chipWidth(key, label string) int { return strWidth(key) + 2 + 1 + strWidth(label) }

func (a *App) drawHeader() {
	s := a.scr
	W := s.W
	bg := cBg
	x := 1
	x += s.Text(x, 0, "◆", sty(cViolet, bg, Bold), 2)
	x += s.Text(x, 0, " Day Planner", sty(cText, bg, Bold), W)

	// right side: chips, laid out right-to-left
	type chipSpec struct {
		key, label string
		active     bool
		ev         Event
	}
	chips := []chipSpec{
		{"c", "Completed", a.view == ViewDone, kev("c")},
		{"p", "Details", a.showDetail, kev("p")},
	}
	rx := W - 1
	for _, c := range chips {
		w := chipWidth(c.key, c.label)
		if rx-w-2 < x+2 {
			break
		}
		rx -= w
		a.chip(rx, 0, c.key, c.label, c.active, c.ev)
		rx -= 2
	}

	// progress
	if a.view == ViewMain {
		done, total := 0, len(a.tasks)
		for _, t := range a.tasks {
			if t.Done() {
				done++
			}
		}
		if total > 0 {
			const bw = 8
			filled := done * bw / total
			txt := fmt.Sprintf(" %d/%d", done, total)
			w := bw + strWidth(txt)
			if rx-w > x+2 {
				rx -= w
				s.Text(rx, 0, strings.Repeat("▰", filled), sty(cGreen, bg), bw)
				s.Text(rx+filled, 0, strings.Repeat("▱", bw-filled), sty(cFaint, bg), bw)
				s.Text(rx+bw, 0, txt, sty(cDim, bg), 10)
				rx -= 2
			}
		}
	}

	// date
	date := a.now().Format("Monday, January 2")
	if x+3+strWidth(date) < rx {
		s.Text(x+2, 0, "·  "+date, sty(cDim, bg), rx-x-2)
	}
}

type hint struct {
	key, label string
	ev         Event
}

func (a *App) hints() []hint {
	switch {
	case a.focus == FocusDetail:
		return []hint{{"esc", "Back to tasks", kev("esc")}, {"ctrl+s", "Save", kev("ctrl+s")}, {"tab", "Tasks", kev("tab")}}
	case a.view == ViewDone:
		return []hint{
			{"space", "Restore", kev(" ")}, {"e", "Edit", kev("e")}, {"tab", "Details", kev("tab")},
			{"x", "Delete", kev("x")}, {"c", "Back", kev("c")}, {"p", "Pane", kev("p")},
		}
	}
	return []hint{
		{"space", "Toggle", kev(" ")}, {"n", "New", kev("n")}, {"e", "Edit", kev("e")},
		{"tab", "Details", kev("tab")}, {"c", "Completed", kev("c")}, {"t", "To today", kev("t")},
		{"x", "Delete", kev("x")}, {"z", "Fold", kev("z")}, {"p", "Pane", kev("p")}, {"q", "Quit", kev("q")},
	}
}

func (a *App) drawFooter() {
	s := a.scr
	y := s.H - 1
	if time.Now().Before(a.toastUntil) {
		fg := cText
		icon := "ℹ"
		switch a.toastKind {
		case toastOK:
			fg, icon = cGreen, "✓"
		case toastErr:
			fg, icon = cRed, "✗"
		default:
			fg = cCyan
		}
		x := 1
		x += s.Text(x, y, icon+" ", sty(fg, cBg, Bold), 3)
		s.Text(x, y, a.toastMsg, sty(cText, cBg), s.W-x-1)
		return
	}
	helpW := chipWidth("?", "Help")
	x := 1
	for _, h := range a.hints() {
		w := chipWidth(h.key, h.label)
		if x+w+2 > s.W-helpW-3 {
			break
		}
		a.chip(x, y, h.key, h.label, false, h.ev)
		x += w + 2
	}
	a.chip(s.W-helpW-1, y, "?", "Help", false, kev("?"))
}

// ---- lists ----

func paneStyles(focused bool) (border, title Style) {
	if focused {
		return sty(cAccent, cPanel), sty(cAccent, cPanel, Bold)
	}
	return sty(cBorder, cPanel), sty(cDim, cPanel, Bold)
}

func (a *App) drawList() {
	s := a.scr
	r := a.listR
	l := a.cur()
	focused := a.focus == FocusList
	bs, ts := paneStyles(focused)
	s.Box(r, bs, cPanel)
	title := "Tasks"
	if a.view == ViewDone {
		title = fmt.Sprintf("Completed · %d", len(a.done))
	}
	s.Text(r.X+2, r.Y, " "+title+" ", ts, r.W-4)
	a.hit(r, func(int, int) { a.setFocus(FocusList) })

	in := Rect{r.X + 1, r.Y + 1, r.W - 2, r.H - 2}
	l.scroll = clamp(l.scroll, 0, max(0, len(l.rows)-in.H))

	if len(l.rows) == 0 || (a.view == ViewDone && len(a.done) == 0) {
		msg := "No completed tasks yet"
		s.Text(in.X+max(0, (in.W-strWidth(msg))/2), in.Y+in.H/2, msg, sty(cDim, cPanel, Italic), in.W)
		return
	}
	for i := 0; i < in.H && l.scroll+i < len(l.rows); i++ {
		row := l.rows[l.scroll+i]
		y := in.Y + i
		switch row.Kind {
		case RowHeader:
			a.drawHeaderRow(in, y, row)
		case RowHint:
			s.Text(in.X+3, y, row.Label, sty(cFaint, cPanel, Italic), in.W-4)
		case RowTask:
			a.drawTaskRow(in, y, row, l, focused)
		}
	}
	if len(l.rows) > in.H {
		scrollbar(s, r.X+r.W-1, in.Y, in.H, len(l.rows), l.scroll, in.H, sty(cDim, cPanel))
	}
}

func scrollbar(s *Screen, x, y, h, total, offset, visible int, st Style) {
	thumb := max(1, h*visible/total)
	maxOff := total - visible
	pos := 0
	if maxOff > 0 {
		pos = offset * (h - thumb) / maxOff
	}
	for i := 0; i < thumb; i++ {
		s.Text(x, y+pos+i, "┃", st, 1)
	}
}

func (a *App) drawHeaderRow(in Rect, y int, row Row) {
	s := a.scr
	col := cDim
	label := row.Label
	bg := cPanel
	x := in.X + 1
	if a.view == ViewMain {
		col = secColor[row.Sec]
		arrow := "▾"
		if row.Collapsed {
			arrow = "▸"
		}
		x += s.Text(x, y, arrow+" ", sty(col, bg), 3)
		sec := row.Sec
		a.hit(Rect{in.X, y, in.W, 1}, func(int, int) {
			a.setFocus(FocusList)
			a.toggleSection(sec)
		})
	} else {
		x++
	}
	x += s.Text(x, y, label, sty(col, bg, Bold), in.W)
	if row.Sub != "" {
		x += s.Text(x, y, "  "+row.Sub, sty(cFaint, bg), in.X+in.W-x)
	}
	count := fmt.Sprintf("%d", row.Count)
	if a.view == ViewMain && row.Count > 0 {
		count = fmt.Sprintf("%d/%d", row.Done, row.Count)
	}
	cx := in.X + in.W - strWidth(count) - 2
	if cx > x+2 {
		s.Text(x+1, y, strings.Repeat("─", cx-x-2), sty(cFaint, bg), cx-x-2)
		s.Text(cx, y, count, sty(cDim, bg), 8)
	}
}

func (a *App) rightInfo(t *Task, sec Section) (txt string, st Style) {
	if a.view != ViewMain {
		return "", Style{}
	}
	switch sec {
	case SecYesterday:
		if a.todayT.Weekday() == time.Monday {
			d, _ := time.ParseInLocation(dayFmt, t.Day, time.Local)
			return d.Format("Mon"), sty(cFaint, cPanel)
		}
	case SecOld:
		d, _ := time.ParseInLocation(dayFmt, t.Day, time.Local)
		return d.Format("Jan 2"), sty(cFaint, cPanel)
	}
	return "", Style{}
}

func shortDate(t, now time.Time) string {
	if t.Year() != now.Year() {
		return t.Format("Jan 2 2006")
	}
	return t.Format("Jan 2")
}

func (a *App) drawTaskRow(in Rect, y int, row Row, l *ListState, focused bool) {
	s := a.scr
	t := row.Task
	sel := t.ID == l.selID
	bg := cPanel
	if sel {
		bg = cSelDim
		if focused {
			bg = cSel
		}
	}
	s.Fill(Rect{in.X, y, in.W, 1}, sty(cText, bg))
	secCol := cDim
	if a.view == ViewMain {
		secCol = secColor[row.Sec]
	}
	if sel {
		barCol := cAccent
		if !focused {
			barCol = cFaint
		}
		s.Text(in.X, y, "▌", sty(barCol, bg), 1)
	}
	glyph, gs := "○", sty(secCol, bg)
	ts := sty(cText, bg)
	if t.Done() {
		glyph, gs = "●", sty(cGreen, bg)
		ts = sty(cDim, bg, Strike)
	}
	s.Text(in.X+2, y, glyph, gs, 1)

	// right-hand info
	right := in.X + in.W - 1
	if t.Details != "" || (t.ID == a.detailTask.idOr0() && !a.detail.Empty()) {
		s.Text(right-1, y, "≡", sty(cFaint, bg), 1)
		right -= 3
	}
	if a.view == ViewDone {
		right = a.drawDoneColumns(in, y, t, bg, right)
	} else if txt, st := a.rightInfo(t, row.Sec); txt != "" {
		st.BG = bg
		w := strWidth(txt)
		s.Text(right-w+1, y, txt, st, w)
		right -= w + 2
	}
	avail := right - (in.X + 4)
	s.Text(in.X+4, y, truncate(t.Title, avail), ts, max(0, avail))

	rowRect := Rect{in.X, y, in.W, 1}
	a.hit(rowRect, func(int, int) { a.clickTask(t) })
	a.hit(Rect{in.X + 1, y, 3, 1}, func(int, int) {
		a.setFocus(FocusList)
		a.selectTask(t)
		a.toggleTask(t)
	})
}

func (t *Task) idOr0() int64 {
	if t == nil {
		return 0
	}
	return t.ID
}

// drawDoneColumns draws created/completed info; returns the new right edge for the title.
func (a *App) drawDoneColumns(in Rect, y int, t *Task, bg Color, right int) int {
	s := a.scr
	if t.Restored {
		txt := "↩ restored to Today"
		if in.W < 50 {
			txt = "↩ Today"
		}
		w := strWidth(txt)
		s.Text(right-w+1, y, txt, sty(cGreen, bg), w)
		return right - w - 2
	}
	doneTxt := "✓ " + t.DoneAt.Format("15:04")
	w := strWidth(doneTxt)
	if in.W >= 40 {
		s.Text(right-w+1, y, doneTxt, sty(cGreen, bg), w)
		right -= w + 2
	}
	if in.W >= 58 {
		cr := "added " + shortDate(t.CreatedAt, a.now())
		cw := strWidth(cr)
		s.Text(right-cw+1, y, cr, sty(cFaint, bg), cw)
		right -= cw + 2
	}
	return right
}

// ---- details pane ----

func (a *App) sectionLabel(t *Task) (string, Color) {
	if a.view == ViewDone {
		return "", cGreen
	}
	sec := a.sectionOf(t.Day)
	switch sec {
	case SecToday:
		return "Today", secColor[sec]
	case SecYesterday:
		if a.todayT.Weekday() == time.Monday {
			d, _ := time.ParseInLocation(dayFmt, t.Day, time.Local)
			return d.Format("Monday"), secColor[sec]
		}
		return "Yesterday", secColor[sec]
	}
	d, _ := time.ParseInLocation(dayFmt, t.Day, time.Local)
	days := int(a.todayT.Sub(d).Hours()/24 + 0.5)
	return fmt.Sprintf("Old · %d days ago", days), secColor[sec]
}

func (a *App) drawDetail(r Rect) {
	s := a.scr
	focused := a.focus == FocusDetail
	bs, ts := paneStyles(focused)
	s.Box(r, bs, cPanel)
	s.Text(r.X+2, r.Y, " Details ", ts, r.W-4)
	a.hit(r, func(int, int) { a.focusDetail() })

	in := Rect{r.X + 2, r.Y + 1, r.W - 4, r.H - 2}
	t := a.currentTask()
	if t == nil {
		msg := "Select a task to see its details"
		s.Text(in.X+max(0, (in.W-strWidth(msg))/2), in.Y+in.H/2, msg, sty(cDim, cPanel, Italic), in.W)
		return
	}
	y := in.Y
	// title (wrapped, at most 3 lines)
	segs := wrapRunes([]rune(t.Title), in.W)
	tr := []rune(t.Title)
	nl := min(len(segs), 3)
	for i := 0; i < nl; i++ {
		line := string(tr[segs[i][0]:segs[i][1]])
		if i == nl-1 && len(segs) > nl {
			line = truncate(string(tr[segs[i][0]:]), in.W)
		}
		ts := sty(cText, cPanel, Bold)
		if t.Done() {
			ts = sty(cDim, cPanel, Bold, Strike)
		}
		s.Text(in.X, y, strings.TrimRight(line, " "), ts, in.W)
		y++
	}
	titleRect := Rect{in.X, in.Y, in.W, nl}
	a.hit(titleRect, func(int, int) { a.askEdit(a.currentTask()) })

	// status + dates
	x := in.X
	if t.Done() {
		x += s.Text(x, y, "● Done", sty(cGreen, cPanel, Bold), in.W)
	} else {
		x += s.Text(x, y, "○ Open", sty(cYellow, cPanel, Bold), in.W)
	}
	if lbl, lc := a.sectionLabel(t); lbl != "" {
		x += s.Text(x, y, "  ·  ", sty(cFaint, cPanel), in.X+in.W-x)
		s.Text(x, y, lbl, sty(lc, cPanel), in.X+in.W-x)
	}
	y++
	now := a.now()
	meta := "created " + t.CreatedAt.Format("Jan 2 15:04")
	if t.CreatedAt.Year() != now.Year() {
		meta = "created " + t.CreatedAt.Format("Jan 2 2006 15:04")
	}
	if !t.DoneAt.IsZero() && (t.Done() || a.view == ViewDone) {
		meta += "  ·  crossed out " + t.DoneAt.Format("Jan 2 15:04")
	}
	s.Text(in.X, y, truncate(meta, in.W), sty(cDim, cPanel), in.W)
	y++
	s.Text(in.X, y, strings.Repeat("─", in.W), sty(cFaint, cPanel), in.W)
	y++

	body := Rect{in.X, y, in.W, in.Y + in.H - y}
	if body.H <= 0 {
		return
	}
	if a.detail.Empty() && !focused {
		s.Text(body.X, body.Y, truncate("No details yet — press Enter or click to add notes", body.W),
			sty(cFaint, cPanel, Italic), body.W)
	}
	a.detail.Draw(s, body, sty(cText, cPanel), focused)
	a.hit(body, func(x, yy int) {
		a.focusDetail()
		a.detail.ClickAt(x, yy)
	})
	if a.detail.total > body.H {
		scrollbar(s, r.X+r.W-1, body.Y, body.H, a.detail.total, a.detail.scroll, body.H, sty(cDim, cPanel))
	}
}

// ---- overlays ----

func (a *App) modalBox(w, h int, title string) Rect {
	s := a.scr
	w = min(w, s.W-2)
	h = min(h, s.H-2)
	r := Rect{(s.W - w) / 2, max(1, (s.H-h)/3), w, h}
	s.Box(r, sty(cAccent, cModal), cModal)
	s.Text(r.X+2, r.Y, " "+title+" ", sty(cAccent, cModal, Bold), r.W-4)
	return r
}

func (a *App) drawInput() {
	s := a.scr
	in := a.input
	w := min(64, s.W-4)
	innerW := w - 4
	lines := min(4, max(1, len(in.ta.vlines(innerW))))
	r := a.modalBox(w, lines+4, in.title)
	body := Rect{r.X + 2, r.Y + 1, innerW, lines}
	// bring the cursor into view inside a tall field
	in.ta.needEnsure = true
	in.ta.Draw(s, body, sty(cText, cModal), true)
	hy := r.Y + r.H - 2
	x := r.X + 2
	x += a.chip(x, hy, "⏎", "Save", false, kev("enter")) + 3
	a.chip(x, hy, "esc", "Cancel", false, kev("esc"))
	a.hit(body, func(x, y int) { in.ta.ClickAt(x, y) })
}

func (a *App) drawConfirm() {
	s := a.scr
	c := a.confirm
	r := a.modalBox(50, 7, "Delete task?")
	s.Text(r.X+2, r.Y+2, truncate("“"+c.task.Title+"”", r.W-4), sty(cText, cModal), r.W-4)
	s.Text(r.X+2, r.Y+3, "This cannot be undone.", sty(cDim, cModal), r.W-4)
	by := r.Y + r.H - 2
	btn := func(x int, label string, active bool, col Color, fn func(int, int)) int {
		st := sty(col, cSel, Bold)
		if active {
			st = sty(cBg, col, Bold)
		}
		w := s.Text(x, by, " "+label+" ", st, 20)
		a.hit(Rect{x, by, w, 1}, fn)
		return w
	}
	x := r.X + 2
	x += btn(x, "Delete (y)", c.yes, cRed, func(int, int) { a.confirm = nil; a.deleteTask(c.task) }) + 2
	btn(x, "Cancel (n)", !c.yes, cAccent, func(int, int) { a.confirm = nil })
}

func (a *App) drawHelp() {
	s := a.scr
	lines := [][2]string{
		{"↑ ↓  j k", "move selection"},
		{"g  G", "first / last task"},
		{"space", "cross out / un-cross task"},
		{"n  a", "new task (added to Today)"},
		{"e  F2", "edit title"},
		{"t", "move task to Today"},
		{"x  Del", "delete task"},
		{"z", "fold / unfold section"},
		{"tab  ⏎", "focus details pane (and edit notes)"},
		{"esc  tab", "back to the task list"},
		{"p", "show / hide details pane"},
		{"c", "completed tasks page"},
		{"q  ctrl+c", "quit (q goes back from Completed)"},
		{"mouse", "click, double-click for details, wheel"},
	}
	r := a.modalBox(64, len(lines)+4, "Keyboard & mouse")
	for i, l := range lines {
		y := r.Y + 2 + i
		s.Text(r.X+3, y, l[0], sty(cAccent, cModal, Bold), 14)
		s.Text(r.X+18, y, l[1], sty(cText, cModal), r.W-20)
	}
}
