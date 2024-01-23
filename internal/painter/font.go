package painter

import (
	"bytes"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"

	"github.com/go-text/render"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/internal/cache"
	"fyne.io/fyne/v2/theme"
)

const (
	// DefaultTabWidth is the default width in spaces
	DefaultTabWidth = 4

	fontTabSpaceSize = 10
)

// CachedFontFace returns a Font face held in memory. These are loaded from the current theme.
func CachedFontFace(style fyne.TextStyle, fontDP float32, texScale float32) *FontCacheItem {
	val, ok := fontCache.Load(style)
	if !ok {
		var f1, f2 font.Face
		switch {
		case style.Monospace:
			f1 = loadMeasureFont(theme.TextMonospaceFont())
			f2 = loadMeasureFont(theme.DefaultTextMonospaceFont())
		case style.Bold:
			if style.Italic {
				f1 = loadMeasureFont(theme.TextBoldItalicFont())
				f2 = loadMeasureFont(theme.DefaultTextBoldItalicFont())
			} else {
				f1 = loadMeasureFont(theme.TextBoldFont())
				f2 = loadMeasureFont(theme.DefaultTextBoldFont())
			}
		case style.Italic:
			f1 = loadMeasureFont(theme.TextItalicFont())
			f2 = loadMeasureFont(theme.DefaultTextItalicFont())
		case style.Symbol:
			f1 = loadMeasureFont(theme.SymbolFont())
			f2 = loadMeasureFont(theme.DefaultSymbolFont())
		default:
			f1 = loadMeasureFont(theme.TextFont())
			f2 = loadMeasureFont(theme.DefaultTextFont())
		}

		if f1 == nil {
			f1 = f2
		}
		faces := []font.Face{f1, f2}
		if emoji := theme.DefaultEmojiFont(); emoji != nil {
			faces = append(faces, loadMeasureFont(emoji))
		}
		val = &FontCacheItem{Fonts: faces}
		fontCache.Store(style, val)
	}

	return val.(*FontCacheItem)
}

// ClearFontCache is used to remove cached fonts in the case that we wish to re-load Font faces
func ClearFontCache() {

	fontCache = &sync.Map{}
}

// DrawString draws a string into an image.
func DrawString(dst draw.Image, s string, color color.Color, f []font.Face, fontSize, scale float32, tabWidth int) {
	r := render.Renderer{
		FontSize: fontSize,
		PixScale: scale,
		Color:    color,
	}

	if s == " " {
		return
	}

	paint := func(seg shaping.Output, x, y float32) {
		r.DrawShapedRunAt(seg, dst, int(x), int(y))
	}
	processString(s, f, fontSize, scale, tabWidth, paint)
}

func processString(s string, f []font.Face, fontSize, scale float32, tabWidth int, cb func(shaping.Output, float32, float32)) (size fyne.Size, advance float32) {
	sh := &shaping.HarfbuzzShaper{}
	s = strings.ReplaceAll(s, "\r", "")
	rs := []rune(s)

	x := float32(0)
	height := float32(0)
	start := 0
	end := len(rs)
	for i, c := range rs {
		if c == '\t' {
			if i > start {
				inset, a := processSubString(rs[start:i], sh, f, fontSize, scale, x, cb)
				x = a
				height = inset.Height
			}

			spacew := scale * fontTabSpaceSize
			x = tabStop(spacew, x, tabWidth)
			start = i + 1
		}
	}

	if start < end {
		trail, a := processSubString(rs[start:], sh, f, fontSize, scale, x, cb)
		x = a
		height = trail.Height
	}
	return fyne.NewSize(x, height), x
}

