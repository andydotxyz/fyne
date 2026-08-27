package widget

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// segmentDump renders the segments as "text|style" pairs so that tests can
// assert on the structure without depending on exact style values.
func segmentDump(e *RichTextEntry) []string {
	var out []string
	for _, seg := range e.Segments() {
		text, ok := seg.(*TextSegment)
		if !ok {
			out = append(out, seg.Textual()+"|object")
			continue
		}

		style := ""
		if text.Style.TextStyle.Bold {
			style += "b"
		}
		if text.Style.TextStyle.Italic {
			style += "i"
		}
		if text.Style.TextStyle.Monospace {
			style += "m"
		}
		if text.Style.TextStyle.Strikethrough {
			style += "s"
		}
		switch text.Style.SizeName {
		case theme.SizeNameHeadingText:
			style += "h1"
		case theme.SizeNameSubHeadingText:
			style += "h2"
		}
		out = append(out, text.Text+"|"+style)
	}
	return out
}

func typeString(e *RichTextEntry, s string) {
	for _, r := range s {
		e.TypedRune(r)
	}
}

func TestRichTextEntry_ParseMarkdown(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("# Title\n\nSome **bold** and *italic* text.")

	assert.Equal(t, "Title\nSome bold and italic text.", e.Text)
	assert.Equal(t, []string{"Title|bh1", "\nSome |", "bold|b", " and |", "italic|i", " text.|"},
		segmentDump(e))
}

func TestRichTextEntry_ParseMarkdown_Blocks(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("- one\n- two\n\n> quoted")

	assert.Equal(t, "•  one\n•  two\nquoted", e.Text)
}

func TestRichTextEntry_SetText(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("# Title")
	e.SetText("plain")

	assert.Equal(t, "plain", e.Text)
	assert.Equal(t, []string{"plain|"}, segmentDump(e))
}

func TestRichTextEntry_TypedRune_KeepsStyles(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("a **b** c")
	e.CursorRow, e.CursorColumn = 0, 3 // just after the bold "b"

	e.TypedRune('X')

	assert.Equal(t, "a bX c", e.Text)
	assert.Equal(t, []string{"a |", "bX|b", " c|"}, segmentDump(e))
	assert.Equal(t, 4, e.CursorColumn)
}

func TestRichTextEntry_TypedRune_AtSegmentStart(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("**b** c")
	e.CursorRow, e.CursorColumn = 0, 0

	e.TypedRune('X')

	assert.Equal(t, "Xb c", e.Text)
	assert.Equal(t, []string{"Xb|b", " c|"}, segmentDump(e))
}

func TestRichTextEntry_Backspace_AcrossSegments(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("a **b** c")
	e.CursorRow, e.CursorColumn = 0, 3

	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})

	assert.Equal(t, "a  c", e.Text)
	// the emptied bold segment stays at the cursor, so typing continues in bold
	assert.Equal(t, []string{"a |", "|b", " c|"}, segmentDump(e))
	assert.Equal(t, 2, e.CursorColumn)
}

func TestRichTextEntry_Rows(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("# Title\n\nBody text")
	provider := e.textProvider()

	assert.Equal(t, 2, provider.rows())
	assert.Equal(t, "Title", string(provider.row(0)))
	assert.Equal(t, "Body text", string(provider.row(1)))

	// the heading row is taller than the body row
	_, headingHeight := provider.rowGeometry(0)
	bodyY, bodyHeight := provider.rowGeometry(1)
	assert.Greater(t, headingHeight, bodyHeight)
	assert.Equal(t, headingHeight, bodyY)
}

func TestRichTextEntry_CursorOffsetAcrossRows(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("# Title\n\nBody")

	e.CursorRow, e.CursorColumn = 1, 2
	assert.Equal(t, 8, e.CursorTextOffset()) // "Title\n" is 6 runes

	row, col := e.rowColFromTextPos(8)
	assert.Equal(t, 1, row)
	assert.Equal(t, 2, col)
}

func TestRichTextEntry_SetStyleForRange(t *testing.T) {
	e := NewRichTextEntry()
	e.SetText("hello world")

	style := RichTextStyleInline
	style.TextStyle.Bold = true
	e.SetStyleForRange(6, 11, style)

	assert.Equal(t, "hello world", e.Text)
	assert.Equal(t, []string{"hello |", "world|b"}, segmentDump(e))
}

