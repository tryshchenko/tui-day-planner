package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T, now time.Time, w, h int) *App {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := NewApp(db, w, h)
	a.now = func() time.Time { return now }
	a.reload()
	return a
}

func seed(t *testing.T, a *App, title string, at time.Time, doneAt *time.Time) int64 {
	t.Helper()
	id, err := a.db.Add(title, at)
	if err != nil {
		t.Fatal(err)
	}
	if doneAt != nil {
		a.db.SetDone(id, doneAt)
	}
	return id
}

func key(a *App, specs ...string) {
	for _, s := range specs {
		a.handle(kev(s))
	}
}

func (a *App) frame() string { a.render(); return a.scr.Dump() }

// Monday 2026-10-19 10:00
var monday = time.Date(2026, 10, 19, 10, 0, 0, 0, time.Local)

func TestSections(t *testing.T) {
	a := testApp(t, monday, 100, 30)
	seed(t, a, "today task", monday, nil)
	seed(t, a, "sunday task", monday.AddDate(0, 0, -1), nil)
	seed(t, a, "friday task", monday.AddDate(0, 0, -3), nil)
	seed(t, a, "thursday task", monday.AddDate(0, 0, -4), nil)
	a.reload()
	var labels []string
	for _, r := range a.main.rows {
		switch r.Kind {
		case RowHeader:
			labels = append(labels, r.Label)
		case RowTask:
			labels = append(labels, r.Task.Title)
		}
	}
	want := "TODAY,today task,YESTERDAY,sunday task,friday task,OLD,thursday task"
	if got := strings.Join(labels, ","); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	// Tuesday: yesterday is only Monday
	a.now = func() time.Time { return monday.AddDate(0, 0, 1) }
	a.reload()
	for _, r := range a.main.rows {
		if r.Kind == RowTask && r.Task.Title == "friday task" && r.Sec != SecOld {
			t.Fatalf("friday should be Old on Tuesday")
		}
	}
}

func TestCrossedLifecycle(t *testing.T) {
	a := testApp(t, monday, 100, 30)
	yesterdayDone := monday.AddDate(0, 0, -1)
	seed(t, a, "finished yesterday", monday.AddDate(0, 0, -2), &yesterdayDone)
	seed(t, a, "open", monday, nil)
	a.reload()
	if len(a.tasks) != 1 || a.tasks[0].Title != "open" {
		t.Fatalf("crossed task from yesterday must be hidden, got %d tasks", len(a.tasks))
	}
	key(a, " ") // cross out selected ("open")
	if !a.currentTask().Done() || len(a.tasks) != 1 {
		t.Fatal("crossed task should stay visible today")
	}
	// next day it disappears
	a.now = func() time.Time { return monday.AddDate(0, 0, 1) }
	a.reload()
	if len(a.tasks) != 0 {
		t.Fatalf("crossed task should vanish next day, got %d", len(a.tasks))
	}
	// Completed page lists both; unchecking restores to today
	key(a, "c")
	if len(a.done) != 2 {
		t.Fatalf("want 2 completed, got %d", len(a.done))
	}
	key(a, " ")
	key(a, "c")
	if len(a.tasks) != 1 {
		t.Fatalf("restored task should be back, got %d", len(a.tasks))
	}
	if a.sectionOf(a.tasks[0].Day) != SecToday {
		t.Fatal("restored task must land in Today")
	}
}

func TestRenderWide(t *testing.T) {
	a := testApp(t, monday, 110, 28)
	id := seed(t, a, "Write the quarterly report for the whole team and send it out", monday, nil)
	a.db.UpdateDetails(id, "First line of notes.\nSecond line with a longer sentence that should wrap around the pane nicely when it is long enough.")
	seed(t, a, "Buy milk", monday, nil)
	d := monday.Add(-time.Hour)
	seed(t, a, "Already done", monday, &d)
	seed(t, a, "Sunday leftover", monday.AddDate(0, 0, -1), nil)
	seed(t, a, "Old one 🙂 日本語", monday.AddDate(0, 0, -9), nil)
	a.reload()
	fmt.Println(a.frame())
	key(a, "c")
	fmt.Println(a.frame())
	key(a, "c", "?")
	fmt.Println(a.frame())
}

