package main

import (
	"fmt"
	"strings"
	"time"
)

type Focus int

const (
	FocusList Focus = iota
	FocusDetail
)

type View int

const (
	ViewMain View = iota
	ViewDone
)

type Hit struct {
	R  Rect
	Fn func(x, y int)
}

type Input struct {
	title  string
	ta     *TextArea
	editID int64 // 0 = new task
}

type Confirm struct {
	task *Task
	yes  bool
}

type toastKind int

const (
	toastInfo toastKind = iota
	toastOK
	toastErr
)

type App struct {
	db  *DB
	scr *Screen
	now func() time.Time

	todayT   time.Time // local midnight
	todayStr string
	yestFrom string // first day of the "Yesterday" section

	tasks     []*Task
	main      ListState
	done      []*Task
	doneL     ListState
	collapsed [3]bool

	view       View
	focus      Focus
	showDetail bool

	detail      *TextArea
	detailTask  *Task
	detailDirty bool
	lastEdit    time.Time

	input   *Input
	confirm *Confirm
	help    bool

	toastMsg   string
	toastKind  toastKind
	toastUntil time.Time

	listR, detR Rect
	listH       int
	hits        []Hit
	lastClickID int64
	lastClickAt time.Time

	dataVer  int64
	quitting bool
}

func NewApp(db *DB, w, h int) *App {
	a := &App{
		db:         db,
		scr:        NewScreen(w, h),
		now:        time.Now,
		detail:     NewTextArea(false),
		showDetail: db.Setting("detail", "1") == "1",
	}
	a.dataVer, _ = db.DataVersion()
	a.reload()
	return a
}

func (a *App) cur() *ListState {
	if a.view == ViewDone {
		return &a.doneL
	}
	return &a.main
}

func (a *App) currentTask() *Task { return a.cur().selected() }

func (a *App) setDates(now time.Time) {
	a.todayT = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	a.todayStr = a.todayT.Format(dayFmt)
	back := 1
	if a.todayT.Weekday() == time.Monday {
		back = 3 // Friday..Sunday
	}
	a.yestFrom = a.todayT.AddDate(0, 0, -back).Format(dayFmt)
}

func (a *App) sectionOf(day string) Section {
	switch {
	case day >= a.todayStr:
		return SecToday
	case day >= a.yestFrom:
		return SecYesterday
	}
	return SecOld
}

func (a *App) flash(kind toastKind, format string, args ...any) {
	a.toastMsg = fmt.Sprintf(format, args...)
	a.toastKind = kind
	a.toastUntil = time.Now().Add(3 * time.Second)
}

func (a *App) fail(err error) { a.flash(toastErr, "%v", err) }

// ---- loading ----

func (a *App) reload() {
	a.saveDetail()
	a.setDates(a.now())
	tasks, err := a.db.Active(a.todayT)
	if err != nil {
		a.fail(err)
		return
	}
	a.tasks = tasks
	a.main.setRows(a.buildRows())
	a.syncDetail()
}

func (a *App) buildRows() []Row {
	var groups [3][]*Task
	for _, t := range a.tasks {
		s := a.sectionOf(t.Day)
		groups[s] = append(groups[s], t)
	}
	var rows []Row
	for sec := SecToday; sec <= SecOld; sec++ {
		g := groups[sec]
		if sec != SecToday && len(g) == 0 {
			continue
		}
		if len(rows) > 0 {
			rows = append(rows, Row{Kind: RowBlank})
		}
		done := 0
		for _, t := range g {
			if t.Done() {
				done++
			}
		}
		h := Row{Kind: RowHeader, Sec: sec, Count: len(g), Done: done, Collapsed: a.collapsed[sec]}
		switch sec {
		case SecToday:
			h.Label, h.Sub = "TODAY", a.todayT.Format("Mon, Jan 2")
		case SecYesterday:
			h.Label = "YESTERDAY"
			if a.todayT.Weekday() == time.Monday {
				h.Sub = "Fri – Sun"
			} else {
				h.Sub = a.todayT.AddDate(0, 0, -1).Format("Mon, Jan 2")
			}
		case SecOld:
			h.Label = "OLD"
		}
		rows = append(rows, h)
		if h.Collapsed {
			continue
		}
		if len(g) == 0 {
			rows = append(rows, Row{Kind: RowHint, Label: "Nothing yet — press n to add a task"})
		}
		for _, t := range g {
			rows = append(rows, Row{Kind: RowTask, Sec: sec, Task: t})
		}
	}
	return rows
}