func TestRichTextEntry_SetStyleForSelection(t *testing.T) {
	e := NewRichTextEntry()
	e.SetText("hello world")
	e.syncSelectable()
	e.sel.selecting = true
	e.sel.selectRow, e.sel.selectColumn = 0, 0
	e.CursorRow, e.CursorColumn = 0, 5
	e.syncSelectable()

	style := RichTextStyleInline
	style.TextStyle.Italic = true
	e.SetStyleForSelection(style)

	assert.Equal(t, []string{"hello|i", " world|"}, segmentDump(e))
}

func TestRichTextEntry_MarkdownMode_Bold(t *testing.T) {
	e := NewRichTextEntry()
	e.TypeMarkdown = true

	typeString(e, "a **b** c")

	assert.Equal(t, "a b c", e.Text)
	assert.Equal(t, []string{"a |", "b|b", " c|"}, segmentDump(e))
}

func TestRichTextEntry_MarkdownMode_Italic(t *testing.T) {
	e := NewRichTextEntry()
	e.TypeMarkdown = true

	typeString(e, "*hi* there")

	assert.Equal(t, "hi there", e.Text)
	assert.Equal(t, []string{"hi|i", " there|"}, segmentDump(e))
}

func TestRichTextEntry_MarkdownMode_Code(t *testing.T) {
	e := NewRichTextEntry()
	e.TypeMarkdown = true

	typeString(e, "run `go test` now")

	assert.Equal(t, "run go test now", e.Text)
	assert.Equal(t, []string{"run |", "go test|m", " now|"}, segmentDump(e))
}

func TestRichTextEntry_MarkdownMode_Heading(t *testing.T) {
	e := NewRichTextEntry()
	e.TypeMarkdown = true

	typeString(e, "# Title")

	assert.Equal(t, "Title", e.Text)
	assert.Equal(t, []string{"Title|bh1"}, segmentDump(e))
}

func TestRichTextEntry_MarkdownMode_NoFalseMatch(t *testing.T) {
	e := NewRichTextEntry()
	e.TypeMarkdown = true

	typeString(e, "2 * 3 * 4")

	assert.Equal(t, "2 * 3 * 4", e.Text)
	assert.Equal(t, []string{"2 * 3 * 4|"}, segmentDump(e))
}

func TestRichTextEntry_Markdown(t *testing.T) {
	source := "# Title\n\nSome **bold** and *italic* text."
	e := NewRichTextEntryFromMarkdown(source)

	assert.Equal(t, source, e.Markdown())
}

func TestRichTextEntry_Undo(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("a **b** c")
	e.CursorRow, e.CursorColumn = 0, 3

	e.TypedRune('X')
	assert.Equal(t, "a bX c", e.Text)

	e.Undo()
	assert.Equal(t, "a b c", e.Text)
	assert.Equal(t, []string{"a |", "b|b", " c|"}, segmentDump(e))

	e.Redo()
	assert.Equal(t, "a bX c", e.Text)
	assert.Equal(t, []string{"a |", "bX|b", " c|"}, segmentDump(e))
}

func TestRichTextEntry_Selection(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("a **b** c")
	e.syncSelectable()
	e.sel.selecting = true
	e.sel.selectRow, e.sel.selectColumn = 0, 0
	e.CursorRow, e.CursorColumn = 0, 4
	e.syncSelectable()

	assert.Equal(t, "a b ", e.SelectedText())
}

func TestRichTextEntry_Renders(t *testing.T) {
	test.NewTempApp(t)

	e := NewRichTextEntryFromMarkdown("# Title\n\nBody **text**")
	w := test.NewTempWindow(t, e)
	w.Resize(fyne.NewSize(200, 100))

	e.FocusGained()
	e.TypedRune('!')
	assert.Equal(t, "!Title\nBody text", e.Text)
}

func TestRichTextEntry_MouseRowAcrossHeights(t *testing.T) {
	test.NewTempApp(t)

	e := NewRichTextEntryFromMarkdown("# Heading\n\nBody\n\nMore")
	w := test.NewTempWindow(t, e)
	w.Resize(fyne.NewSize(300, 200))

	provider := e.textProvider()
	e.syncSelectable()
	innerPad := e.Theme().Size(theme.SizeNameInnerPadding)
	lineSpace := e.Theme().Size(theme.SizeNameLineSpacing)

	for row := 0; row < provider.rows(); row++ {
		y, height := provider.rowGeometry(row)
		mid := y + height/2 + innerPad - lineSpace
		got, _ := e.sel.getRowCol(fyne.NewPos(1, mid))
		assert.Equal(t, row, got, "row %d at y %v", row, mid)
	}
}

