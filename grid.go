package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
)

type TextAlign int

const (
	AlignLeft TextAlign = iota
	AlignRight

	titleOffset   = 0
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
	Columns   []GridColumn
	VScroller bool
	Title     string
	BP        BufferProxy

	visible        bool
	focused        bool
	dataStartY     int
	hScrollPos     int
	vScrollPos     int
	dataIndex      int
	bounds         BufferRegion
	dataBounds     BufferRegion
	data           [][]string
	customDrawFunc CustomDrawFunc
	sync.RWMutex
}

func (s *ScrollableGrid) SetCustomDrawFunc(f CustomDrawFunc) {
	s.customDrawFunc = f
}

func (s *ScrollableGrid) clearRow(y int, vars ...termbox.Attribute) {
	bg := termbox.ColorDefault
	if len(vars) > 0 {
		bg = vars[0]
	}
	for i := s.dataBounds.X; i < s.dataBounds.X+s.dataBounds.W; i++ {
		s.BP.SetCell(i, y, ' ', termbox.ColorDefault, bg)
	}
}

func (s *ScrollableGrid) drawRow(cellY int, row []string, fg, bg termbox.Attribute, customizable bool, vars ...bool) {
	titleFormat := false
	if len(vars) > 0 {
		titleFormat = true
	}
	totalLen := 0
	dx := s.dataBounds.X

	// always draw the first heading
	for i := 0; i < len(row); i++ {
		if i == 0 || i >= s.hScrollPos {
			item := s.Columns[i].Format(row[i], titleFormat)
			// text length
			tw := len(item)
			totalLen += tw
			if totalLen > s.dataBounds.W {
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

func (s *ScrollableGrid) drawBorder() {
	fg := FGColor
	bg := BGColor
	if s.Focused() {
		fg |= termbox.AttrBold
	}

	// top left
	s.BP.SetCell(s.bounds.X, s.bounds.Y, '\u2554', fg, bg)
	// top right
	s.BP.SetCell(s.bounds.X+s.bounds.W, s.bounds.Y, '\u2557', fg, bg)
	for i := 1; i < s.bounds.W; i++ {
		// top line
		s.BP.SetCell(s.bounds.X+i, s.bounds.Y, '\u2550', fg, bg)
		// heading bottom
		s.BP.SetCell(s.bounds.X+i, s.bounds.Y+columnsOffset+1, '\u2550', fg, bg)
		// bottom line
		s.BP.SetCell(s.bounds.X+i, s.bounds.Y+s.bounds.H-1, '\u2550', fg, bg)
	}
	for y := 1; y < s.bounds.H-1; y++ {
		// left line
		s.BP.SetCell(s.bounds.X, s.bounds.Y+y, '\u2551', fg, bg)
		// right line
		s.BP.SetCell(s.bounds.X+s.bounds.W, s.bounds.Y+y, '\u2551', fg, bg)
	}
	// left heading junction
	s.BP.SetCell(s.bounds.X, s.bounds.Y+columnsOffset+1, '\u2560', fg, bg)
	// right heading junction
	s.BP.SetCell(s.bounds.X+s.bounds.W, s.bounds.Y+columnsOffset+1, '\u2563', fg, bg)
	// bottom left
	s.BP.SetCell(s.bounds.X, s.bounds.Y+s.bounds.H-1, '\u255a', fg, bg)
	// bottom right
	s.BP.SetCell(s.bounds.X+s.bounds.W, s.bounds.Y+s.bounds.H-1, '\u255d', fg, bg)
}

func (s *ScrollableGrid) drawHints() {
	if !s.Focused() {
		return
	}
	// horizontal scroller
	s.BP.WriteText(s.bounds.X, s.bounds.Y+columnsOffset, FGColor|termbox.AttrBold, BGColor, "\u2190")
	s.BP.WriteText(s.bounds.X+s.bounds.W, s.bounds.Y+columnsOffset, FGColor|termbox.AttrBold, BGColor, "\u2192")
	// vertical scroller
	if s.VScroller {
		cx := (s.bounds.X + s.bounds.W) / 2
		s.BP.WriteText(cx, s.dataBounds.Y-1, FGColor|termbox.AttrBold, BGColor, " \u2191 ")
		s.BP.WriteText(cx, s.dataBounds.Y+s.dataBounds.H, FGColor|termbox.AttrBold, BGColor, " \u2193 ")
	}
}

func (s *ScrollableGrid) drawHeading() {
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
	s.clearRow(s.bounds.Y+columnsOffset, highlightBGColor)
	s.drawRow(s.bounds.Y+columnsOffset, headers, highlightFGColor, highlightBGColor, false, true)
}

func (s *ScrollableGrid) drawData() {
	dataLen := len(s.data)
	if dataLen == 0 {
		return
	}

	// start of data index
	startDataIndex := s.vScrollPos
	i := 0
	for {
		row := s.data[startDataIndex]
		fg := FGColor
		bg := BGColor
		selectionIndex := s.dataIndex - s.vScrollPos
		if selectionIndex == i && s.VScroller {
			bg = BGSelectionColor
			fg = FGSelectionColor
		}
		s.drawRow(s.dataBounds.Y+i, row, fg, bg, true)
		startDataIndex++
		i++
		if i >= s.availableRowsSpace() || startDataIndex > dataLen-1 {
			break
		}
	}
}

func (s *ScrollableGrid) drawTitle() {
	cx := s.bounds.X + (s.bounds.W-runewidth.StringWidth(s.Title))/2
	s.BP.WriteText(cx, s.bounds.Y+titleOffset, FGColor, FGColor, strings.ToUpper(s.Title))
}

func (s *ScrollableGrid) drawBuffer() {
	s.RLock()
	defer s.RUnlock()

	s.drawHeading()
	s.drawBorder()
	s.drawTitle()
	s.drawData()
	s.drawHints()
}

func (s *ScrollableGrid) availableRowsSpace() int {
	return s.dataBounds.H
}

func (s *ScrollableGrid) adjustScrollPos() {
	dataLen := len(s.data)

	if dataLen == 0 {
		s.dataIndex = -1
		s.vScrollPos = 0
		return
	}

	// bound checking
	if s.dataIndex < 0 {
		s.dataIndex = 0
	}
	if s.dataIndex > dataLen-1 {
		s.dataIndex = dataLen - 1
	}

	// scroll data when beyond viewport
	delta := s.dataIndex - s.vScrollPos
	if delta < 0 || delta > s.availableRowsSpace()-1 {
		if delta > 0 {
			delta = delta - s.availableRowsSpace() + 1
		}
		s.vScrollPos += delta
	}
}

func (s *ScrollableGrid) Resize(bounds BufferRegion) {
	s.bounds = bounds
	s.dataBounds = BufferRegion{
		bounds.X + 1,
		bounds.Y + dataOffset,
		bounds.W - 1,
		bounds.H - dataOffset - 1,
	}
	s.adjustScrollPos()
	s.Redraw()
}

func (s *ScrollableGrid) HandleEvent(ev termbox.Event) bool {
	if !s.visible {
		return false
	}

	switch ev.Type {
	case termbox.EventKey:
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
	s.dataIndex = 0
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
		s.dataIndex--
		s.adjustScrollPos()
		s.Redraw()
	}
}

func (s *ScrollableGrid) scrollDown() {
	if s.VScroller {
		s.dataIndex++
		s.adjustScrollPos()
		s.Redraw()
	}
}