func (a *App) loadDone() {
	tasks, err := a.db.Completed()
	if err != nil {
		a.fail(err)
		return
	}
	for _, t := range tasks {
		t.Group = t.DoneAt.Format(dayFmt)
	}
	a.done = tasks
	a.rebuildDone()
}

func (a *App) rebuildDone() {
	yest := a.todayT.AddDate(0, 0, -1).Format(dayFmt)
	var rows []Row
	for i := 0; i < len(a.done); {
		j := i
		for j < len(a.done) && a.done[j].Group == a.done[i].Group {
			j++
		}
		g := a.done[i:j]
		if len(rows) > 0 {
			rows = append(rows, Row{Kind: RowBlank})
		}
		h := Row{Kind: RowHeader, Count: len(g)}
		switch g[0].Group {
		case a.todayStr:
			h.Label, h.Sub = "TODAY", g[0].DoneAt.Format("Mon, Jan 2")
		case yest:
			h.Label, h.Sub = "YESTERDAY", g[0].DoneAt.Format("Mon, Jan 2")
		default:
			h.Label = strings.ToUpper(g[0].DoneAt.Format("Mon, Jan 2"))
			if g[0].DoneAt.Year() != a.todayT.Year() {
				h.Label += g[0].DoneAt.Format(" 2006")
			}
		}
		rows = append(rows, h)
		for _, t := range g {
			rows = append(rows, Row{Kind: RowTask, Task: t})
		}
		i = j
	}
	a.doneL.setRows(rows)
	a.syncDetail()
}

// ---- detail editor sync ----

func (a *App) syncDetail() {
	t := a.currentTask()
	if t == nil {
		a.saveDetail()
		a.detailTask = nil
		a.detail.SetText("")
		return
	}
	if a.detailTask != nil && a.detailTask.ID == t.ID {
		if a.detailTask != t {
			a.detailTask = t
			if !a.detailDirty && a.focus != FocusDetail && t.Details != a.detail.Text() {
				a.detail.SetText(t.Details)
			}
		}
		return
	}
	a.saveDetail()
	a.detailTask = t
	a.detail.SetText(t.Details)
	a.detailDirty = false
}

func (a *App) saveDetail() {
	if !a.detailDirty || a.detailTask == nil {
		a.detailDirty = false
		return
	}
	txt := a.detail.Text()
	if err := a.db.UpdateDetails(a.detailTask.ID, txt); err != nil {
		a.fail(err)
		return
	}
	a.detailTask.Details = txt
	a.detailDirty = false
}

// ---- actions ----

func (a *App) shutdown() {
	a.saveDetail()
	a.quitting = true
}

func (a *App) moveSel(delta int) {
	l := a.cur()
	l.move(delta)
	l.ensureVisible(a.listH)
	a.syncDetail()
}

func (a *App) selectEdge(first bool) {
	l := a.cur()
	l.moveTo(first)
	l.ensureVisible(a.listH)
	a.syncDetail()
}

func (a *App) selectTask(t *Task) {
	l := a.cur()
	l.selID = t.ID
	a.syncDetail()
}

func (a *App) toggleTask(t *Task) {
	if t == nil {
		return
	}
	now := a.now()
	if a.view == ViewDone {
		if t.Done() {
			if err := a.db.Restore(t.ID, now); err != nil {
				a.fail(err)
				return
			}
			t.Restored = true
			t.Day = a.todayStr
			a.flash(toastOK, "Restored “%s” to Today", truncate(t.Title, 30))
		} else {
			if err := a.db.SetDone(t.ID, &now); err != nil {
				a.fail(err)
				return
			}
			t.Restored = false
			t.DoneAt = now
		}
		return
	}
	var err error
	if t.Done() {
		err = a.db.SetDone(t.ID, nil)
		t.DoneAt = time.Time{}
	} else {
		err = a.db.SetDone(t.ID, &now)
		t.DoneAt = now
	}
	if err != nil {
		a.fail(err)
		return
	}
	a.main.setRows(a.buildRows())
}

