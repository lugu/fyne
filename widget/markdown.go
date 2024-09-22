package widget

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

// NewRichTextFromMarkdown configures a RichText widget by parsing the provided markdown content.
//
// Since: 2.1
func NewRichTextFromMarkdown(content string) *RichText {
	return NewRichText(parseMarkdown(content)...)
}

// ParseMarkdown allows setting the content of this RichText widget from a markdown string.
// It will replace the content of this widget similarly to SetText, but with the appropriate formatting.
func (t *RichText) ParseMarkdown(content string) {
	t.Segments = parseMarkdown(content)
	t.Refresh()
}

// AppendMarkdown parses the given markdown string and appends the
// content to the widget, with the appropriate formatting.
// This API is intended for appending complete markdown documents or
// standalone fragments, and should not be used to parse a single
// markdown document piecewise.
//
// Since: 2.5
func (t *RichText) AppendMarkdown(content string) {
	t.Segments = append(t.Segments, parseMarkdown(content)...)
	t.Refresh()
}

type markdownRenderer []RichTextSegment

func (m *markdownRenderer) AddOptions(...renderer.Option) {}

func (m *markdownRenderer) Render(_ io.Writer, source []byte, n ast.Node) error {
	segs, err := renderNode(source, n, false)
	*m = segs
	return err
}

func renderNode(source []byte, n ast.Node, blockquote bool) ([]RichTextSegment, error) {
	switch t := n.(type) {
	case *ast.Document:
		return renderChildren(source, n, blockquote)
	case *ast.Paragraph:
		children, err := renderChildren(source, n, blockquote)
		if !blockquote {
			linebreak := &TextSegment{Style: RichTextStyleParagraph}
			children = append(children, linebreak)
		}
		return children, err
	case *ast.List:
		items, err := renderChildren(source, n, blockquote)
		return []RichTextSegment{
			&ListSegment{Items: items, Ordered: t.Marker != '*' && t.Marker != '-' && t.Marker != '+'},
		}, err
	case *ast.ListItem:
		texts, err := renderChildren(source, n, blockquote)
		return []RichTextSegment{&ParagraphSegment{Texts: texts}}, err
	case *ast.TextBlock:
		return renderChildren(source, n, blockquote)
	case *ast.Heading:
		text := forceIntoHeadingText(source, n)
		switch t.Level {
		case 1:
			return []RichTextSegment{&TextSegment{Style: RichTextStyleHeading, Text: text}}, nil
		case 2:
			return []RichTextSegment{&TextSegment{Style: RichTextStyleSubHeading, Text: text}}, nil
		default:
			textSegment := TextSegment{Style: RichTextStyleParagraph, Text: text}
			textSegment.Style.TextStyle.Bold = true
			return []RichTextSegment{&textSegment}, nil
		}
	case *ast.ThematicBreak:
		return []RichTextSegment{&SeparatorSegment{}}, nil
	case *ast.Link:
		link, _ := url.Parse(string(t.Destination))
		text := forceIntoText(source, n)
		return []RichTextSegment{&HyperlinkSegment{Alignment: fyne.TextAlignLeading, Text: text, URL: link}}, nil
	case *ast.CodeSpan:
		text := forceIntoText(source, n)
		return []RichTextSegment{&TextSegment{Style: RichTextStyleCodeInline, Text: text}}, nil
	case *ast.CodeBlock, *ast.FencedCodeBlock:
		var data []byte
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			data = append(data, line.Value(source)...)
		}
		if len(data) == 0 {
			return nil, nil
		}
		if data[len(data)-1] == '\n' {
			data = data[:len(data)-1]
		}
		return []RichTextSegment{&TextSegment{Style: RichTextStyleCodeBlock, Text: string(data)}}, nil
	case *ast.Emphasis:
		text := string(forceIntoText(source, n))
		switch t.Level {
		case 2:
			return []RichTextSegment{&TextSegment{Style: RichTextStyleStrong, Text: text}}, nil
		default:
			return []RichTextSegment{&TextSegment{Style: RichTextStyleEmphasis, Text: text}}, nil
		}
	case *ast.Text:
		text := string(t.Text(source))
		if text == "" {
			// These empty text elements indicate single line breaks after non-text elements in goldmark.
			return []RichTextSegment{&TextSegment{Style: RichTextStyleInline, Text: " "}}, nil
		}
		text = suffixSpaceIfAppropriate(text, n)
		if blockquote {
			return []RichTextSegment{&TextSegment{Style: RichTextStyleBlockquote, Text: text}}, nil
		}
		return []RichTextSegment{&TextSegment{Style: RichTextStyleInline, Text: text}}, nil
	case *ast.Blockquote:
		return renderChildren(source, n, true)
	case *ast.Image:
		dest := string(t.Destination)
		u, err := storage.ParseURI(dest)
		if err != nil {
			u = storage.NewFileURI(dest)
		}
		return []RichTextSegment{&ImageSegment{Source: u, Title: string(t.Title), Alignment: fyne.TextAlignCenter}}, nil
	case *east.TableCell:
		segs, err := renderChildren(source, n, blockquote)
		if err != nil {
			return nil, err
		}
		return []RichTextSegment{NewTableCell(NewRichText(segs...))}, nil

	case *east.TableHeader:
		segs, err := renderChildren(source, n, blockquote)
		if err != nil {
			return nil, err
		}
		cells := make([]*TableCell, len(segs))
		for i, seg := range segs {
			cell, ok := seg.(*TableCell)
			if !ok {
				return nil, fmt.Errorf("Unable to cast element %d to TableCell", i)
			}
			cells[i] = cell
		}
		return []RichTextSegment{&TableRow{cells: cells}}, nil
	case *east.TableRow:
		segs, err := renderChildren(source, n, blockquote)
		if err != nil {
			return nil, err
		}
		cells := make([]*TableCell, len(segs))
		for i, seg := range segs {
			cell, ok := seg.(*TableCell)
			if !ok {
				return nil, fmt.Errorf("Unable to cast element %d to TableCell", i)
			}
			cells[i] = cell
		}
		return []RichTextSegment{&TableRow{cells: cells}}, nil
	case *east.Table:
		segs, err := renderChildren(source, n, blockquote)
		if err != nil {
			return nil, err
		}
		rows := make([]*TableRow, len(segs))
		for i, seg := range segs {
			row, ok := seg.(*TableRow)
			if !ok {
				return nil, fmt.Errorf("Unable to cast element %d to TableCell", i)
			}
			rows[i] = row
		}
		return []RichTextSegment{NewTableSegment(rows)}, nil
	}
	return nil, nil
}

