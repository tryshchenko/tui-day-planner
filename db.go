package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: no cgo, single static binary
)

const dayFmt = "2006-01-02"

// Task is one note in the tracker.
type Task struct {
	ID        int64
	Title     string
	Details   string
	CreatedAt time.Time
	Day       string    // list-day (YYYY-MM-DD): which day section the task belongs to
	DoneAt    time.Time // zero when open
	Pos       int64

	// UI-only state, never persisted.
	Restored bool   // un-crossed while the Completed page is open
	Group    string // Completed page grouping key (day the task was crossed out)
}

func (t *Task) Done() bool { return !t.DoneAt.IsZero() && !t.Restored }

type DB struct{ sql *sql.DB }

// DefaultPath returns ~/.day-planner.db unless DAY_PLANNER_DB overrides it.
func DefaultPath() (string, error) {
	if p := os.Getenv("DAY_PLANNER_DB"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".day-planner.db"), nil
}

func OpenDB(path string) (*DB, error) {
	esc := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(path)
	dsn := "file:" + esc + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One stable connection: PRAGMA data_version then reports only *other* processes' commits.
	sdb.SetMaxOpenConns(1)
	sdb.SetMaxIdleConns(1)
	sdb.SetConnMaxLifetime(0)
	_, err = sdb.Exec(`
CREATE TABLE IF NOT EXISTS tasks (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	title      TEXT    NOT NULL,
	details    TEXT    NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	day        TEXT    NOT NULL,
	done_at    INTEGER,
	pos        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS tasks_done_at ON tasks(done_at);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);`)
	if err != nil {
		sdb.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &DB{sql: sdb}, nil
}

func (d *DB) Close() error { return d.sql.Close() }

const taskCols = `id, title, details, created_at, day, COALESCE(done_at, 0), pos`

func scanTasks(rows *sql.Rows) ([]*Task, error) {
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		var t Task
		var created, done int64
		if err := rows.Scan(&t.ID, &t.Title, &t.Details, &created, &t.Day, &done, &t.Pos); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(created, 0)
		if done != 0 {
			t.DoneAt = time.Unix(done, 0)
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// Active returns open tasks plus tasks crossed out since startOfToday.
func (d *DB) Active(startOfToday time.Time) ([]*Task, error) {
	rows, err := d.sql.Query(`SELECT `+taskCols+` FROM tasks
		WHERE done_at IS NULL OR done_at >= ?
		ORDER BY day DESC, pos ASC`, startOfToday.Unix())
	if err != nil {
		return nil, err
	}
	return scanTasks(rows)
}

// Completed returns every crossed-out task, newest first.
func (d *DB) Completed() ([]*Task, error) {
	rows, err := d.sql.Query(`SELECT ` + taskCols + ` FROM tasks
		WHERE done_at IS NOT NULL ORDER BY done_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	return scanTasks(rows)
}

func (d *DB) Add(title string, now time.Time) (int64, error) {
	res, err := d.sql.Exec(`INSERT INTO tasks(title, created_at, day, pos) VALUES(?,?,?,?)`,
		title, now.Unix(), now.Format(dayFmt), now.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateTitle(id int64, title string) error {
	_, err := d.sql.Exec(`UPDATE tasks SET title=? WHERE id=?`, title, id)
	return err
}

func (d *DB) UpdateDetails(id int64, details string) error {
	_, err := d.sql.Exec(`UPDATE tasks SET details=? WHERE id=?`, details, id)
	return err
}

// SetDone crosses a task out (at != nil) or un-crosses it (at == nil).
func (d *DB) SetDone(id int64, at *time.Time) error {
	if at == nil {
		_, err := d.sql.Exec(`UPDATE tasks SET done_at=NULL WHERE id=?`, id)
		return err
	}
	_, err := d.sql.Exec(`UPDATE tasks SET done_at=? WHERE id=?`, at.Unix(), id)
	return err
}

// Restore un-crosses a task and moves it to today.
func (d *DB) Restore(id int64, now time.Time) error {
	_, err := d.sql.Exec(`UPDATE tasks SET done_at=NULL, day=?, pos=? WHERE id=?`,
		now.Format(dayFmt), now.UnixNano(), id)
	return err
}

func (d *DB) MoveToToday(id int64, now time.Time) error {
	_, err := d.sql.Exec(`UPDATE tasks SET day=?, pos=? WHERE id=?`,
		now.Format(dayFmt), now.UnixNano(), id)
	return err
}

func (d *DB) Delete(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}

func (d *DB) Setting(key, def string) string {
	var v string
	if err := d.sql.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.sql.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// DataVersion changes whenever another connection/process commits.
func (d *DB) DataVersion() (int64, error) {
	var v int64
	err := d.sql.QueryRow(`PRAGMA data_version`).Scan(&v)
	return v, err
}
