package main

import (
	"github.com/nsf/termbox-go"
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

type ControlManager interface {
	Add(c Control) int
	HandleEvent(ev termbox.Event) bool
}

type cuiControlManager struct {
	controls []Control
}

func (m *cuiControlManager) Add(c Control) int {
	if m.controls == nil {
		m.controls = []Control{}
	}
	m.controls = append(m.controls, c)
	return len(m.controls)
}

func (m *cuiControlManager) HandleEvent(ev termbox.Event) bool {
	switch ev.Type {
	case termbox.EventKey:
		switch ev.Key {
		case termbox.KeyTab:
			return true

		}
	case termbox.EventResize:
		return true
	}
	return false
}
