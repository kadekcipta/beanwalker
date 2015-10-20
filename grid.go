package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/nsf/termbox-go"
)

type TextAlign int

const (
	AlignLeft TextAlign = iota
	AlignRight

	titleOffset   = -1
	columnsOffset = 1
	vHintsOffset  = 2
	dataOffset    = 3
)

// CustomDrawFunc provides callback to draw a value
// Arguments are row index, column name and value and returns foreground and background attributes
type CustomDrawFunc func(int, string, string) (termbox.Attribute, termbox.Attribute)

type GridColumn struct {
	Name  string
	Align TextAlign
	Width int
}

// Format returns the formatted value for this column information
func (c *GridColumn) Format(s string, vars ...bool) string {
	titleFormat := false
	if len(vars) > 0 {
		titleFormat = vars[0]
	}
	format := strconv.Itoa(c.Width) + "s"
	if c.Align == AlignLeft {
		format = "-" + format
	}
	format = "%" + format

	if titleFormat {
		return strings.Title(fmt.Sprintf(format, s))
	}
	return fmt.Sprintf(format, s)
}

// ScrollableGrid represents the interface to arrange string data in tabular format
// Vertical and horizontal scrolling are provided by arrows keys
type ScrollableGrid struct {
	sync.RWMutex
	X              int
	Y              int
	Width          int
	Height         int
	Columns        []GridColumn
	VScroller      bool
	Title          string
	BP             BufferProxy
	visible        bool
	focused        bool
	dataStartY     int
	hScrollPos     int
	vScrollPos     int
	data           [][]string
	customDrawFunc CustomDrawFunc
}

func (s *ScrollableGrid) SetCustomDrawFunc(f CustomDrawFunc) {
	s.customDrawFunc = f
}

func (s *ScrollableGrid) clearRow(y int, vars ...termbox.Attribute) {
	bg := termbox.ColorDefault
	if len(vars) > 0 {
		bg = vars[0]
	}
	for i := s.X; i < s.X+s.Width-1; i++ {
		s.BP.SetCell(i, y, ' ', termbox.ColorDefault, bg)
	}
}

func (s *ScrollableGrid) drawRow(cellY int, row []string, fg, bg termbox.Attribute, customizable bool, vars ...bool) {
	titleFormat := false
	if len(vars) > 0 {
		titleFormat = true
	}

	totalLen := 0
	dx := s.X

	s.clearRow(cellY, bg)

	// always draw the first heading
	for i := 0; i < len(row); i++ {
		if i == 0 || i >= s.hScrollPos {
			item := s.Columns[i].Format(row[i], titleFormat)
			// text length
			tw := len(item)
			totalLen += tw
			if totalLen > s.Width {
				break
			}

			if customizable && s.customDrawFunc != nil {
				dataIndex := cellY - s.dataStartY
				fg, bg := s.customDrawFunc(dataIndex, s.Columns[i].Name, row[i])
				s.BP.WriteText(dx, cellY, fg, bg, item)
			} else {
				s.BP.WriteText(dx, cellY, fg, bg, item)
			}

			dx += tw
		}
	}
}

func (s *ScrollableGrid) drawBox() {
	fg := FGColor
	bg := BGColor
	if s.Focused() {
		fg |= termbox.AttrBold
	}

	s.BP.SetCell(s.X-1, s.Y-1, '┌', fg, bg)
	s.BP.SetCell(s.X+s.Width-1, s.Y-1, '┐', fg, bg)
	for i := 0; i < s.Width-1; i++ {
		s.BP.SetCell(s.X+i, s.Y-1, '─', fg, bg)
		s.BP.SetCell(s.X+i, s.Y-1+s.Height, '─', fg, bg)
	}

	for y := 0; y < s.Height-1; y++ {
		s.BP.SetCell(s.X-1, s.Y+y, '│', fg, bg)
		s.BP.SetCell(s.X+s.Width-1, s.Y+y, '│', fg, bg)
	}

	s.BP.SetCell(s.X-1, s.Y-1+s.Height, '└', fg, bg)
	s.BP.SetCell(s.X+s.Width-1, s.Y-1+s.Height, '┘', fg, bg)
}

func (s *ScrollableGrid) drawHints() {
	if !s.Focused() {
		return
	}

	s.BP.WriteText(s.X-2, s.Y+columnsOffset, FGColor|termbox.AttrBold, BGColor, "\u2190")
	s.BP.WriteText(s.X+s.Width, s.Y+columnsOffset, FGColor|termbox.AttrBold, BGColor, "\u2192")

	if s.VScroller {
		cx := (s.X + s.Width) / 2
		s.BP.WriteText(cx, s.Y+vHintsOffset, FGColor|termbox.AttrBold, BGColor, "\u2191")
		s.BP.WriteText(cx, s.Y+s.Height-1, FGColor|termbox.AttrBold, BGColor, "\u2193")
	}
}

