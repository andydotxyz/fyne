package widget

import (
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Declare conformity with interfaces
var (
	_ fyne.Widget      = (*RichTextEntry)(nil)
	_ fyne.Focusable   = (*RichTextEntry)(nil)
	_ fyne.Disableable = (*RichTextEntry)(nil)
)

// RichTextEntry widget allows styled text to be edited when focused.
// It behaves like an [Entry] - but the content is held as a list of [RichTextSegment],
// so different runs of text can use their own style and objects can appear inline.
//
// The content can be set from markdown using [RichTextEntry.ParseMarkdown] or [NewRichTextEntryFromMarkdown].
// Setting [RichTextEntry.TypeMarkdown] additionally converts markdown into the matching style as it is typed.
//
// Since: 2.9
type RichTextEntry struct {
	Entry

	// TypeMarkdown converts markdown into the style that it describes as soon as it has been typed.
	TypeMarkdown bool
}

// NewRichTextEntry creates a new rich text entry widget.
// The returned entry accepts multiple lines and wraps on word boundaries.
//
// Since: 2.9
func NewRichTextEntry() *RichTextEntry {
	e := &RichTextEntry{}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	e.ExtendBaseWidget(e)
	return e
}

// ExtendBaseWidget is used by an extending widget to make use of BaseWidget functionality.
func (e *RichTextEntry) ExtendBaseWidget(wid fyne.Widget) {
	e.richProvider()
	e.Entry.ExtendBaseWidget(wid)
}

// NewRichTextEntryFromMarkdown creates a new rich text entry widget with the
// content described by the specified markdown.
//
// Since: 2.9
func NewRichTextEntryFromMarkdown(content string) *RichTextEntry {
	e := NewRichTextEntry()
	e.ParseMarkdown(content)
	return e
}

// Segments returns the styled segments that make up the content of this entry.
// The result should not be modified directly, use [RichTextEntry.SetSegments].
//
// Since: 2.9
func (e *RichTextEntry) Segments() []RichTextSegment {
	return e.richProvider().Segments
}

// SetSegments replaces the content of this entry with the specified segments.
// Segments that cannot be edited in place, such as lists or code blocks, are
// converted to styled text so that the whole content remains editable.
//
// Since: 2.9
func (e *RichTextEntry) SetSegments(segments []RichTextSegment) {
	provider := e.richProvider()
	provider.Segments = editableSegments(segments)

	e.CursorRow, e.CursorColumn = 0, 0
	e.ClearSelection()
	e.undoStack.Clear()
	e.updateTextAndRefresh(provider.String(), false)
	e.updateCursorAndSelection()
}

// SetText replaces the content of this entry with unstyled text.
//
// Since: 2.9
func (e *RichTextEntry) SetText(text string) {
	e.SetSegments([]RichTextSegment{&TextSegment{Style: RichTextStyleInline, Text: text}})
}

// ParseMarkdown replaces the content of this entry with the result of parsing
// the specified markdown.
//
// Since: 2.9
func (e *RichTextEntry) ParseMarkdown(content string) {
	e.SetSegments(parseMarkdown(content))
}

// AppendMarkdown parses the specified markdown and adds the result to the end of
// the current content.
//
// Since: 2.9
func (e *RichTextEntry) AppendMarkdown(content string) {
	provider := e.richProvider()
	segments := provider.Segments
	if current := provider.String(); current != "" && !strings.HasSuffix(current, "\n") {
		segments = append(segments, &TextSegment{Style: RichTextStyleInline, Text: "\n"})
	}
	segments = append(segments, editableSegments(parseMarkdown(content))...)

	pos := e.CursorTextOffset()
	provider.Segments = mergeSegments(segments)
	e.updateTextAndRefresh(provider.String(), false)
	e.setCursorOffset(pos)
}

// SetStyleForRange applies the specified style to the text between the two rune
// offsets, splitting segments where required. Positions outside the content are
// clamped and a start at or after the end does nothing.
//
// Since: 2.9
func (e *RichTextEntry) SetStyleForRange(start, end int, style RichTextStyle) {
	style.Inline = true
	e.styleRange(start, end, func(s *RichTextStyle) {
		*s = style
	})

	pos := e.CursorTextOffset()
	provider := e.richProvider()
	provider.Segments = mergeSegments(provider.Segments)
	e.Refresh()
	e.setCursorOffset(pos)
}

// SetStyleForSelection applies the specified style to the currently selected
// text. It does nothing if there is no selection.
//
// Since: 2.9
func (e *RichTextEntry) SetStyleForSelection(style RichTextStyle) {
	e.syncSelectable()
	start, end := e.sel.selection()
	if start < 0 || start == end {
		return
	}

	e.SetStyleForRange(start, end, style)
}

// StyleAtCursor returns the style that newly typed text will use.
// Any current selection is ignored.
//
// Since: 2.9
func (e *RichTextEntry) StyleAtCursor() RichTextStyle {
	pos := e.CursorTextOffset()
	style := RichTextStyleInline

	off := 0
	for _, seg := range e.richProvider().Segments {
		length := utf8.RuneCountInString(seg.Textual())
		text, isText := seg.(*TextSegment)
		if isText && ((off < pos && pos <= off+length) || (off == pos && length == 0)) {
			style = text.Style
		}
		if off > pos {
			break
		}
		off += length
	}
	return style
}

// TypedRune receives text input events when this widget is focused.
func (e *RichTextEntry) TypedRune(r rune) {
	e.Entry.TypedRune(r)

	if e.TypeMarkdown && e.styleMarkdownAtCursor(r) {
		e.Refresh()
	}
}

// TypedKey receives key input events when this widget is focused.
func (e *RichTextEntry) TypedKey(key *fyne.KeyEvent) {
	e.Entry.TypedKey(key)

	switch key.Name {
	case fyne.KeyBackspace, fyne.KeyDelete:
		if e.pruneEmptySegments() {
			e.Refresh()
		}
	case fyne.KeyReturn, fyne.KeyEnter:
		if e.MultiLine && isBlockStyle(e.StyleAtCursor()) {
			// headings and quotes cover a single line, so the next one starts
			// again from the base style
			e.insertEmptySegmentAt(e.CursorTextOffset(), RichTextStyleInline)
			e.Refresh()
		}
	}
}

// isBlockStyle reports whether a style applies to a whole line, rather than to a
// run of text within one.
func isBlockStyle(style RichTextStyle) bool {
	if style.QuotingDepth > 0 {
		return true
	}
	return style.SizeName != "" && style.SizeName != theme.SizeNameText
}

// Undo un-does the last modifying user-action.
//
// Since: 2.9
func (e *RichTextEntry) Undo() {
	_, action := e.undoStack.Undo(e.Text)
	e.applyUndoAction(action, true)
}

// Redo re-applies the last undone user-action.
//
// Since: 2.9
func (e *RichTextEntry) Redo() {
	_, action := e.undoStack.Redo(e.Text)
	e.applyUndoAction(action, false)
}

// applyUndoAction runs an undo stack entry against the segments to retain style.
func (e *RichTextEntry) applyUndoAction(action entryUndoAction, undo bool) {
	modify, ok := action.(*entryModifyAction)
	if !ok {
		return
	}

	provider := e.richProvider()
	pos := modify.Position
	if modify.Delete == undo { // put the text back
		provider.insertAt(pos, modify.Text)
		pos += len(modify.Text)
	} else {
		provider.deleteFromTo(pos, pos+len(modify.Text))
	}

	e.pruneEmptySegments()
	content := provider.String()
	e.updateText(content, false)
	e.setCursorOffset(pos)

	if e.OnChanged != nil {
		e.OnChanged(content)
	}
	e.validate()
	e.Refresh()
}

// setCursorOffset moves the cursor to the specified rune offset in the content.
func (e *RichTextEntry) setCursorOffset(pos int) {
	e.CursorRow, e.CursorColumn = e.rowColFromTextPos(pos)
	e.syncSelectable()
}

// richProvider returns the rich text that holds the content of this entry,
// setting up the first segment if it has not been prepared yet.
func (e *RichTextEntry) richProvider() *RichText {
	if !e.rich {
		e.rich = true
		e.initTextProvider()
		e.text.Segments = []RichTextSegment{&TextSegment{Style: RichTextStyleInline, Text: e.Text}}
	}
	return &e.text
}

// splitAt divides segments as required so that the specified rune offset falls
// on a segment boundary, returning the index of the segment starting there.
func (e *RichTextEntry) splitAt(pos int) int {
	provider := e.richProvider()

	off := 0
	for i := 0; i < len(provider.Segments); i++ {
		seg := provider.Segments[i]
		length := utf8.RuneCountInString(seg.Textual())
		if pos == off {
			return i
		}
		if pos < off+length {
			text, ok := seg.(*TextSegment)
			if !ok {
				return i + 1 // objects cannot be divided
			}

			runes := []rune(text.Text)
			cut := pos - off
			tail := &TextSegment{Style: text.Style, Text: string(runes[cut:])}
			text.Text = string(runes[:cut])

			segments := make([]RichTextSegment, 0, len(provider.Segments)+1)
			segments = append(segments, provider.Segments[:i+1]...)
			segments = append(segments, tail)
			segments = append(segments, provider.Segments[i+1:]...)
			provider.Segments = segments
			return i + 1
		}
		off += length
	}
	return len(provider.Segments)
}

// styleRange updates the style of every text segment between the two rune
// offsets, dividing segments at the boundaries where required.
func (e *RichTextEntry) styleRange(start, end int, apply func(*RichTextStyle)) {
	provider := e.richProvider()
	start = max(start, 0)
	end = min(end, provider.len())
	if start >= end {
		return
	}

	e.splitAt(start)
	e.splitAt(end)

	off := 0
	for _, seg := range provider.Segments {
		length := utf8.RuneCountInString(seg.Textual())
		if length > 0 && off >= start && off+length <= end {
			if text, ok := seg.(*TextSegment); ok {
				apply(&text.Style)
			}
		}
		off += length
	}
}

// insertEmptySegmentAt places an empty segment with the given style at the rune offset.
func (e *RichTextEntry) insertEmptySegmentAt(pos int, style RichTextStyle) {
	style.Inline = true
	provider := e.richProvider()
	e.dropEmptySegmentsAt(pos) // only one segment may claim the text typed here
	i := e.splitAt(pos)

	segments := make([]RichTextSegment, 0, len(provider.Segments)+1)
	segments = append(segments, provider.Segments[:i]...)
	segments = append(segments, &TextSegment{Style: style})
	segments = append(segments, provider.Segments[i:]...)
	provider.Segments = segments
}

// dropEmptySegmentsAt removes any empty text segments sitting at the given rune
// offset, so that a replacement can take ownership of text typed there.
func (e *RichTextEntry) dropEmptySegmentsAt(pos int) {
	provider := e.richProvider()

	segments := make([]RichTextSegment, 0, len(provider.Segments))
	off := 0
	for _, seg := range provider.Segments {
		length := utf8.RuneCountInString(seg.Textual())
		if _, isText := seg.(*TextSegment); isText && length == 0 && off == pos {
			continue
		}

		segments = append(segments, seg)
		off += length
	}
	provider.Segments = segments
}

// pruneEmptySegments drops empty text segments, apart from any at the cursor -
// where one may have been placed to hold a style that is about to be typed.
// Returns true if any segment was removed.
func (e *RichTextEntry) pruneEmptySegments() bool {
	provider := e.richProvider()
	cursor := e.CursorTextOffset()

	segments := make([]RichTextSegment, 0, len(provider.Segments))
	off := 0
	for _, seg := range provider.Segments {
		length := utf8.RuneCountInString(seg.Textual())
		if _, isText := seg.(*TextSegment); isText && length == 0 && off != cursor {
			continue
		}

		segments = append(segments, seg)
		off += length
	}

	if len(segments) == 0 {
		segments = append(segments, &TextSegment{Style: RichTextStyleInline})
	}

	removed := len(segments) != len(provider.Segments)
	provider.Segments = segments
	return removed
}

// mergeSegments joins neighbouring text segments that share a style, keeping the
// segment list as short as the content allows.
func mergeSegments(in []RichTextSegment) []RichTextSegment {
	out := make([]RichTextSegment, 0, len(in))
	for _, seg := range in {
		text, isText := seg.(*TextSegment)
		if !isText {
			out = append(out, seg)
			continue
		}

		if len(out) > 0 {
			if prev, ok := out[len(out)-1].(*TextSegment); ok && prev.Style == text.Style {
				prev.Text += text.Text
				continue
			}
		}
		if text.Text == "" && len(in) > 1 {
			continue
		}
		out = append(out, &TextSegment{Style: text.Style, Text: text.Text})
	}

	if len(out) == 0 {
		out = append(out, &TextSegment{Style: RichTextStyleInline})
	}
	return out
}

// editableSegments converts parsed rich text into a form where every character
// can be edited: a flat run of inline segments, with block boundaries expressed
// as newline characters instead of block styling.
func editableSegments(in []RichTextSegment) []RichTextSegment {
	return mergeSegments(trimTrailingNewline(flattenSegments(in, nil)))
}

// trimTrailingNewline removes the line break that closes the final block, as
// there is no following content for it to separate.
func trimTrailingNewline(segments []RichTextSegment) []RichTextSegment {
	for i := len(segments) - 1; i >= 0; i-- {
		text, ok := segments[i].(*TextSegment)
		if !ok {
			return segments
		}
		if text.Text == "" {
			continue
		}

		text.Text = strings.TrimSuffix(text.Text, "\n")
		return segments
	}
	return segments
}

// endsWithNewline reports whether the flattened content already ends with a
// line break, so that nested blocks do not each add one of their own.
func endsWithNewline(segments []RichTextSegment) bool {
	for i := len(segments) - 1; i >= 0; i-- {
		content := segments[i].Textual()
		if content == "" {
			continue
		}
		return strings.HasSuffix(content, "\n")
	}
	return false
}

func flattenSegments(in []RichTextSegment, out []RichTextSegment) []RichTextSegment {
	appendText := func(style RichTextStyle, text string) {
		style.Inline = true
		out = append(out, &TextSegment{Style: style, Text: text})
	}
	newline := func() {
		if len(out) == 0 || endsWithNewline(out) {
			return // nothing to break away from, or a break is already there
		}
		appendText(RichTextStyleInline, "\n")
	}

	for _, seg := range in {
		switch t := seg.(type) {
		case *TextSegment:
			if t.Text != "" {
				appendText(t.Style, t.Text)
			}
			if !t.Style.Inline {
				newline()
			}
		case *HyperlinkSegment:
			out = append(out, t)
		case *ImageSegment:
			out = append(out, t)
			newline()
		case *SeparatorSegment:
			newline()
			appendText(RichTextStyleInline, "---")
			newline()
		case *CodeBlockSegment:
			style := RichTextStyleCodeBlock
			style.QuotingDepth = t.quotingLevel
			appendText(style, t.Text)
			newline()
		case *CheckBoxSegment:
			marker := "- [ ] "
			if t.Checked {
				marker = "- [x] "
			}
			appendText(RichTextStyleInline, marker+t.Text)
			newline()
		case *ListSegment:
			for _, item := range t.Segments() {
				out = flattenSegments([]RichTextSegment{item}, out)
			}
		case *TableSegment:
			out = flattenTable(t, out)
		case *ParagraphSegment:
			out = flattenSegments(t.Texts, out)
			newline()
		default:
			if block, ok := seg.(RichTextBlock); ok {
				out = flattenSegments(block.Segments(), out)
				if !seg.Inline() {
					newline()
				}
				continue
			}
			out = append(out, seg)
		}
	}
	return out
}

func flattenTable(t *TableSegment, out []RichTextSegment) []RichTextSegment {
	row := func(cells [][]RichTextSegment, style RichTextStyle) {
		line := &strings.Builder{}
		line.WriteString("| ")
		for i, cell := range cells {
			if i > 0 {
				line.WriteString(" | ")
			}
			for _, seg := range cell {
				line.WriteString(seg.Textual())
			}
		}
		line.WriteString(" |\n")

		style.Inline = true
		out = append(out, &TextSegment{Style: style, Text: line.String()})
	}

	if len(t.Headers) > 0 {
		row(t.Headers, RichTextStyleStrong)
	}
	for _, r := range t.Rows {
		row(r, RichTextStyleInline)
	}
	return out
}

var markdownInlineStyles = []struct {
	marker string
	apply  func(*RichTextStyle)
}{
	{"~~", func(s *RichTextStyle) { s.TextStyle.Strikethrough = true }},
	{"**", func(s *RichTextStyle) { s.TextStyle.Bold = true }},
	{"__", func(s *RichTextStyle) { s.TextStyle.Bold = true }},
	{"`", func(s *RichTextStyle) { s.TextStyle.Monospace = true; s.codeInline = true }},
	{"*", func(s *RichTextStyle) { s.TextStyle.Italic = true }},
	{"_", func(s *RichTextStyle) { s.TextStyle.Italic = true }},
}

// styleMarkdownAtCursor looks for markdown that was completed by the rune just
// typed and, if it finds any, replaces it with the style that it describes.
// It reports whether the content was changed.
func (e *RichTextEntry) styleMarkdownAtCursor(typed rune) bool {
	pos := e.CursorTextOffset()
	runes := []rune(e.Text)
	if pos > len(runes) {
		return false
	}

	lineStart := 0
	for i := pos - 1; i >= 0; i-- {
		if runes[i] == '\n' {
			lineStart = i + 1
			break
		}
	}

	if typed == ' ' {
		return e.styleMarkdownBlock(runes, lineStart, pos)
	}
	return e.styleMarkdownInline(runes, lineStart, pos)
}

func (e *RichTextEntry) styleMarkdownInline(runes []rune, lineStart, pos int) bool {
	line := runes[lineStart:pos]

	for _, m := range markdownInlineStyles {
		marker := []rune(m.marker)
		if len(line) <= len(marker) || string(line[len(line)-len(marker):]) != m.marker {
			continue
		}

		body := line[:len(line)-len(marker)]
		open := lastIndexRunes(body, marker)
		if open < 0 {
			continue
		}

		content := body[open+len(marker):]
		if len(content) == 0 || strings.TrimSpace(string(content)) == "" ||
			content[0] == ' ' || content[len(content)-1] == ' ' {
			continue
		}
		if len(marker) == 1 && m.marker != "`" {
			// a single marker must not be half of a "**" or "__" pair
			if open > 0 && body[open-1] == marker[0] {
				continue
			}
			if content[len(content)-1] == marker[0] {
				continue
			}
		}

		openPos := lineStart + open
		base := e.StyleAtCursor()
		provider := e.richProvider()

		provider.deleteFromTo(pos-len(marker), pos)
		provider.deleteFromTo(openPos, openPos+len(marker))

		end := openPos + len(content)
		e.styleRange(openPos, end, m.apply)
		provider.Segments = mergeSegments(provider.Segments)

		// text typed after the styled run returns to the style around it
		e.insertEmptySegmentAt(end, base)

		e.finishStyling(end)
		return true
	}
	return false
}

func (e *RichTextEntry) styleMarkdownBlock(runes []rune, lineStart, pos int) bool {
	style, ok := markdownBlockStyle(string(runes[lineStart : pos-1]))
	if !ok {
		return false
	}

	lineEnd := lineStart
	for lineEnd < len(runes) && runes[lineEnd] != '\n' {
		lineEnd++
	}
	lineEnd -= pos - lineStart // the marker and its trailing space are removed

	provider := e.richProvider()
	provider.deleteFromTo(lineStart, pos)

	if lineEnd > lineStart {
		e.styleRange(lineStart, lineEnd, func(s *RichTextStyle) { *s = style })
	}
	provider.Segments = mergeSegments(provider.Segments)

	// so that the rest of the line is typed in the new style
	e.insertEmptySegmentAt(lineStart, style)

	e.finishStyling(lineStart)
	return true
}

func markdownBlockStyle(prefix string) (RichTextStyle, bool) {
	quoting := 0
	for strings.HasPrefix(prefix, ">") {
		quoting++
		prefix = strings.TrimPrefix(strings.TrimPrefix(prefix, ">"), " ")
	}

	var style RichTextStyle
	switch prefix {
	case "":
		if quoting == 0 {
			return style, false
		}
		style = RichTextStyleBlockquote
	case "#":
		style = RichTextStyleHeading
	case "##":
		style = RichTextStyleSubHeading
	case "###":
		style = RichTextStyleStrong
	default:
		return style, false
	}

	style.Inline = true
	style.QuotingDepth = quoting
	if quoting > 0 {
		style.TextStyle.Italic = true
	}
	return style, true
}

// finishStyling updates the entry state after segments have been restyled.
func (e *RichTextEntry) finishStyling(cursor int) {
	provider := e.richProvider()

	content := provider.String()
	e.updateText(content, false)
	e.setCursorOffset(cursor)

	// the undo stack holds plain text, which can no longer describe this change
	e.undoStack.Clear()

	if e.OnChanged != nil {
		e.OnChanged(content)
	}
	e.validate()
}

func lastIndexRunes(haystack, needle []rune) int {
	for i := len(haystack) - len(needle); i >= 0; i-- {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return i
		}
	}
	return -1
}
