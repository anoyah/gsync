package tui

import "strings"

type tone uint8

const (
	text tone = iota
	muted
	faint
	accent
	bright
	success
	warning
	danger
	selected
)

var colors = []string{
	"\x1b[0;48;5;234;38;5;252m",
	"\x1b[0;48;5;234;38;5;245m",
	"\x1b[0;48;5;234;38;5;242m",
	"\x1b[0;48;5;234;38;5;117m",
	"\x1b[0;1;48;5;234;38;5;255m",
	"\x1b[0;48;5;234;38;5;114m",
	"\x1b[0;48;5;234;38;5;222m",
	"\x1b[0;48;5;234;38;5;210m",
	"\x1b[0;1;48;5;237;38;5;117m",
}

type cell struct {
	glyph string
	role  tone
}
type canvas struct {
	width, height int
	cells         [][]cell
}

func newCanvas(width, height int) *canvas {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	c := &canvas{width: width, height: height, cells: make([][]cell, height)}
	for y := range c.cells {
		c.cells[y] = make([]cell, width)
		for x := range c.cells[y] {
			c.cells[y][x].glyph = " "
		}
	}
	return c
}
func textWidth(s string) int {
	n := 0
	for _, r := range safe(s) {
		n += cellWidth(r)
	}
	return n
}
func (c *canvas) put(x, y int, s string, role tone, width int) int {
	if y < 0 || y >= c.height || x < 0 || x >= c.width || width < 1 {
		return x
	}
	if width > c.width-x {
		width = c.width - x
	}
	for _, r := range clip(s, width) {
		w := cellWidth(r)
		if w == 0 {
			if x > 0 {
				previous := x - 1
				if c.cells[y][previous].glyph == "" && previous > 0 {
					previous--
				}
				c.cells[y][previous].glyph += string(r)
			}
			continue
		}
		c.cells[y][x] = cell{string(r), role}
		if w == 2 {
			c.cells[y][x+1] = cell{"", role}
		}
		x += w
	}
	return x
}
func (c *canvas) right(end, y int, s string, role tone) {
	s = clip(s, end-2)
	c.put(end-textWidth(s), y, s, role, textWidth(s))
}
func (c *canvas) center(x, width, y int, s string, role tone) {
	s = clip(s, width)
	c.put(x+(width-textWidth(s))/2, y, s, role, width)
}
func (c *canvas) rule(x, y, width int, role tone) {
	if width > 0 {
		c.put(x, y, strings.Repeat("─", width), role, width)
	}
}
func (c *canvas) shortcut(x, y int, key, label string, role tone, limit int) int {
	n := textWidth(key) + textWidth(label) + 5
	if x+n > limit {
		return x
	}
	end := c.put(x, y, " "+key+" ", selected, 3)
	end = c.put(end+1, y, label, role, textWidth(label))
	return end + 2
}
func (c *canvas) render() string {
	var b strings.Builder
	b.WriteString("\x1b[H")
	for y, row := range c.cells {
		b.WriteString(colors[text])
		b.WriteString("\x1b[2K")
		last := text
		for _, cell := range row {
			if cell.glyph == "" {
				continue
			}
			if cell.role != last {
				b.WriteString(colors[cell.role])
				last = cell.role
			}
			b.WriteString(cell.glyph)
		}
		if y < c.height-1 {
			b.WriteString("\r\n")
		}
	}
	b.WriteString(colors[text])
	b.WriteString("\x1b[J\x1b[0m")
	return b.String()
}
func tail(s string, width int) string {
	s = safe(s)
	if textWidth(s) <= width {
		return s
	}
	if width < 2 {
		return clip(s, width)
	}
	runes := []rune(s)
	start := len(runes)
	n := 0
	for start > 0 {
		w := cellWidth(runes[start-1])
		if n+w > width-1 {
			break
		}
		n += w
		start--
	}
	return "…" + string(runes[start:])
}