func TestRichTextEntry_MouseColumnUsesSegmentSize(t *testing.T) {
	test.NewTempApp(t)

	e := NewRichTextEntryFromMarkdown("# Heading")
	w := test.NewTempWindow(t, e)
	w.Resize(fyne.NewSize(300, 200))
	e.syncSelectable()

	th := e.Theme()
	provider := e.textProvider()
	// clicking just past the fourth character of the large heading text
	x := provider.lineSizeToColumn(4, 0, th.Size(theme.SizeNameText), th.Size(theme.SizeNameInnerPadding)).Width
	_, col := e.sel.getRowCol(fyne.NewPos(x+1, 2))
	assert.Equal(t, 4, col)
}

func TestRichTextEntry_ReturnLeavesHeading(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("# Title")
	e.CursorRow, e.CursorColumn = 0, 5

	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	typeString(e, "body")

	assert.Equal(t, "Title\nbody", e.Text)
	assert.Equal(t, []string{"Title\n|bh1", "body|"}, segmentDump(e))
}

func TestRichTextEntry_ReturnKeepsInlineStyle(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("**bold**")
	e.CursorRow, e.CursorColumn = 0, 4

	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	typeString(e, "more")

	assert.Equal(t, "bold\nmore", e.Text)
	assert.Equal(t, []string{"bold\nmore|b"}, segmentDump(e))
}

func TestRichTextEntry_Wrapping(t *testing.T) {
	test.NewTempApp(t)

	e := NewRichTextEntry()
	e.SetText("one two three four five six seven eight nine ten")
	w := test.NewTempWindow(t, e)
	w.Resize(fyne.NewSize(120, 200))
	e.Refresh()

	provider := e.textProvider()
	assert.Greater(t, provider.rows(), 1)

	// every row must map back to the offset that it starts at
	for row := 0; row < provider.rows(); row++ {
		start := textPosFromRowCol(row, 0, provider)
		gotRow, gotCol := e.rowColFromTextPos(start)
		assert.Equal(t, 0, gotCol, "row %d", row)
		assert.Equal(t, row, gotRow, "row %d", row)
	}
}

func TestRichTextEntry_AppendMarkdown(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("first")
	e.AppendMarkdown("\n\n**second**")

	assert.Equal(t, "first\nsecond", e.Text)
	assert.Equal(t, []string{"first\n|", "second|b"}, segmentDump(e))
}

func TestRichTextEntry_MarkdownRoundTrip(t *testing.T) {
	source := "# Title\n\nSome **bold** text\n\n## Second\n\nWith `code` and *emphasis*"
	e := NewRichTextEntryFromMarkdown(source)
	again := NewRichTextEntryFromMarkdown(e.Markdown())

	assert.Equal(t, e.Text, again.Text)
	assert.Equal(t, segmentDump(e), segmentDump(again))
}

func TestRichTextEntry_Disabled(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("**bold** text")
	e.Disable()
	e.Refresh()

	for _, seg := range e.Segments() {
		text := seg.(*TextSegment)
		assert.Equal(t, theme.ColorNameDisabled, text.Style.ColorName)
	}
	assert.True(t, e.Segments()[0].(*TextSegment).Style.TextStyle.Bold)

	e.Enable()
	e.Refresh()
	assert.Equal(t, theme.ColorNameForeground, e.Segments()[0].(*TextSegment).Style.ColorName)
}

func TestRichTextEntry_QuoteIndent(t *testing.T) {
	test.NewTempApp(t)

	e := NewRichTextEntryFromMarkdown("body\n\n> quoted\n\nafter")
	w := test.NewTempWindow(t, e)
	w.Resize(fyne.NewSize(300, 200))
	e.Refresh()

	th := e.Theme()
	provider := e.textProvider()
	textSize, innerPad := th.Size(theme.SizeNameText), th.Size(theme.SizeNameInnerPadding)
	x := func(row int) float32 {
		return provider.lineSizeToColumn(0, row, textSize, innerPad).Width
	}

	// the quote is indented, the lines either side of it are not
	assert.Equal(t, x(0), x(2), "the line after a quote must not inherit its indent")
	assert.Greater(t, x(1), x(0), "a quoted line is indented")
}

func TestRichTextEntry_DeleteWordAcrossRows(t *testing.T) {
	e := NewRichTextEntryFromMarkdown("# Title\n\nalpha beta gamma")
	e.CursorRow, e.CursorColumn = e.rowColFromTextPos(e.richProvider().len())
	e.syncSelectable()

	e.deleteWord(false)

	assert.Equal(t, "Title\nalpha beta ", e.Text)
}
