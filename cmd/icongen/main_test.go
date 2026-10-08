package main

import (
	"image/color"
	"testing"
)

func TestGlyphBoxIsSquareAndInsideUnitSquare(t *testing.T) {
	x0, y0, x1, y1 := glyphBox()
	if x1-x0 != y1-y0 {
		t.Errorf("glyph box is not square: %v x %v", x1-x0, y1-y0)
	}
	if x0 < 0 || y0 < 0 || x1 > 1 || y1 > 1 {
		t.Errorf("glyph box escapes the unit square: (%v, %v)-(%v, %v)", x0, y0, x1, y1)
	}
}

func TestRenderGlyphCoverage(t *testing.T) {
	x0, y0, x1, y1 := glyphBox()
	const size = 64
	img := render(size, x0, y0, x1, y1)

	points := []struct {
		name string
		ux   float64 // unit square coordinate
		uy   float64
		op   bool
	}{
		{"bowl stroke", bowlCX + (bowlROut+bowlRIn)/2, bowlCY, true},
		{"counter", bowlCX, bowlCY, false},
		{"tail", (tailX0+tailX1)/2, (tailY0+tailY1)/2, true},
		{"corner margin", x0 + 0.03*(x1-x0), y0 + 0.03*(y1-y0), false},
	}
	for _, p := range points {
		px := int((p.ux - x0) / (x1 - x0) * size)
		py := int((p.uy - y0) / (y1 - y0) * size)
		got := img.NRGBAAt(px, py)
		filled := got.A == 255
		if filled != p.op {
			t.Errorf("%s at (%v, %v): alpha = %d, want filled = %v", p.name, px, py, got.A, p.op)
		}
		if filled && got != (color.NRGBA{R: letterGreen.R, G: letterGreen.G, B: letterGreen.B, A: 255}) {
			t.Errorf("%s: colour = %v, want %v", p.name, got, letterGreen)
		}
	}
}

func TestIconsetFileNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range iconset {
		if seen[f.name] {
			t.Errorf("duplicate iconset file %s", f.name)
		}
		seen[f.name] = true
		if f.size < 16 || f.size > 1024 {
			t.Errorf("iconset file %s has out of range size %d", f.name, f.size)
		}
	}
}
