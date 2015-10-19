package main

import (
	"github.com/nsf/termbox-go"
)

const (
	BGColor = termbox.ColorDefault
	FGColor = termbox.ColorDefault
)

type Control interface {
	HandleEvent(ev termbox.Event) bool
	Refresh()
	Resize(x, y, w, h int)
	SetFocus(v bool)
	Focused() bool
	SetVisible(b bool)
	Visible() bool
}
