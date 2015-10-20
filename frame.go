package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kr/beanstalk"
	"github.com/nsf/termbox-go"
)

const (
	titleLine  = "Beanstalkd Stats Monitor"
	statusLine = "[TAB] Switch Focus  [ESC] Exit  [\u2190 \u2192 \u2191 \u2193] Scroll"

	infoColor = termbox.ColorDefault
)

var hostInfo string

type mainFrame struct {
	c               *beanstalk.Conn
	statEvt         chan struct{}
	tubesStatsGrid  *ScrollableGrid
	systemStatsGrid *ScrollableGrid
	controls        []Control
	focusIndex      int
}

func (m *mainFrame) Clear(fg termbox.Attribute, bg termbox.Attribute) {
	termbox.Clear(fg, bg)
}

func (m *mainFrame) SetCell(x, y int, ch rune, fg, bg termbox.Attribute) {
	termbox.SetCell(x, y, ch, fg, bg)
}

func (m *mainFrame) WriteText(x, y int, fg, bg termbox.Attribute, s string) {
	for _, c := range s {
		termbox.SetCell(x, y, c, fg, bg)
		x++
	}
}

func (m *mainFrame) getSystemStats() [][]string {
	// list tubes
	stats, err := m.c.Stats()
	if err != nil {
		return nil
	}

	data := [][]string{}

	row := []string{}
	// get headers as reference
	for _, col := range m.systemStatsGrid.Columns {
		value, _ := stats[col.Name]
		row = append(row, value)
	}

	data = append(data, row)

	return data
}

func (m *mainFrame) getTubeStats() [][]string {
	// list tubes
	tubes, err := m.c.ListTubes()
	if err != nil {
		return nil
	}

	data := [][]string{}

	for _, tubeName := range tubes {
		tube := &beanstalk.Tube{m.c, tubeName}
		stats, err := tube.Stats()
		if err != nil {
			return nil
		}
		row := []string{}

		// get headers as reference
		for _, col := range m.tubesStatsGrid.Columns {
			value, _ := stats[col.Name]
			row = append(row, value)
		}
		data = append(data, row)
	}

	return data
}

func (m *mainFrame) pollStats(interval int) {
	m.statEvt = make(chan struct{})

	collectStats := func() {
		sysData := m.getSystemStats()
		if sysData != nil {
			m.systemStatsGrid.UpdateData(sysData)
		}

		tubeData := m.getTubeStats()
		if tubeData != nil {
			m.tubesStatsGrid.UpdateData(tubeData)
		}
	}

	collectStats()

	go func() {
		defer close(m.statEvt)

		for {
			<-time.After(time.Duration(interval) * time.Second)
			collectStats()
			m.statEvt <- struct{}{}
		}
	}()
}

func (m *mainFrame) redraw() {
	m.Clear(termbox.ColorDefault, termbox.ColorDefault)
	w, h := termbox.Size()
	m.systemStatsGrid.Resize(2, 3, w-4, 5)
	m.tubesStatsGrid.Resize(2, 10, w-4, h-11)

	m.WriteText(2, 1, infoColor, termbox.ColorDefault, titleLine)
	// write connected host info
	m.WriteText(w-len(hostInfo)-3, 1, FGColor, BGColor, hostInfo)
	m.WriteText(2, h-1, infoColor, termbox.ColorDefault, statusLine)
}

func (m *mainFrame) refresh() {
	m.redraw()
	termbox.Flush()
}

func (m *mainFrame) connect(host string, port int) error {
	c, err := beanstalk.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}
	m.c = c

	return nil
}

func (m *mainFrame) disconnect() {
	if m.c != nil {
		m.c.Close()
	}
}

func (m *mainFrame) dispatchEvent(ev termbox.Event) bool {
	for _, c := range m.controls {
		if c.Focused() && c.HandleEvent(ev) {
			c.Redraw()
			return true
		}
	}
	return false
}

func (m *mainFrame) rotateFocus() {
	for _, c := range m.controls {
		c.SetFocus(false)
	}
	m.focusIndex++
	if m.focusIndex > len(m.controls)-1 {
		m.focusIndex = 0
	}
	m.controls[m.focusIndex].SetFocus(true)
}

func (m *mainFrame) startLoop(interval int) {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()

	termbox.SetInputMode(termbox.InputEsc)

	evt := make(chan termbox.Event)
	defer close(evt)

	go func() {
		for {
			evt <- termbox.PollEvent()
		}
	}()

	m.pollStats(interval)
	m.refresh()

mainloop:
	for {
		select {
		case ev := <-evt:

			if m.dispatchEvent(ev) {
				m.refresh()
			}

			switch ev.Type {
			case termbox.EventKey:
				switch ev.Key {
				case termbox.KeyEsc:
					break mainloop

				case termbox.KeyTab:
					m.rotateFocus()
					m.refresh()

				}
			case termbox.EventError:
				panic(ev.Err)

			case termbox.EventResize:
				m.refresh()
			}

		case <-m.statEvt:
			m.refresh()
		}
	}
}

