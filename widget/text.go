package widget

import (
	"image/color"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/internal/cache"
	"fyne.io/fyne/v2/internal/widget"
	"fyne.io/fyne/v2/theme"
)

const (
	passwordChar = "•"
)

var (
	// RichTextStyleInline represents standard text that can be surrounded by other elements.
	//
	// Since: 2.1
	RichTextStyleInline = RichTextStyle{
		ColorName: theme.ColorNameForeground,
		SizeName:  theme.SizeNameText,
		Inline:    true,
	}
	// RichTextStyleParagraph represents standard text that should appear separate from other text.
	//
	// Since: 2.1
	RichTextStyleParagraph = RichTextStyle{
		ColorName: theme.ColorNameForeground,
		SizeName:  theme.SizeNameText,
		Inline:    false,
	}
)

// RichTextStyle describes the details of a text object inside a RichText widget.
//
// Since: 2.1
type RichTextStyle struct {
	Alignment fyne.TextAlign
	ColorName fyne.ThemeColorName
	Inline    bool
	SizeName  fyne.ThemeSizeName
	TextStyle fyne.TextStyle
}

// RichTextSegment describes any element that can be rendered in a RichText widget.
//
// Since: 2.1
type RichTextSegment interface {
	Inline() bool
	Textual() string
	Visual() fyne.CanvasObject

	Select(pos1, pos2 fyne.Position)
	SelectedText() string
	Unselect()
}

// TextSegment represents the styling for a segment of rich text.
//
// Since: 2.1
type TextSegment struct {
	Style RichTextStyle
	Text  string

	concealed bool // TODO a different type
}

// Inline should return true if this text can be included within other elements, or false if it creates a new block.
func (t *TextSegment) Inline() bool {
	return t.Style.Inline
}

// Textual returns the content of this segment rendered to plain text.
func (t *TextSegment) Textual() string {
	return t.Text
}

// Visual returns the graphical elements required to render this segment.
func (t *TextSegment) Visual() fyne.CanvasObject {
	obj := canvas.NewText(t.Text, t.color())

	obj.Alignment = t.Style.Alignment
	obj.TextStyle = t.Style.TextStyle
	obj.TextSize = t.size()
	return obj
}

// Select tells the segment that the user is selecting the content between the two positions.
func (t *TextSegment) Select(begin, end fyne.Position) {
	// no-op: this will be added when we progress to editor
}

// SelectedText should return the text representation of any content currently selected through the Select call.
func (t *TextSegment) SelectedText() string {
	// no-op: this will be added when we progress to editor
	return ""
}

// Unselect tells the segment that the user is has cancelled the previous selection.
func (t *TextSegment) Unselect() {
	// no-op: this will be added when we progress to editor
}

func (t TextSegment) color() color.Color {
	if t.Style.ColorName != "" {
		return fyne.CurrentApp().Settings().Theme().Color(t.Style.ColorName, fyne.CurrentApp().Settings().ThemeVariant())
	}

	return theme.ForegroundColor()
}

func (t TextSegment) size() float32 {
	if t.Style.SizeName != "" {
		return fyne.CurrentApp().Settings().Theme().Size(t.Style.SizeName)
	}

	return theme.TextSize()
}

// RichText represents the base element for a rich text-based widget.
//
// Since: 2.1
type RichText struct {
	BaseWidget
	Segments []RichTextSegment
	Wrapping fyne.TextWrap

	inset     fyne.Size     // this varies due to how the widget works (entry with scroller vs others with padding)
	rowBounds []rowBoundary // cache for boundaries
}

// NewRichText returns a new RichText widget that renders the given text and segments.
// If no segments are specified it will be converted to a single segment using the default text settings.
//
// Since: 2.1
func NewRichText(segments ...RichTextSegment) *RichText {
	t := &RichText{Segments: segments}
	t.updateRowBounds()
	return t
}

// NewRichTextWithText returns a new RichText widget that renders the given text.
// The string will be converted to a single text segment using the default text settings.
//
// Since: 2.1
func NewRichTextWithText(text string) *RichText {
	return NewRichText(&TextSegment{
		Style: RichTextStyleInline,
		Text:  text,
	})
}

