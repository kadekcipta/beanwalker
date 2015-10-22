package main

import (
	"github.com/nsf/termbox-go"
)

const (
	BGColor          = termbox.ColorDefault
	FGColor          = termbox.ColorDefault
	BGSelectionColor = termbox.ColorRed
	FGSelectionColor = termbox.ColorWhite
)

type BufferRegion struct {
	X, Y, W, H int
}

func (b BufferRegion) Valid() bool {
	return b.H > 0 && b.W > 0
}

type Control interface {
	HandleEvent(ev termbox.Event) bool
	Redraw()
	Resize(BufferRegion)
	SetFocus(v bool)
	Focused() bool
	SetVisible(b bool)
	Visible() bool
}

type BufferProxy interface {
	Clear(termbox.Attribute, termbox.Attribute)
	SetCell(x, y int, ch rune, fg, bg termbox.Attribute)
	WriteText(x, y int, fg, bg termbox.Attribute, s string)
}
