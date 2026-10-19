package main

type RowKind int

const (
	RowTask RowKind = iota
	RowHeader
	RowBlank
	RowHint
)

type Section int

const (
	SecToday Section = iota
	SecYesterday
	SecOld
)

type Row struct {
	Kind        RowKind
	Sec         Section
	Label, Sub  string
	Count, Done int
	Collapsed   bool
	Task        *Task
}

// ListState is a scrollable list of rows with a selected task.
type ListState struct {
	rows   []Row
	selID  int64
	scroll int
}

func (l *ListState) selIndex() int {
	for i, r := range l.rows {
		if r.Kind == RowTask && r.Task.ID == l.selID {
			return i
		}
	}
	return -1
}

func (l *ListState) selected() *Task {
	if i := l.selIndex(); i >= 0 {
		return l.rows[i].Task
	}
	return nil
}

func (l *ListState) taskRows() []int {
	var out []int
	for i, r := range l.rows {
		if r.Kind == RowTask {
			out = append(out, i)
		}
	}
	return out
}

// setRows swaps in new rows, keeping the selection (or the nearest task).
func (l *ListState) setRows(rows []Row) {
	prev := max(0, l.selIndex())
	l.rows = rows
	if l.selIndex() >= 0 {
		return
	}
	best := -1
	for i, r := range rows {
		if r.Kind == RowTask && (best < 0 || abs(i-prev) < abs(best-prev)) {
			best = i
		}
	}
	l.selID = 0
	if best >= 0 {
		l.selID = rows[best].Task.ID
	}
}

func (l *ListState) move(delta int) {
	idx := l.taskRows()
	if len(idx) == 0 {
		return
	}
	pos := 0
	cur := l.selIndex()
	for p, i := range idx {
		if i == cur {
			pos = p
		}
	}
	pos = clamp(pos+delta, 0, len(idx)-1)
	l.selID = l.rows[idx[pos]].Task.ID
}

func (l *ListState) moveTo(first bool) {
	idx := l.taskRows()
	if len(idx) == 0 {
		return
	}
	if first {
		l.selID = l.rows[idx[0]].Task.ID
	} else {
		l.selID = l.rows[idx[len(idx)-1]].Task.ID
	}
}

func (l *ListState) ensureVisible(h int) {
	i := l.selIndex()
	if i < 0 || h <= 0 {
		return
	}
	if i < l.scroll {
		l.scroll = i
	}
	if i >= l.scroll+h {
		l.scroll = i - h + 1
	}
	// keep the section header in view when selecting its first task
	if i > 0 && l.rows[i-1].Kind == RowHeader && l.scroll > i-1 {
		l.scroll = i - 1
	}
	if l.selID == l.firstTaskID() {
		l.scroll = 0
	}
	l.scroll = clamp(l.scroll, 0, max(0, len(l.rows)-h))
}

func (l *ListState) firstTaskID() int64 {
	for _, r := range l.rows {
		if r.Kind == RowTask {
			return r.Task.ID
		}
	}
	return 0
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