// CreateRenderer is a private method to Fyne which links this widget to its renderer
func (t *RichText) CreateRenderer() fyne.WidgetRenderer {
	t.ExtendBaseWidget(t)
	r := &textRenderer{obj: t}

	t.updateRowBounds() // set up the initial text layout etc
	r.Refresh()
	return r
}

// MinSize calculates the minimum size of this rich text.
// This is based on the contained text with a standard amount of padding added.
func (t *RichText) MinSize() fyne.Size {
	charMinSize := t.charMinSize(false)
	concealedMinSize := t.charMinSize(true)
	height := float32(0)
	width := float32(0)
	i := 0

	t.propertyLock.RLock()
	count := t.rows()
	wrap := t.Wrapping
	t.propertyLock.RUnlock()

	for ; i < count; i++ {
		str := string(t.row(i))
		bound := t.rowBoundary(i)
		min := fyne.MeasureText(str, bound.seg.size(), bound.seg.Style.TextStyle)
		if str == "" {
			if bound.seg.concealed {
				min = concealedMinSize
			} else {
				min = charMinSize
			}
		}
		if wrap == fyne.TextWrapOff {
			width = fyne.Max(width, min.Width)
		}
		if i == count-1 || bound == nil || !bound.inline {
			height += min.Height
		}
	}

	if height == 0 {
		height = charMinSize.Height
	}

	return fyne.NewSize(width, height).
		Add(fyne.NewSize(theme.Padding()*4, theme.Padding()*4).Subtract(t.inset).Subtract(t.inset))
}

// Resize sets a new size for the rich text.
// This should only be called if it is not in a container with a layout manager.
//
// Implements: fyne.Widget
func (t *RichText) Resize(size fyne.Size) {
	t.propertyLock.RLock()
	baseSize := t.size
	t.propertyLock.RUnlock()
	if baseSize == size {
		return
	}

	t.propertyLock.Lock()
	t.size = size
	t.propertyLock.Unlock()
	t.updateRowBounds()

	t.Refresh()
	cache.Renderer(t).Layout(size)
}

// Refresh triggers a redraw of the rich text.
//
// Implements: fyne.Widget
func (t *RichText) Refresh() {
	t.updateRowBounds()

	t.BaseWidget.Refresh()
}

// String returns the text widget buffer as string
func (t *RichText) String() string {
	ret := strings.Builder{}
	for _, seg := range t.Segments {
		ret.WriteString(seg.Textual())
	}
	return ret.String()
}

// Len returns the text widget buffer length
func (t *RichText) len() int {
	ret := 0
	for _, seg := range t.Segments {
		ret += len([]rune(seg.Textual()))
	}
	return ret
}

// insertAt inserts the text at the specified position
func (t *RichText) insertAt(pos int, runes string) {
	index := 0
	start := 0
	var into *TextSegment
	for i, seg := range t.Segments {
		if _, ok := seg.(*TextSegment); !ok {
			continue
		}
		end := start + len([]rune(seg.(*TextSegment).Text))
		into = seg.(*TextSegment)
		index = i
		if end > pos {
			break
		}

		start = end
	}

	if into == nil {
		return
	}
	r := ([]rune)(into.Text)
	r2 := append(r[:pos], append([]rune(runes), r[pos:]...)...)
	into.Text = string(r2)
	t.Segments[index] = into

	t.Refresh()
}

// deleteFromTo removes the text between the specified positions
func (t *RichText) deleteFromTo(lowBound int, highBound int) string {
	// TODO handle start portion, whole elements and end portion!
	index := 0
	start := 0
	var from *TextSegment
	for i, seg := range t.Segments {
		if _, ok := seg.(*TextSegment); !ok {
			continue
		}
		end := start + len([]rune(seg.(*TextSegment).Text))
		from = seg.(*TextSegment)
		index = i
		if end > lowBound {
			break
		}

		start = end
	}

	if from == nil {
		return ""
	}
	deleted := make([]rune, highBound-lowBound)
	r := ([]rune)(from.Text)
	copy(deleted, r[lowBound:highBound])
	if highBound > len(r) {
		highBound = len(r) // TODO remove this workaround and delete all segments)
	}
	r2 := append(r[:lowBound], r[highBound:]...)
	from.Text = string(r2)
	t.Segments[index] = from
	t.Refresh()
	return string(deleted)
}