func (m *mainFrame) show(host string, port, pollInterval int) {
	// try to connect, exit on failure
	if err := m.connect(host, port); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(-1)
	}

	hostInfo = fmt.Sprintf("Connected to %s:%d", host, port)

	if m.controls == nil {
		m.focusIndex = 0
		m.controls = []Control{}

		// system stats
		m.systemStatsGrid = &ScrollableGrid{
			X:     1,
			Y:     2,
			Title: "[ System Stats ]",
			BP:    m,
			Columns: []GridColumn{
				{"hostname", AlignLeft, 20},
				{"current-jobs-urgent", AlignRight, 20},
				{"current-jobs-ready", AlignRight, 21},
				{"current-jobs-reserved", AlignRight, 23},
				{"current-jobs-delayed", AlignRight, 21},
				{"current-jobs-buried", AlignRight, 21},
				{"cmd-put", AlignRight, 9},
				{"cmd-peek", AlignRight, 10},
				{"cmd-peek-ready", AlignRight, 16},
				{"cmd-peek-delayed", AlignRight, 18},
				{"cmd-peek-buried", AlignRight, 17},
				{"cmd-reserve", AlignRight, 13},
				{"cmd-use", AlignRight, 9},
				{"cmd-watch", AlignRight, 11},
				{"cmd-ignore", AlignRight, 12},
				{"cmd-delete", AlignRight, 12},
				{"cmd-release", AlignRight, 13},
				{"cmd-bury", AlignRight, 10},
				{"cmd-kick", AlignRight, 10},
				{"cmd-stats", AlignRight, 11},
				{"cmd-stats-job", AlignRight, 15},
				{"cmd-stats-tube", AlignRight, 16},
				{"cmd-list-tubes", AlignRight, 16},
				{"cmd-list-tube-used", AlignRight, 20},
				{"cmd-list-tubes-watched", AlignRight, 24},
				{"cmd-pause-tube", AlignRight, 16},
				{"job-timeouts", AlignRight, 14},
				{"total-jobs", AlignRight, 11},
				{"max-job-size", AlignRight, 13},
				{"current-tubes", AlignRight, 14},
				{"current-connections", AlignRight, 21},
				{"current-producers", AlignRight, 19},
				{"current-workers", AlignRight, 17},
				{"current-waiting", AlignRight, 17},
				{"total-connections", AlignRight, 19},
				{"pid", AlignRight, 10},
				{"version", AlignRight, 10},
				{"rusage-utime", AlignRight, 14},
				{"rusage-stime", AlignRight, 14},
				{"uptime", AlignRight, 10},
				{"binlog-oldest-index", AlignRight, 21},
				{"binlog-current-index", AlignRight, 22},
				{"binlog-max-size", AlignRight, 17},
				{"binlog-records-written", AlignRight, 24},
				{"binlog-records-migrated", AlignRight, 25},
				{"id", AlignRight, 20},
			},
		}
		m.systemStatsGrid.SetCustomDrawFunc(func(index int, col, value string) (termbox.Attribute, termbox.Attribute) {
			if col == m.systemStatsGrid.Columns[0].Name {
				return termbox.ColorRed | termbox.AttrBold, BGColor
			}
			return FGColor, BGColor
		})
		m.systemStatsGrid.SetVisible(true)
		m.systemStatsGrid.reset()
		m.controls = append(m.controls, m.systemStatsGrid)

		m.tubesStatsGrid = &ScrollableGrid{
			X:         1,
			Y:         2,
			VScroller: true,
			Title:     "[ Tubes Stats ]",
			BP:        m,
			Columns: []GridColumn{
				{"name", AlignLeft, 20},
				{"current-jobs-urgent", AlignRight, 21},
				{"current-jobs-ready", AlignRight, 21},
				{"current-jobs-reserved", AlignRight, 21},
				{"current-jobs-delayed", AlignRight, 21},
				{"current-jobs-buried", AlignRight, 21},
				{"total-jobs", AlignRight, 12},
				{"current-using", AlignRight, 15},
				{"current-waiting", AlignRight, 17},
				{"current-watching", AlignRight, 18},
				{"pause", AlignRight, 7},
				{"cmd-delete", AlignRight, 11},
				{"cmd-pause-tube", AlignRight, 16},
				{"pause-time-left", AlignRight, 17},
			},
		}
		m.tubesStatsGrid.SetVisible(true)
		m.tubesStatsGrid.reset()
		m.controls = append(m.controls, m.tubesStatsGrid)

		m.tubesStatsGrid.SetCustomDrawFunc(func(index int, col, value string) (termbox.Attribute, termbox.Attribute) {
			fg := FGColor
			bg := BGColor

			switch col {
			case "current-jobs-delayed":
				fg = termbox.ColorYellow

			case "current-jobs-buried":
				fg = termbox.ColorRed

			case "current-jobs-ready":
				if value != "0" {
					fg = termbox.ColorGreen
				}

			}

			if index%2 == 0 {
				// striped rows
				fg |= termbox.AttrBold
			}
			return fg, bg
		})

		if len(m.controls) > 0 {
			m.controls[m.focusIndex].SetFocus(true)
		}
	}
	m.startLoop(pollInterval)
}