func processSubString(str []rune, sh shaping.Shaper, f []font.Face, fontSize, scale float32, startX float32,
	cb func(run shaping.Output, x, y float32)) (size fyne.Size, advance float32) {
	in := shaping.Input{
		Text:     str,
		RunStart: 0,
		RunEnd:   len(str),
		Size:     fixed.I(int(fontSize * scale)),
	}
	seg := shaping.Segmenter{}
	runs := seg.Split(in, fixedFontmap(f))

	base := float32(0)
	line := make(shaping.Line, len(runs))
	for i, run := range runs {
		line[i] = sh.Shape(run)

		asc := fixed266ToFloat32(line[i].LineBounds.Ascent)
		if asc > base {
			base = asc
		}
	}

	advance = startX
	thickness := float32(0)
	for _, run := range line {
		sz, adv := processFontRun(run, float32ToFixed266(fontSize), base, &advance, cb)
		if sz.Height > thickness {
			thickness = sz.Height
		}

		if adv >= advance {
			advance = adv
		}
	}
	return fyne.NewSize(advance, thickness), advance
}

func loadMeasureFont(data fyne.Resource) font.Face {
	loaded, err := font.ParseTTF(bytes.NewReader(data.Content()))
	if err != nil {
		fyne.LogError("font load error", err)
		return nil
	}

	return loaded
}

// MeasureString returns how far dot would advance by drawing s with f.
// Tabs are translated into a dot location change.
func MeasureString(f []font.Face, s string, textSize float32, tabWidth int) (size fyne.Size, advance float32) {
	if s == " " {
		return // TODO - space currently has no measurable size, this needs to be fixed for consistency!
	}

	return processString(s, f, textSize, 1, tabWidth, func(shaping.Output, float32, float32) {})
}

// RenderedTextSize looks up how big a string would be if drawn on screen.
// It also returns the distance from top to the text baseline.
func RenderedTextSize(text string, fontSize float32, style fyne.TextStyle) (size fyne.Size, baseline float32) {
	size, base := cache.GetFontMetrics(text, fontSize, style)
	if base != 0 {
		return size, base
	}

	size, base = measureText(text, fontSize, style)
	cache.SetFontMetrics(text, fontSize, style, size, base)
	return size, base
}

func fixed266ToFloat32(i fixed.Int26_6) float32 {
	return float32(float64(i) / (1 << 6))
}

func float32ToFixed266(f float32) fixed.Int26_6 {
	return fixed.Int26_6(float64(f) * (1 << 6))
}

func measureText(text string, fontSize float32, style fyne.TextStyle) (fyne.Size, float32) {
	face := CachedFontFace(style, fontSize, 1)
	return MeasureString(face.Fonts, text, fontSize, style.TabWidth)
}

func tabStop(spacew, x float32, tabWidth int) float32 {
	if tabWidth <= 0 {
		tabWidth = DefaultTabWidth
	}

	tabw := spacew * float32(tabWidth)
	tabs, _ := math.Modf(float64((x + tabw) / tabw))
	return tabw * float32(tabs)
}

func processFontRun(run shaping.Output, textSize fixed.Int26_6, y float32, advance *float32,
	cb func(run shaping.Output, x, y float32)) (size fyne.Size, base float32) {

	if len(run.Glyphs) == 1 {
		if run.Glyphs[0].GlyphID == 0 {
			in := shaping.Input{
				Text:      []rune{0xfffd},
				RunStart:  0,
				RunEnd:    1,
				Direction: di.DirectionLTR,
				Face:      run.Face,
				Size:      textSize,
			}
			shaper := &shaping.HarfbuzzShaper{}
			out := shaper.Shape(in)
			y := fixed266ToFloat32(out.LineBounds.Ascent)

			cb(out, *advance, y)
			*advance = fixed266ToFloat32(out.Advance) + *advance

			return fyne.NewSize(*advance, fixed266ToFloat32(run.LineBounds.LineThickness())),
				fixed266ToFloat32(run.LineBounds.Ascent)
		}
	}

	cb(run, *advance, y)
	*advance = fixed266ToFloat32(run.Advance) + *advance
	return fyne.NewSize(*advance, fixed266ToFloat32(run.LineBounds.LineThickness())),
		fixed266ToFloat32(run.LineBounds.Ascent)
}

type FontCacheItem struct {
	Fonts []font.Face
}

var fontCache = &sync.Map{} // map[fyne.TextStyle]*FontCacheItem

type fixedFontmap []font.Face

func (ff fixedFontmap) ResolveFace(r rune) font.Face {
	for _, f := range ff {
		if _, has := f.NominalGlyph(r); has {
			return f
		}
	}
	return ff[0]
}