// rows returns the number of text rows in this text entry.
// The entry may be longer than required to show this amount of content.
func (t *RichText) rows() int {
	return len(t.rowBounds)
}

// Row returns the characters in the row specified.
// The row parameter should be between 0 and t.Rows()-1.
func (t *RichText) row(row int) []rune {
	if row < 0 || row >= t.rows() {
		return nil
	}
	bounds := t.rowBounds[row]
	from := bounds.begin
	to := bounds.end
	if from < 0 || to > len([]rune(bounds.seg.Text)) {
		return nil
	}
	if to < from {
		return nil
	}

	b := ([]rune)(bounds.seg.Text)
	return b[from:to]
}

// RowBoundary returns the boundary of the row specified.
// The row parameter should be between 0 and t.Rows()-1.
func (t *RichText) rowBoundary(row int) *rowBoundary {
	t.propertyLock.RLock()
	defer t.propertyLock.RUnlock()
	if row < 0 || row >= t.rows() {
		return nil
	}
	return &t.rowBounds[row]
}

// RowLength returns the number of visible characters in the row specified.
// The row parameter should be between 0 and t.Rows()-1.
func (t *RichText) rowLength(row int) int {
	return len(t.row(row))
}

// CharMinSize returns the average char size to use for internal computation
func (t *RichText) charMinSize(concealed bool) fyne.Size {
	defaultChar := "M"
	if concealed {
		defaultChar = passwordChar
	}

	// TODO move this out as our first segment may not be text!
	return fyne.MeasureText(defaultChar, t.Segments[0].(*TextSegment).size(), t.Segments[0].(*TextSegment).Style.TextStyle)
}

// lineSizeToColumn returns the rendered size for the line specified by row up to the col position
func (t *RichText) lineSizeToColumn(col, row int) fyne.Size {
	line := t.row(row)
	if line == nil {
		return fyne.NewSize(0, 0)
	}

	if col >= len(line) {
		col = len(line)
	}

	measureText := string(line[0:col])
	bound := t.rowBoundary(row)
	if bound.seg.concealed {
		measureText = strings.Repeat(passwordChar, col)
	}

	label := canvas.NewText(measureText, color.Black)
	label.TextStyle = bound.seg.Style.TextStyle
	label.TextSize = bound.seg.size()
	return label.MinSize().Add(fyne.NewSize(theme.Padding()-t.inset.Width, 0))
}

// updateRowBounds updates the row bounds used to render properly the text widget.
// updateRowBounds should be invoked every time a segment Text, widget Wrapping or size changes.
func (t *RichText) updateRowBounds() {
	t.propertyLock.RLock()
	var bounds []rowBoundary
	for _, seg := range t.Segments {
		if _, ok := seg.(*TextSegment); !ok {
			continue
		}
		textSeg := seg.(*TextSegment)
		textStyle := textSeg.Style.TextStyle
		textSize := textSeg.size()
		maxWidth := t.size.Width - 2*theme.Padding()

		bounds = append(bounds, lineBounds(textSeg, t.Wrapping, maxWidth, func(text []rune) float32 {
			return fyne.MeasureText(string(text), textSize, textStyle).Width
		})...)
		if len(bounds) == 0 {
			continue
		}
		bounds[len(bounds)-1].inline = seg.Inline()
	}
	t.propertyLock.RUnlock()

	t.propertyLock.Lock()
	t.rowBounds = bounds
	t.propertyLock.Unlock()
}

// Renderer
type textRenderer struct {
	widget.BaseRenderer
	obj *RichText
}

