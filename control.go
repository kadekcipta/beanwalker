package main

import (
	"github.com/nsf/termbox-go"
)

const (
	BGColor          = termbox.ColorDefault
	FGColor          = termbox.ColorDefault
	BGSelectionColor = termbox.ColorDefault
	FGSelectionColor = termbox.ColorBlue | termbox.AttrReverse
)

type Control interface {
	HandleEvent(ev termbox.Event) bool
	Redraw()
	Resize(x, y, w, h int)
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