func (a *App) addTask(title string) {
	id, err := a.db.Add(title, a.now())
	if err != nil {
		a.fail(err)
		return
	}
	a.collapsed[SecToday] = false
	a.main.selID = id
	a.reload()
	a.main.ensureVisible(a.listH)
	a.flash(toastOK, "Added to Today")
}

func (a *App) renameTask(id int64, title string) {
	if err := a.db.UpdateTitle(id, title); err != nil {
		a.fail(err)
		return
	}
	for _, t := range a.tasks {
		if t.ID == id {
			t.Title = title
		}
	}
	for _, t := range a.done {
		if t.ID == id {
			t.Title = title
		}
	}
}

func (a *App) deleteTask(t *Task) {
	if a.detailTask != nil && a.detailTask.ID == t.ID {
		a.detailDirty = false
		a.detailTask = nil
	}
	if err := a.db.Delete(t.ID); err != nil {
		a.fail(err)
		return
	}
	if a.view == ViewDone {
		for i, d := range a.done {
			if d.ID == t.ID {
				a.done = append(a.done[:i], a.done[i+1:]...)
				break
			}
		}
		a.rebuildDone()
	} else {
		a.reload()
	}
	a.cur().ensureVisible(a.listH)
	a.flash(toastInfo, "Deleted")
}

func (a *App) moveToToday(t *Task) {
	if t == nil || t.Done() || a.view != ViewMain {
		return
	}
	if a.sectionOf(t.Day) == SecToday {
		a.flash(toastInfo, "Already in Today")
		return
	}
	if err := a.db.MoveToToday(t.ID, a.now()); err != nil {
		a.fail(err)
		return
	}
	a.collapsed[SecToday] = false
	a.reload()
	a.main.ensureVisible(a.listH)
	a.flash(toastOK, "Moved to Today")
}

func (a *App) toggleSection(sec Section) {
	a.collapsed[sec] = !a.collapsed[sec]
	a.main.setRows(a.buildRows())
	a.syncDetail()
}

func (a *App) toggleSectionOfSelection() {
	if t := a.main.selected(); t != nil {
		a.toggleSection(a.sectionOf(t.Day))
	}
}

func (a *App) togglePane() {
	a.showDetail = !a.showDetail
	v := "0"
	if a.showDetail {
		v = "1"
	}
	a.db.SetSetting("detail", v)
	if !a.showDetail {
		a.setFocus(FocusList)
	}
}

func (a *App) setFocus(f Focus) {
	if a.focus == FocusDetail && f != FocusDetail {
		a.saveDetail()
	}
	a.focus = f
}

func (a *App) focusDetail() {
	if a.currentTask() == nil {
		return
	}
	if !a.showDetail {
		a.showDetail = true
		a.db.SetSetting("detail", "1")
	}
	a.focus = FocusDetail
	a.detail.needEnsure = true
}

func (a *App) openDone() {
	a.setFocus(FocusList)
	a.saveDetail()
	a.view = ViewDone
	a.doneL = ListState{}
	a.loadDone()
}

func (a *App) closeDone() {
	a.setFocus(FocusList)
	a.saveDetail()
	a.view = ViewMain
	a.reload()
}

func (a *App) openInput(title string, editID int64, text string) {
	ta := NewTextArea(true)
	ta.SetText(text)
	a.input = &Input{title: title, ta: ta, editID: editID}
}

func (a *App) submitInput() {
	in := a.input
	text := strings.TrimSpace(in.ta.Text())
	a.input = nil
	if text == "" {
		return
	}
	if in.editID == 0 {
		a.addTask(text)
	} else {
		a.renameTask(in.editID, text)
	}
}

func (a *App) tick() {
	now := a.now()
	if a.detailDirty && now.Sub(a.lastEdit) > 900*time.Millisecond {
		a.saveDetail()
	}
	if now.Format(dayFmt) != a.todayStr {
		a.reload()
		if a.view == ViewDone {
			a.loadDone()
		}
		a.flash(toastInfo, "New day — yesterday's crossed-out tasks moved to Completed")
		return
	}
	if v, err := a.db.DataVersion(); err == nil && v != a.dataVer {
		a.dataVer = v
		a.reload()
		if a.view == ViewDone {
			a.loadDone()
		}
	}
}
