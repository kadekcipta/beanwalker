package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kr/beanstalk"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
)

type cmd struct {
	sc     string
	info   string
	action func()
}

const (
	titleLine            = "Beanstalkd Stats Monitor"
	connectionInfo       = "Host: %s:%d"
	beanstalkVersionInfo = "Server version: %s"

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
	bsVersion       string
	debugText       string
	commands        []cmd
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

func (m *mainFrame) debug(s string) {
	m.debugText = s
	m.refresh()
}

func (m *mainFrame) quit()       { m.debug("") }
func (m *mainFrame) pauseTube()  { m.debug("pause") }
func (m *mainFrame) clearJobs()  { m.debug("clear jobs") }
func (m *mainFrame) deleteTube() { m.debug("delete tube") }

func (m *mainFrame) execCommand(sc string) {
	if !m.tubesStatsGrid.Focused() {
		return
	}
	for _, c := range m.commands {
		if c.sc == sc && c.action != nil {
			c.action()
		}
	}
}

func (m *mainFrame) initCommands(x, y int) {
	m.commands = []cmd{
		{"q", "Quit", m.quit},
		{"p", "Pause", m.pauseTube},
		{"c", "Clear Jobs", m.clearJobs},
		{"d", "Delete", m.deleteTube},
		{"TAB", "Navigate", nil},
		{"[\u2190 \u2192 \u2191 \u2193]", "Scroll", nil},
	}

	dx := x
	for _, c := range m.commands {
		m.WriteText(dx, y, termbox.ColorRed, BGColor, c.sc)
		dx += runewidth.StringWidth(c.sc) + 1
		m.WriteText(dx, y, FGColor, BGColor, c.info)
		dx += runewidth.StringWidth(c.info) + 2
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
	m.systemStatsGrid.Resize(BufferRegion{1, 2, w - 3, 5})
	m.tubesStatsGrid.Resize(BufferRegion{1, 8, w - 3, h - 10})

	m.WriteText(1, 0, infoColor, termbox.ColorDefault, titleLine)
	beanstalkInfo := fmt.Sprintf(beanstalkVersionInfo, m.bsVersion)
	m.WriteText(w-runewidth.StringWidth(hostInfo)-1, 0, FGColor, BGColor, hostInfo)
	m.WriteText(w-runewidth.StringWidth(beanstalkInfo)-1, 1, FGColor|termbox.AttrBold, BGColor, beanstalkInfo)
	m.initCommands(2, h-2)
	m.WriteText(w-runewidth.StringWidth(m.debugText), h-1, FGColor, BGColor, m.debugText)
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
	// get server version
	stats, err := c.Stats()
	if err != nil {
		return err
	}

	version, ok := stats["version"]
	if ok {
		m.bsVersion = version
	}

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

func (m *mainFrame) navigateFocus() {
	// list of visible controls
	visibles := []Control{}
	for _, c := range m.controls {
		c.SetFocus(false)
		if c.Visible() {
			visibles = append(visibles, c)
		}
	}
	m.focusIndex++
	if m.focusIndex > len(visibles)-1 {
		m.focusIndex = 0
	}
	m.controls[m.focusIndex].SetFocus(true)
}

func (m *mainFrame) startLoop(interval int) {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}

	defer func() {
		termbox.Close()
		m.disconnect()
	}()

	termbox.SetInputMode(termbox.InputEsc)
	evt := make(chan termbox.Event)

	go func() {
		defer close(evt)
		for {
			evt <- termbox.PollEvent()
		}
	}()

	m.pollStats(interval)
	m.refresh()

	for {
		select {
		case ev := <-evt:
			if m.dispatchEvent(ev) {
				m.refresh()
				continue
			}

			switch ev.Type {
			case termbox.EventKey:
				switch ev.Key {
				case termbox.KeyTab:
					m.navigateFocus()
					m.refresh()

				default:
					if ev.Ch == 'q' {
						return
					}
					m.execCommand(string(ev.Ch))
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

	hostInfo = fmt.Sprintf(connectionInfo, host, port)

	if m.controls == nil {
		m.focusIndex = 0
		m.controls = []Control{}

		// system stats
		m.systemStatsGrid = &ScrollableGrid{
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
		/*
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
		*/
		if len(m.controls) > 0 {
			m.controls[m.focusIndex].SetFocus(true)
		}
		/*
			m.tubesStatsGrid.UpdateData([][]string{
				[]string{"tube 1", "120", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 2", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 3", "0", "230", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "330"},
				[]string{"tube 4", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "111", "0"},
				[]string{"tube 5", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 6", "120", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 7", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 8", "0", "230", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "330"},
				[]string{"tube 9", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "111", "0"},
				[]string{"tube 10", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 11", "120", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 12", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 13", "0", "230", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "330"},
				[]string{"tube 14", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "111", "0"},
				[]string{"tube 15", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 16", "120", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 17", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 18", "0", "230", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "330"},
				[]string{"tube 19", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "111", "0"},
				[]string{"tube 20", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 21", "120", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 22", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 23", "0", "230", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "330"},
				[]string{"tube 24", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "111", "0"},
				[]string{"tube 25", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 26", "120", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 27", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 28", "0", "230", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "330"},
				[]string{"tube 29", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "111", "0"},
				[]string{"tube 30", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 31", "120", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 32", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 33", "0", "230", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "330"},
				[]string{"tube 34", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "111", "0"},
				[]string{"tube 35", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 36", "120", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 37", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
				[]string{"tube 38", "0", "230", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "330"},
				[]string{"tube 39", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "111", "0"},
				[]string{"tube 40", "0", "0", "1", "0", "1", "1", "0", "0", "1", "0", "1", "1", "0"},
			})
		*/
	}

	m.startLoop(pollInterval)
}
