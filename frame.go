package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kr/beanstalk"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
)

type controlCmd struct {
	key         termbox.Key
	shortcut    string
	description string
	global      bool
	action      func()
}

const (
	titleLine            = "Beanstalkd Stats Monitor"
	connectionInfo       = "%s:%d"
	beanstalkVersionInfo = "v%s"

	infoColor = termbox.ColorDefault
)

var hostInfo string

type mainFrame struct {
	c              *beanstalk.Conn
	statEvt        chan struct{}
	tubesStatsGrid *ScrollableGrid
	sysStatsGrid   *ScrollableGrid
	controls       []Control
	focusIndex     int
	bsVersion      string
	debugText      string
	commands       []controlCmd
	done           chan struct{}
	host           string
	port           int
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
	go func() {
		<-time.After(5 * time.Second)
		m.debugText = ""
		m.refresh()
	}()
}

func (m *mainFrame) quit() { close(m.done) }

func (m *mainFrame) pauseTube() { m.debug("Not implemented yet") }

func (m *mainFrame) doClearJobs(tubeName string) {
	c, err := m.createConnection()
	if err != nil {
		return
	}
	defer c.Close()
	t := &beanstalk.Tube{c, tubeName}
	// wipe out ready jobs
	for {
		id, _, err := t.PeekReady()
		if err != nil {
			return
		}
		if err := c.Delete(id); err != nil {
			return
		}
	}
	// wipe out buried jobs
	for {
		id, _, err := t.PeekBuried()
		if err != nil {
			return
		}
		if err := c.Delete(id); err != nil {
			return
		}
	}
	// wipe out delayed jobs
	for {
		id, _, err := t.PeekDelayed()
		if err != nil {
			return
		}
		if err := c.Delete(id); err != nil {
			return
		}
	}
}

func (m *mainFrame) clearJobs() {
	if m.tubesStatsGrid.Focused() {
		row := m.tubesStatsGrid.CurrentRow()
		if row != nil {
			tubeName := row[0]
			go m.doClearJobs(tubeName)
		}
	}
}

func (m *mainFrame) deleteTube() { m.debug("Not implemented yet") }

func (m *mainFrame) execCommand(key termbox.Key) {
	for _, c := range m.commands {
		if c.key == key && c.action != nil {
			if !c.global && !m.tubesStatsGrid.Focused() {
				continue
			}
			c.action()
		}
	}
}

func (m *mainFrame) initCommands(x, y int) {
	m.commands = []controlCmd{
		{termbox.KeyCtrlQ, "CTRL+q", "Quit", true, m.quit},
		{termbox.KeyCtrlP, "CTRL+p", "Pause", false, m.pauseTube},
		{termbox.KeyCtrlC, "CTRL+c", "Clear Jobs", false, m.clearJobs},
		{termbox.KeyCtrlD, "CTRL+d", "Delete", false, m.deleteTube},
		{termbox.KeyTab, "TAB", "Navigate", true, m.navigateFocus},
		{termbox.Key(0), "\u2190 \u2192 \u2191 \u2193", "Scroll", true, nil},
	}

	dx := x
	for _, c := range m.commands {
		m.WriteText(dx, y, termbox.ColorRed, BGColor, c.shortcut)
		dx += runewidth.StringWidth(c.shortcut) + 1
		m.WriteText(dx, y, FGColor, BGColor, c.description)
		dx += runewidth.StringWidth(c.description) + 2
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
	for _, col := range m.sysStatsGrid.Columns {
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
			m.sysStatsGrid.UpdateData(sysData)
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
	m.sysStatsGrid.Resize(BufferRegion{1, 2, w - 3, 5})
	m.tubesStatsGrid.Resize(BufferRegion{1, 8, w - 3, h - 10})

	m.WriteText(1, 1, infoColor, termbox.ColorDefault, titleLine)
	beanstalkInfo := fmt.Sprintf(beanstalkVersionInfo, m.bsVersion)
	m.WriteText(w-runewidth.StringWidth(hostInfo)-1, 0, FGColor, BGColor, hostInfo)
	m.WriteText(w-runewidth.StringWidth(beanstalkInfo)-1, 1, FGColor|termbox.AttrBold, BGColor, beanstalkInfo)
	m.initCommands(2, h-2)
	cx := (w - runewidth.StringWidth(m.debugText)) / 2
	m.WriteText(cx, h-1, termbox.ColorYellow, BGColor, m.debugText)
}

func (m *mainFrame) refresh() {
	m.redraw()
	termbox.Flush()
}

func (m *mainFrame) createConnection() (*beanstalk.Conn, error) {
	c, err := beanstalk.Dial("tcp", fmt.Sprintf("%s:%d", m.host, m.port))
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (m *mainFrame) connect() error {
	c, err := m.createConnection()
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
	m.refresh()
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
		case <-m.done:
			return

		case ev := <-evt:
			if m.dispatchEvent(ev) {
				m.refresh()
				continue
			}

			switch ev.Type {
			case termbox.EventKey:
				if ev.Key >= termbox.KeyCtrlA && ev.Key <= termbox.KeyCtrlZ {
					m.execCommand(ev.Key)
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
	m.host = host
	m.port = port
	// try to connect, exit on failure
	if err := m.connect(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(-1)
	}

	hostInfo = fmt.Sprintf(connectionInfo, host, port)
	m.done = make(chan struct{})

	if m.controls == nil {
		m.focusIndex = 0
		m.controls = []Control{}

		// system stats
		m.sysStatsGrid = &ScrollableGrid{
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
				{"cmd-stats-job", AlignRight, 15},
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
		m.sysStatsGrid.SetCustomDrawFunc(func(index int, col, value string) (termbox.Attribute, termbox.Attribute) {
			if col == m.sysStatsGrid.Columns[0].Name {
				return termbox.ColorRed, BGColor
			}
			return FGColor, BGColor
		})
		m.sysStatsGrid.SetVisible(true)
		m.sysStatsGrid.reset()
		m.controls = append(m.controls, m.sysStatsGrid)

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
		if len(m.controls) > 0 {
			m.controls[m.focusIndex].SetFocus(true)
		}
	}

	m.startLoop(pollInterval)
}
