package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestScreenshots renders README screenshots as SVG into docs/screenshots.
// It only runs when asked:  UPDATE_SCREENSHOTS=1 go test -run TestScreenshots
func TestScreenshots(t *testing.T) {
	if os.Getenv("UPDATE_SCREENSHOTS") == "" {
		t.Skip("set UPDATE_SCREENSHOTS=1 to regenerate docs/screenshots")
	}
	dir := filepath.Join("docs", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	shot := func(name string, a *App) {
		a.render()
		if err := os.WriteFile(filepath.Join(dir, name+".svg"), []byte(svgScreen(a.scr)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := demoApp(t)
	shot("main", a)

	key(a, "n")
	a.input.ta.Insert("Call the bank about the new card")
	shot("new-task", a)
	key(a, "esc", "c")
	shot("completed", a)
}

func demoApp(t *testing.T) *App {
	now := time.Date(2026, 10, 19, 14, 30, 0, 0, time.Local) // a Monday
	a := testApp(t, now, 112, 30)
	at := func(daysAgo, h, m int) time.Time {
		d := now.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, time.Local)
	}
	done := func(daysAgo, h, m int) *time.Time { x := at(daysAgo, h, m); return &x }

	// already archived: shows up on the Completed page
	seed(t, a, "Draft Q4 planning doc", at(5, 9, 10), done(4, 16, 45))
	seed(t, a, "Update Go toolchain to 1.26", at(4, 11, 0), done(4, 11, 20))
	seed(t, a, "Water the plants", at(3, 8, 0), done(2, 10, 5))
	seed(t, a, "Groceries for the weekend", at(3, 8, 5), done(3, 18, 30))

	seed(t, a, "Read “Designing Data-Intensive Applications” ch. 5", at(7, 21, 0), nil)
	seed(t, a, "Renew passport", at(5, 9, 0), nil)
	seed(t, a, "Fix flaky CI job on arm64", at(3, 10, 15), nil)
	seed(t, a, "Plan the weekend hike", at(2, 19, 40), nil)

	seed(t, a, "Coffee beans", at(0, 8, 30), done(0, 9, 5))
	seed(t, a, "Review PR #142 — SQLite WAL migration", at(0, 9, 0), done(0, 11, 40))
	notes := seed(t, a, "Write release notes for v0.3", at(0, 9, 15), nil)
	seed(t, a, "Book dentist appointment", at(0, 10, 0), nil)
	seed(t, a, "Prepare the offsite agenda", at(0, 12, 20), nil)
	a.db.UpdateDetails(notes, strings.Join([]string{
		"Highlights:",
		"- Completed page, grouped by the day tasks were crossed out",
		"- Mouse support: click, double-click for details, wheel",
		"- On Mondays, Yesterday covers Fri – Sun",
		"- Notes autosave as you type",
		"",
		"Still to do: screenshots for the README",
	}, "\n"))

	a.reload()
	a.main.selID = notes
	a.syncDetail()
	return a
}

// ---- SVG export ----

const (
	svgCW   = 8.4  // cell width
	svgCH   = 19.0 // cell height
	svgFont = 14.0
	svgPad  = 18.0
	svgBar  = 30.0 // window title bar
)

func hexColor(c Color) string { return fmt.Sprintf("#%06x", int32(c)&0xffffff) }

func svgEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// svgScreen draws the cell grid inside a minimal terminal window frame.
func svgScreen(s *Screen) string {
	var b strings.Builder
	gw, gh := float64(s.W)*svgCW, float64(s.H)*svgCH
	W, H := gw+2*svgPad, gh+svgBar+svgPad
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g">`+"\n", W, H, W, H)
	fmt.Fprintf(&b, `<rect width="%g" height="%g" rx="10" fill="%s" stroke="%s"/>`+"\n", W, H, hexColor(cBg), hexColor(cBorder))
	for i, c := range []string{"#ff5f57", "#febc2e", "#28c840"} {
		fmt.Fprintf(&b, `<circle cx="%g" cy="%g" r="6" fill="%s"/>`+"\n", svgPad+float64(i)*20, svgBar/2+2, c)
	}
	fmt.Fprintf(&b, `<g transform="translate(%g %g)" font-family="Menlo, 'SF Mono', Consolas, 'DejaVu Sans Mono', monospace" font-size="%g">`+"\n", svgPad, svgBar, svgFont)

	cell := func(x, y int) Cell { return s.cells[y*s.W+x] }
	bgOf := func(c Cell) Color {
		if c.S.BG < 0 {
			return cBg
		}
		return c.S.BG
	}
	// backgrounds, merged into horizontal runs
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; {
			bg := bgOf(cell(x, y))
			n := 1
			for x+n < s.W && bgOf(cell(x+n, y)) == bg {
				n++
			}
			if bg != cBg {
				fmt.Fprintf(&b, `<rect x="%g" y="%g" width="%g" height="%g" fill="%s"/>`+"\n",
					float64(x)*svgCW, float64(y)*svgCH, float64(n)*svgCW+0.5, svgCH+0.5, hexColor(bg))
			}
			x += n
		}
	}
	// glyphs: box drawing as geometry so lines join across cells, the rest as text runs
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; {
			c := cell(x, y)
			if c.W == 0 || c.R == ' ' {
				x++
				continue
			}
			fg := c.S.FG
			if fg < 0 {
				fg = cText
			}
			px, py := float64(x)*svgCW, float64(y)*svgCH
			if svgBox(&b, c.R, px, py, hexColor(fg)) {
				x++
				continue
			}
			var run []rune
			cols := 0
			for x+cols < s.W {
				d := cell(x+cols, y)
				if d.W == 0 {
					cols++
					continue
				}
				if d.S != c.S || boxRune(d.R) {
					break
				}
				run = append(run, d.R)
				cols += int(d.W)
			}
			text := strings.TrimRight(string(run), " ")
			tw := float64(strWidth(text)) * svgCW
			attrs := ""
			if c.S.A&Bold != 0 {
				attrs += ` font-weight="bold"`
			}
			if c.S.A&Italic != 0 {
				attrs += ` font-style="italic"`
			}
			fmt.Fprintf(&b, `<text x="%g" y="%g" fill="%s" textLength="%g" lengthAdjust="spacingAndGlyphs" xml:space="preserve"%s>%s</text>`+"\n",
				px, py+svgCH*0.73, hexColor(fg), tw, attrs, svgEscape(text))
			if c.S.A&Strike != 0 {
				fmt.Fprintf(&b, `<rect x="%g" y="%g" width="%g" height="1" fill="%s"/>`+"\n", px, py+svgCH*0.5, tw, hexColor(fg))
			}
			x += cols
		}
	}
	if s.curVisible {
		fmt.Fprintf(&b, `<rect x="%g" y="%g" width="2" height="%g" fill="%s"/>`+"\n",
			float64(s.curX)*svgCW, float64(s.curY)*svgCH+2, svgCH-4, hexColor(cText))
	}
	b.WriteString("</g>\n</svg>\n")
	return b.String()
}

func boxRune(r rune) bool { return strings.ContainsRune("─│╭╮╰╯▌┃", r) }

func svgBox(b *strings.Builder, r rune, x, y float64, fill string) bool {
	mx, my := x+svgCW/2, y+svgCH/2
	line := func(d string) {
		fmt.Fprintf(b, `<path d="%s" stroke="%s" stroke-width="1.2" fill="none"/>`+"\n", d, fill)
	}
	const rad = 5.0
	switch r {
	case '─':
		line(fmt.Sprintf("M%g %gH%g", x, my, x+svgCW))
	case '│':
		line(fmt.Sprintf("M%g %gV%g", mx, y, y+svgCH))
	case '╭':
		line(fmt.Sprintf("M%g %gV%gA%g %g 0 0 1 %g %gH%g", mx, y+svgCH, my+rad, rad, rad, mx+rad, my, x+svgCW))
	case '╮':
		line(fmt.Sprintf("M%g %gH%gA%g %g 0 0 1 %g %gV%g", x, my, mx-rad, rad, rad, mx, my+rad, y+svgCH))
	case '╰':
		line(fmt.Sprintf("M%g %gV%gA%g %g 0 0 0 %g %gH%g", mx, y, my-rad, rad, rad, mx+rad, my, x+svgCW))
	case '╯':
		line(fmt.Sprintf("M%g %gH%gA%g %g 0 0 0 %g %gV%g", x, my, mx-rad, rad, rad, mx, my-rad, y))
	case '▌':
		fmt.Fprintf(b, `<rect x="%g" y="%g" width="%g" height="%g" fill="%s"/>`+"\n", x, y, svgCW/2, svgCH, fill)
	case '┃':
		fmt.Fprintf(b, `<rect x="%g" y="%g" width="3" height="%g" fill="%s"/>`+"\n", mx-1.5, y, svgCH, fill)
	default:
		return false
	}
	return true
}