func (s *ScrollableGrid) drawColumns() {
	headers := []string{}
	for _, c := range s.Columns {
		headers = append(headers, c.Name)
	}
	highlightFGColor := FGColor
	highlightBGColor := FGColor | termbox.AttrReverse
	if !s.Focused() {
		highlightBGColor = BGColor
		highlightFGColor |= termbox.AttrBold
	}
	s.drawRow(s.Y+columnsOffset, headers, highlightFGColor, highlightBGColor, false, true)
}

func (s *ScrollableGrid) drawData() {

	maxRows := s.availableRowsSpace()
	dataIndex := 0

	for i := s.vScrollPos; i < len(s.data); i++ {
		row := s.data[i]
		if dataIndex < maxRows {
			s.drawRow(s.Y+dataOffset+dataIndex, row, FGColor, BGColor, true)
		}
		dataIndex++
	}
}

func (s *ScrollableGrid) drawTitle() {
	cx := s.X + (s.Width-len(s.Title))/2
	s.BP.WriteText(cx, s.Y+titleOffset, FGColor, FGColor|termbox.AttrReverse, strings.ToUpper(s.Title))
}

func (s *ScrollableGrid) drawBuffer() {
	s.Lock()
	defer s.Unlock()

	s.drawColumns()
	s.drawHints()
	s.drawData()
	s.drawBox()
	s.drawTitle()
}

func (s *ScrollableGrid) availableRowsSpace() int {
	return s.Height - dataOffset - 1
}

func (s *ScrollableGrid) adjustScrollPos() {
	// adjust scrolling up
	if s.vScrollPos < 0 {
		s.vScrollPos = 0
		return
	}

	// adjust scrolling down
	dataLen := len(s.data)
	maxScroll := 0
	if dataLen > s.availableRowsSpace() {
		maxScroll = dataLen - s.availableRowsSpace()
	}

	// check current scrollpos whether it's still valid
	if s.vScrollPos > maxScroll {
		s.vScrollPos = maxScroll
	}
}

func (s *ScrollableGrid) Resize(x, y, w, h int) {
	s.Width = w - 1
	s.Height = h
	s.X = x + 1
	s.Y = y
	s.adjustScrollPos()
	s.Redraw()
}

func (s *ScrollableGrid) HandleEvent(ev termbox.Event) bool {
	if !s.visible {
		return false
	}

	switch ev.Type {
	case termbox.EventKey:
		if s.focused {
			switch ev.Key {
			case termbox.KeyArrowLeft:
				s.scrollLeft()
				return true

			case termbox.KeyArrowRight:
				s.scrollRight()
				return true

			case termbox.KeyArrowUp:
				s.scrollUp()
				return true

			case termbox.KeyArrowDown:
				s.scrollDown()
				return true
			}
		}
	case termbox.EventResize:
		s.Redraw()
		return true
	}

	return false
}

func (s *ScrollableGrid) UpdateData(rows [][]string) {
	s.Lock()
	defer s.Unlock()

	// clear data
	s.data = [][]string{}
	if len(rows) > 0 {
		for _, row := range rows {
			if len(row) == len(s.Columns) {
				// copy the data
				s.data = append(s.data, row[:])
			}
		}
	}
	s.adjustScrollPos()
}

func (s *ScrollableGrid) Redraw() {
	if s.visible {
		s.drawBuffer()
	}
}

func (s *ScrollableGrid) SetFocus(v bool) {
	s.focused = v
	if s.visible {
		s.Redraw()
	}
}

func (s *ScrollableGrid) Focused() bool {
	return s.focused
}

func (s *ScrollableGrid) SetVisible(v bool) {
	s.visible = v
	s.Redraw()
}

func (s *ScrollableGrid) Visible() bool {
	return s.visible
}

func (s *ScrollableGrid) reset() {
	s.hScrollPos = 1
	s.vScrollPos = 0
}

func (s *ScrollableGrid) scrollRight() {
	s.hScrollPos++
	max := len(s.Columns) - 1
	if s.hScrollPos > max {
		s.hScrollPos = max
	}
	s.Redraw()
}

func (s *ScrollableGrid) scrollLeft() {
	s.hScrollPos--
	if s.hScrollPos < 1 {
		s.hScrollPos = 1
	}
	s.Redraw()
}

func (s *ScrollableGrid) scrollUp() {
	if s.VScroller {
		s.vScrollPos--
		s.adjustScrollPos()
		s.Redraw()
	}
}

func (s *ScrollableGrid) scrollDown() {
	if s.VScroller {
		s.vScrollPos++
		s.adjustScrollPos()
		s.Redraw()
	}
}