func TestRenderNarrow(t *testing.T) {
	a := testApp(t, monday, 40, 14)
	seed(t, a, "Buy milk", monday, nil)
	seed(t, a, "A very long task title that cannot possibly fit in this tiny window", monday, nil)
	a.reload()
	fmt.Println(a.frame())
	key(a, "p")
	fmt.Println(a.frame())
	key(a, "n")
	fmt.Println(a.frame())
}

func TestEditDetails(t *testing.T) {
	a := testApp(t, monday, 100, 24)
	seed(t, a, "task", monday, nil)
	a.reload()
	key(a, "tab")
	if a.focus != FocusDetail {
		t.Fatal("tab should focus details")
	}
	for _, r := range "hello" {
		key(a, string(r))
	}
	key(a, "enter", "w", "o")
	key(a, "esc")
	if a.focus != FocusList {
		t.Fatal("esc should return to list")
	}
	got, _ := a.db.Active(a.todayT)
	if got[0].Details != "hello\nwo" {
		t.Fatalf("details not saved: %q", got[0].Details)
	}
}

func TestParser(t *testing.T) {
	evs, rest := parseInput([]byte("a\x1b[A\x1b[<0;10;5M\x1b[<64;3;4M\x1b[Z\x1b[1;5D\x1b"))
	if len(rest) != 0 {
		t.Fatalf("rest %q", rest)
	}
	var got []string
	for _, e := range evs {
		switch e.Type {
		case EvKey:
			got = append(got, e.Key)
		case EvMouse:
			got = append(got, fmt.Sprintf("m%d@%d,%d", e.Btn, e.X, e.Y))
		}
	}
	want := "rune,up,m0@9,4,m64@2,3,shift+tab,ctrl+left,esc"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v", got)
	}
	evs, rest = parseInput([]byte("\x1b[200~line1\nline2\x1b[201~"))
	if len(evs) != 1 || evs[0].Text != "line1\nline2" {
		t.Fatalf("paste: %+v", evs)
	}
	_, rest = parseInput([]byte("\x1b[20"))
	if len(rest) == 0 {
		t.Fatal("incomplete CSI should be kept")
	}
}

func TestTextAreaWrapNav(t *testing.T) {
	ta := NewTextArea(false)
	ta.SetText("hello world this is a test\nsecond")
	ta.gw = 12
	ta.row, ta.col = 0, 0
	ta.HandleKey(kev("down"))
	if ta.row != 0 || ta.col == 0 {
		t.Fatalf("down should move to next visual line, row=%d col=%d", ta.row, ta.col)
	}
	ta.HandleKey(kev("up"))
	if ta.col != 0 {
		t.Fatalf("up col=%d", ta.col)
	}
	ta.row, ta.col = 1, 3
	ta.HandleKey(kev("backspace"))
	if ta.Text() != "hello world this is a test\nseond" {
		t.Fatalf("%q", ta.Text())
	}
}

func TestMouseToggleAndSelect(t *testing.T) {
	a := testApp(t, monday, 100, 24)
	seed(t, a, "one", monday, nil)
	seed(t, a, "two", monday, nil)
	a.reload()
	a.render()
	// find the row of "two" in the dump
	lines := strings.Split(a.scr.Dump(), "\n")
	y := -1
	for i, l := range lines {
		if strings.Contains(l, "two") {
			y = i
		}
	}
	if y < 0 {
		t.Fatal("row not found")
	}
	a.handle(Event{Type: EvMouse, X: 12, Y: y, Btn: 0, Press: true})
	if a.currentTask().Title != "two" {
		t.Fatalf("click should select, got %s", a.currentTask().Title)
	}
	a.render()
	a.handle(Event{Type: EvMouse, X: 3, Y: y, Btn: 0, Press: true})
	if !a.currentTask().Done() {
		t.Fatal("click on checkbox should toggle")
	}
}
