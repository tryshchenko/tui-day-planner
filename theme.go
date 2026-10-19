package main

// Tokyo-Night-inspired palette.
var (
	cBg     = RGB(0x16161e) // screen background
	cPanel  = RGB(0x1a1b26) // pane background
	cModal  = RGB(0x24283b) // overlays
	cSel    = RGB(0x2b3050) // selected row, focused pane
	cSelDim = RGB(0x232639) // selected row, unfocused pane
	cBorder = RGB(0x3b4261)
	cText   = RGB(0xc0caf5)
	cDim    = RGB(0x7982a9)
	cFaint  = RGB(0x4a5173)
	cAccent = RGB(0x7aa2f7)
	cViolet = RGB(0xbb9af7)
	cGreen  = RGB(0x9ece6a)
	cYellow = RGB(0xe0af68)
	cOrange = RGB(0xff9e64)
	cRed    = RGB(0xf7768e)
	cCyan   = RGB(0x7dcfff)
)

func RGB(hex uint32) Color { return Color(hex) }

// Section colors age from cool to warm.
var secColor = [3]Color{cAccent, cYellow, cOrange}