// MinSize calculates the minimum size of a rich text widget.
// This is based on the contained text with a standard amount of padding added.
func (r *textRenderer) MinSize() fyne.Size {
	r.obj.propertyLock.RLock()
	bounds := r.obj.rowBounds
	wrap := r.obj.Wrapping
	r.obj.propertyLock.RUnlock()

	charMinSize := r.obj.charMinSize(false)
	height := float32(0)
	width := float32(0)
	i := 0

	r.obj.propertyLock.RLock()
	objs := r.Objects()
	count := int(fyne.Min(float32(len(objs)), float32(r.obj.rows())))
	r.obj.propertyLock.RUnlock()

	for ; i < count; i++ {
		var bound *rowBoundary
		if i < count {
			bound = &bounds[i]
		}
		min := objs[i].MinSize()
		if wrap == fyne.TextWrapOff {
			width = fyne.Max(width, min.Width)
		}

		if i == count-1 || bound == nil || !bound.inline {
			height += min.Height
		}
	}

	if height == 0 {
		height = charMinSize.Height
	}
	return fyne.NewSize(width, height).
		Add(fyne.NewSize(theme.Padding()*4, theme.Padding()*4).Subtract(r.obj.inset).Subtract(r.obj.inset))
}

func (r *textRenderer) Layout(size fyne.Size) {
	r.obj.propertyLock.RLock()
	bounds := r.obj.rowBounds
	defer r.obj.propertyLock.RUnlock()

	left := theme.Padding()*2 - r.obj.inset.Width
	yPos := theme.Padding()*2 - r.obj.inset.Height
	lineHeight := r.obj.charMinSize(false).Height
	lineWidth := size.Width - yPos*2
	var rowTexts []*canvas.Text
	rowAlign := fyne.TextAlignLeading
	for i, obj := range r.Objects() {
		rowTexts = append(rowTexts, obj.(*canvas.Text))
		var bound *rowBoundary
		if i < len(bounds) {
			bound = &bounds[i]
		}

		if len(rowTexts) == 1 && bound != nil {
			rowAlign = bound.seg.Style.Alignment
		}
		if i < len(r.Objects())-1 && (bound == nil || bound.inline) {
			continue
		}
		r.layoutRow(rowTexts, rowAlign, left, yPos, lineWidth, lineHeight)
		yPos += lineHeight
		rowTexts = nil
	}
}

func (r *textRenderer) layoutRow(texts []*canvas.Text, align fyne.TextAlign, xPos, yPos, lineWidth, lineHeight float32) {
	if len(texts) == 1 {
		texts[0].Resize(fyne.NewSize(lineWidth, lineHeight))
		texts[0].Move(fyne.NewPos(xPos, yPos))
		return
	}
	for i, text := range texts {
		size := text.MinSize()

		text.Resize(fyne.NewSize(size.Width, fyne.Max(lineHeight, size.Height)))
		text.Move(fyne.NewPos(xPos, yPos)) // TODO also baseline align for height (need new measure info)

		xPos += size.Width
		if i < len(texts)-1 {
			xPos += fyne.MeasureText(" ", text.TextSize, text.TextStyle).Width
		}
	}
	spare := lineWidth - xPos
	switch align {
	case fyne.TextAlignTrailing:
		first := texts[0]
		first.Resize(fyne.NewSize(first.Size().Width+spare, lineHeight))
		first.Alignment = fyne.TextAlignTrailing

		for _, text := range texts[1:] {
			text.Move(text.Position().Add(fyne.NewPos(spare, 0)))
		}
	case fyne.TextAlignCenter:
		pad := spare / 2
		first := texts[0]
		first.Resize(fyne.NewSize(first.Size().Width+pad, lineHeight))
		first.Alignment = fyne.TextAlignTrailing
		last := texts[len(texts)-1]
		last.Resize(fyne.NewSize(last.Size().Width+pad, lineHeight))
		last.Alignment = fyne.TextAlignLeading

		for _, text := range texts[1:] {
			text.Move(text.Position().Add(fyne.NewPos(pad, 0)))
		}
	default:
		last := texts[len(texts)-1]
		last.Resize(fyne.NewSize(last.Size().Width+spare, lineHeight))
		last.Alignment = fyne.TextAlignLeading
	}
}

