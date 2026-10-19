package main

import "time"

func (a *App) handle(ev Event) {
	a.layout()
	switch ev.Type {
	case EvTick:
		a.tick()
	case EvQuit:
		a.shutdown()
	case EvKey:
		a.onKey(ev)
	case EvPaste:
		a.onPaste(ev.Text)
	case EvMouse:
		a.onMouse(ev)
	}
}

func (a *App) onKey(ev Event) {
	if ev.Key == "ctrl+c" {
		a.shutdown()
		return
	}
	switch {
	case a.help:
		a.help = false
	case a.confirm != nil:
		a.confirmKey(ev)
	case a.input != nil:
		a.inputKey(ev)
	case a.focus == FocusDetail:
		a.detailKey(ev)
	default:
		a.listKey(ev)
	}
}

func (a *App) onPaste(text string) {
	switch {
	case a.input != nil:
		a.input.ta.Insert(text)
	case a.focus == FocusDetail && a.detailTask != nil && !a.help && a.confirm == nil:
		if a.detail.Insert(text) {
			a.markDirty()
		}
	}
}

func (a *App) markDirty() {
	a.detailDirty = true
	a.lastEdit = a.now()
}

func (a *App) inputKey(ev Event) {
	switch ev.Key {
	case "esc":
		a.input = nil
	case "enter":
		a.submitInput()
	default:
		a.input.ta.HandleKey(ev)
	}
}

func (a *App) confirmKey(ev Event) {
	c := a.confirm
	switch {
	case ev.Key == "esc", ev.Key == "rune" && (ev.Rune == 'n' || ev.Rune == 'N' || ev.Rune == 'q'):
		a.confirm = nil
	case ev.Key == "rune" && (ev.Rune == 'y' || ev.Rune == 'Y'):
		a.confirm = nil
		a.deleteTask(c.task)
	case ev.Key == "left", ev.Key == "right", ev.Key == "tab", ev.Key == "shift+tab",
		ev.Key == "rune" && (ev.Rune == 'h' || ev.Rune == 'l'):
		c.yes = !c.yes
	case ev.Key == "enter":
		a.confirm = nil
		if c.yes {
			a.deleteTask(c.task)
		}
	}
}

func (a *App) detailKey(ev Event) {
	switch ev.Key {
	case "esc", "tab", "shift+tab":
		a.setFocus(FocusList)
		return
	case "ctrl+s":
		a.saveDetail()
		a.flash(toastOK, "Saved")
		return
	}
	if a.detailTask == nil {
		a.setFocus(FocusList)
		return
	}
	if a.detail.HandleKey(ev) {
		a.markDirty()
	}
}

func (a *App) listKey(ev Event) {
	t := a.currentTask()
	inDone := a.view == ViewDone
	switch ev.Key {
	case "up":
		a.moveSel(-1)
	case "down":
		a.moveSel(1)
	case "pgup":
		a.moveSel(-max(1, a.listH-2))
	case "pgdn":
		a.moveSel(max(1, a.listH-2))
	case "home":
		a.selectEdge(true)
	case "end":
		a.selectEdge(false)
	case "enter", "tab", "right":
		a.focusDetail()
	case "delete":
		a.askDelete(t)
	case "f2":
		a.askEdit(t)
	case "esc":
		if inDone {
			a.closeDone()
		}
	case "rune":
		if ev.Alt {
			return
		}
		switch ev.Rune {
		case ' ':
			a.toggleTask(t)
		case 'j':
			a.moveSel(1)
		case 'k':
			a.moveSel(-1)
		case 'g':
			a.selectEdge(true)
		case 'G':
			a.selectEdge(false)
		case 'n', 'a':
			if inDone {
				a.closeDone()
			}
			a.openInput("New task", 0, "")
		case 'e':
			a.askEdit(t)
		case 'x':
			a.askDelete(t)
		case 't':
			a.moveToToday(t)
		case 'z':
			if !inDone {
				a.toggleSectionOfSelection()
			}
		case 'p':
			a.togglePane()
		case 'c':
			if inDone {
				a.closeDone()
			} else {
				a.openDone()
			}
		case '?':
			a.help = true
		case 'q':
			if inDone {
				a.closeDone()
			} else {
				a.shutdown()
			}
		}
	}
}

func (a *App) askEdit(t *Task) {
	if t != nil {
		a.openInput("Edit task", t.ID, t.Title)
	}
}

func (a *App) askDelete(t *Task) {
	if t != nil {
		a.confirm = &Confirm{task: t}
	}
}

// ---- mouse ----

func (a *App) onMouse(ev Event) {
	switch {
	case ev.Btn == 0 && ev.Press:
		for i := len(a.hits) - 1; i >= 0; i-- {
			if h := a.hits[i]; h.R.Contains(ev.X, ev.Y) {
				if h.Fn != nil {
					h.Fn(ev.X, ev.Y)
				}
				return
			}
		}
	case ev.Btn == 64 || ev.Btn == 65:
		if a.input != nil || a.confirm != nil || a.help {
			return
		}
		d := 3
		if ev.Btn == 64 {
			d = -3
		}
		switch {
		case a.listR.Contains(ev.X, ev.Y):
			l := a.cur()
			l.scroll = clamp(l.scroll+d, 0, max(0, len(l.rows)-a.listH))
		case a.showDetail && a.detR.Contains(ev.X, ev.Y):
			a.detail.ScrollBy(d)
		}
	}
}

// clickTask handles a click on a task row: select, and open details on double click.
func (a *App) clickTask(t *Task) {
	a.setFocus(FocusList)
	a.selectTask(t)
	now := time.Now()
	if a.lastClickID == t.ID && now.Sub(a.lastClickAt) < 400*time.Millisecond {
		a.lastClickID = 0
		a.focusDetail()
		return
	}
	a.lastClickID, a.lastClickAt = t.ID, now
}