func suffixSpaceIfAppropriate(text string, n ast.Node) string {
	next := n.NextSibling()
	if next != nil && next.Type() == ast.TypeInline && !strings.HasSuffix(text, " ") {
		return text + " "
	}
	return text
}

func renderChildren(source []byte, n ast.Node, blockquote bool) ([]RichTextSegment, error) {
	children := make([]RichTextSegment, 0, n.ChildCount())
	for childCount, child := n.ChildCount(), n.FirstChild(); childCount > 0; childCount-- {
		if child == nil {
			continue
		}
		segs, err := renderNode(source, child, blockquote)
		if err != nil {
			return children, err
		}
		children = append(children, segs...)
		child = child.NextSibling()
	}
	return children, nil
}

func forceIntoText(source []byte, n ast.Node) string {
	texts := make([]string, 0)
	ast.Walk(n, func(n2 ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch t := n2.(type) {
			case *ast.Text:
				texts = append(texts, string(t.Text(source)))
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.Join(texts, " ")
}

func forceIntoHeadingText(source []byte, n ast.Node) string {
	var text strings.Builder
	ast.Walk(n, func(n2 ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch t := n2.(type) {
			case *ast.Text:
				text.Write(t.Text(source))
			}
		}
		return ast.WalkContinue, nil
	})
	return text.String()
}

func parseMarkdown(content string) []RichTextSegment {
	r := markdownRenderer{}
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithRenderer(&r))
	err := md.Convert([]byte(content), nil)
	if err != nil {
		fyne.LogError("Failed to parse markdown", err)
	}
	return r
}

type (
	DummyRichTextSegment struct{}
	TableCell            struct {
		BaseWidget
		DummyRichTextSegment
		content  *RichText
		renderer cellRenderer
	}
	TableRow struct {
		DummyRichTextSegment
		cells []*TableCell
	}
	TableSegment struct {
		Table
		DummyRichTextSegment
		rows []*TableRow
		size fyne.Size
	}
)

// DummyRichTextSegment is used by TableRow and TableCell to conform with RichTextSegment.
func (c *DummyRichTextSegment) Inline() bool                    { return false }
func (c *DummyRichTextSegment) Textual() string                 { panic("not implemented") }
func (c *DummyRichTextSegment) Update(fyne.CanvasObject)        { panic("not implemented") }
func (c *DummyRichTextSegment) Visual() fyne.CanvasObject       { panic("not implemented") }
func (c *DummyRichTextSegment) Select(pos1, pos2 fyne.Position) { panic("not implemented") }
func (c *DummyRichTextSegment) SelectedText() string            { panic("not implemented") }
func (c *DummyRichTextSegment) Unselect()                       { panic("not implemented") }

// Cell implements CreateRenderer and draw the underlaying RichTextSegments using RichText.
func (c *TableCell) CreateRenderer() fyne.WidgetRenderer {
	return c.renderer
}

func NewTableCell(content *RichText) *TableCell {
	cell := &TableCell{
		content:  content,
		renderer: NewCellRenderer(content),
	}
	cell.ExtendBaseWidget(cell)
	return cell
}

func (c *TableCell) updateSegment(content *RichText) {
	c.content = content
	c.renderer.setObject(c.content)
}

func NewTableSegment(rows []*TableRow) *TableSegment {
	length := func() (int, int) {
		if len(rows) > 0 {
			return len(rows), len(rows[0].cells)
		}
		return 0, 0
	}
	create := func() fyne.CanvasObject {
		return NewTableCell(NewRichText(&TextSegment{}))
	}
	update := func(pos TableCellID, o fyne.CanvasObject) {
		if pos.Row >= len(rows) || pos.Col >= len(rows[pos.Row].cells) {
			return
		}
		cell := o.(*TableCell)
		cell.updateSegment(rows[pos.Row].cells[pos.Col].content)
	}
	table := &TableSegment{
		Table: Table{
			Length:     length,
			CreateCell: create,
			UpdateCell: update,
		},
		rows: rows,
	}
	table.ExtendBaseWidget(table)
	table.resize()
	return table
}

func (l *TableSegment) resize() {
	// Compute the size of the columns and rows
	widths := []float32{}
	heights := []float32{}
	for i, row := range l.rows {
		for j, cell := range row.cells {
			width := cell.content.MinSize().Width
			height := cell.content.MinSize().Height
			if len(heights) < i+1 {
				heights = append(heights, height)
			} else if heights[i] < height {
				heights[i] = height
			}
			if len(widths) < j+1 {
				widths = append(widths, width)
			} else if widths[j] < width {
				widths[j] = width
			}
		}
	}
	l.size.Height = 0
	for i, height := range heights {
		l.SetRowHeight(i, height)
		l.size.Height += height + 4
	}
	l.size.Width = 0
	for j, width := range widths {
		l.SetColumnWidth(j, width+8)
		l.size.Width += width + 16
	}
}

func (l *TableSegment) Unselect()                       { panic("not implemented") }
func (l *TableSegment) Select(pos1, pos2 fyne.Position) { panic("not implemented") }
func (l *TableSegment) SelectedText() string            { panic("not implemented") }

// MinSize returns the table size otherwise is it minimzed.
func (l *TableSegment) MinSize() fyne.Size {
	return l.size
}

// Visual returns the graphical elements required to render this segment.
func (l *TableSegment) Visual() fyne.CanvasObject {
	return l
}

// Update applies the current state of this table segment to an existing visual.
func (l *TableSegment) Update(o fyne.CanvasObject) {}

// cellRenderer implements fyne.WidgetRenderer. It contains exactly one canvas object.
type cellRenderer []fyne.CanvasObject

func NewCellRenderer(object fyne.CanvasObject) cellRenderer {
	return cellRenderer([]fyne.CanvasObject{object})
}

func (r cellRenderer) setObject(object fyne.CanvasObject) {
	r[0] = object
}

// Destroy does nothing in this implementation.
func (r cellRenderer) Destroy() {
}

// Layout updates the contained object to be the requested size.
func (r cellRenderer) Layout(s fyne.Size) {
	r[0].Resize(s)
}

// MinSize returns the smallest size that this render can use, returned from the underlying object.
func (r cellRenderer) MinSize() fyne.Size {
	return r[0].MinSize()
}

// Objects returns the objects that should be rendered.
func (r cellRenderer) Objects() []fyne.CanvasObject {
	return r
}

// Refresh requests the underlying object to redraw.
func (r cellRenderer) Refresh() {
	r[0].Refresh()
}