func (r *textRenderer) Refresh() {
	index := 0
	var objs []fyne.CanvasObject
	for ; index < r.obj.rows(); index++ {
		bound := r.obj.rowBoundary(index)

		obj := bound.seg.Visual()

		if txt, ok := obj.(*canvas.Text); ok {
			if bound.begin != 0 || bound.end != len([]rune(txt.Text)) {
				txt.Text = txt.Text[bound.begin:bound.end]
			}
			if bound.seg.concealed {
				txt.Text = strings.Repeat(passwordChar, len([]rune(txt.Text)))
			}
		}
		objs = append(objs, obj)
	}

	r.obj.propertyLock.Lock()
	r.SetObjects(objs)
	r.obj.propertyLock.Unlock()

	r.Layout(r.obj.Size())
	canvas.Refresh(r.obj)
}

// splitLines accepts a text segment and returns a slice of boundary metadata denoting the
// start and end indices of each line delimited by the newline character.
func splitLines(seg *TextSegment) []rowBoundary {
	var low, high int
	var lines []rowBoundary
	text := []rune(seg.Text)
	length := len(text)
	for i := 0; i < length; i++ {
		if text[i] == '\n' {
			high = i
			lines = append(lines, rowBoundary{seg, low, high, false})
			low = i + 1
		}
	}
	return append(lines, rowBoundary{seg, low, length, true})
}

// binarySearch accepts a function that checks if the text width less the maximum width and the start and end rune index
// binarySearch returns the index of rune located as close to the maximum line width as possible
func binarySearch(lessMaxWidth func(int, int) bool, low int, maxHigh int) int {
	if low >= maxHigh {
		return low
	}
	if lessMaxWidth(low, maxHigh) {
		return maxHigh
	}
	high := low
	delta := maxHigh - low
	for delta > 0 {
		delta /= 2
		if lessMaxWidth(low, high+delta) {
			high += delta
		}
	}
	for (high < maxHigh) && lessMaxWidth(low, high+1) {
		high++
	}
	return high
}

// findSpaceIndex accepts a slice of runes and a fallback index
// findSpaceIndex returns the index of the last space in the text, or fallback if there are no spaces
func findSpaceIndex(text []rune, fallback int) int {
	curIndex := fallback
	for ; curIndex >= 0; curIndex-- {
		if unicode.IsSpace(text[curIndex]) {
			break
		}
	}
	if curIndex < 0 {
		return fallback
	}
	return curIndex
}

// lineBounds accepts a slice of Segments, a wrapping mode, a maximum line width and a function to measure line width.
// lineBounds returns a slice containing the boundary metadata of each line with the given wrapping applied.
func lineBounds(seg *TextSegment, wrap fyne.TextWrap, maxWidth float32, measurer func([]rune) float32) []rowBoundary {
	lines := splitLines(seg)
	if maxWidth <= 0 || wrap == fyne.TextWrapOff {
		return lines
	}

	text := []rune(seg.Text)
	checker := func(low int, high int) bool {
		return measurer(text[low:high]) <= maxWidth
	}

	var bounds []rowBoundary
	for _, l := range lines {
		low := l.begin
		high := l.end
		if low == high {
			bounds = append(bounds, l)
			continue
		}
		switch wrap {
		case fyne.TextTruncate:
			high = binarySearch(checker, low, high)
			bounds = append(bounds, rowBoundary{seg, low, high, false})
		case fyne.TextWrapBreak:
			for low < high {
				if measurer(text[low:high]) <= maxWidth {
					bounds = append(bounds, rowBoundary{seg, low, high, false})
					low = high
					high = l.end
				} else {
					high = binarySearch(checker, low, high)
				}
			}
		case fyne.TextWrapWord:
			for low < high {
				sub := text[low:high]
				if measurer(sub) <= maxWidth {
					bounds = append(bounds, rowBoundary{seg, low, high, false})
					low = high
					high = l.end
					if low < high && unicode.IsSpace(text[low]) {
						low++
					}
				} else {
					last := low + len(sub) - 1
					high = low + findSpaceIndex(sub, binarySearch(checker, low, last)-low)
				}
			}
		}
	}
	return bounds
}

type rowBoundary struct {
	seg        *TextSegment
	begin, end int
	inline     bool
}
