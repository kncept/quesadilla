// Command icongen draws the Quesadilla app icon: an avocado-green letter Q.
//
// The icon is written as a set of PNGs, one per size macOS expects inside an
// .iconset directory, ready to be converted into an .icns file with:
//
//	iconutil -c icns <iconset-dir> -o Quesadilla.icns
//
// Usage: go run ./cmd/icongen <iconset-dir>
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// letterGreen is the avocado-green fill of the letter Q.
var letterGreen = color.NRGBA{R: 0x7e, G: 0xa1, B: 0x22, A: 0xff}

// samples is the supersampling factor per axis, used to antialias the glyph.
const samples = 4

// Glyph geometry, expressed within a unit square. The letter Q is an annulus
// (the bowl) plus a capsule (the tail) crossing it at the bottom right.
const (
	bowlCX   = 0.45
	bowlCY   = 0.44
	bowlROut = 0.30
	bowlRIn  = 0.16

	tailX0 = 0.53
	tailY0 = 0.53
	tailX1 = 0.78
	tailY1 = 0.79
	tailR  = 0.055
)

// glyphPad is the empty margin kept around the glyph, as a fraction of the
// glyph's own bounding box.
const glyphPad = 0.10

type iconFile struct {
	name string
	size int
}

// iconset is every file macOS expects in an .iconset directory. Sizes are
// rendered once per distinct pixel size, so a few PNGs are duplicated.
var iconset = []iconFile{
	{"icon_16x16.png", 16},
	{"icon_16x16@2x.png", 32},
	{"icon_32x32.png", 32},
	{"icon_32x32@2x.png", 64},
	{"icon_128x128.png", 128},
	{"icon_128x128@2x.png", 256},
	{"icon_256x256.png", 256},
	{"icon_256x256@2x.png", 512},
	{"icon_512x512.png", 512},
	{"icon_512x512@2x.png", 1024},
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: icongen <iconset-dir>")
		os.Exit(2)
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}

	x0, y0, x1, y1 := glyphBox()
	for _, f := range iconset {
		img := render(f.size, x0, y0, x1, y1)
		path := filepath.Join(dir, f.name)
		file, err := os.Create(path)
		if err != nil {
			fail(err)
		}
		if err := png.Encode(file, img); err != nil {
			file.Close()
			fail(err)
		}
		if err := file.Close(); err != nil {
			fail(err)
		}
	}
}

// glyphBox returns the square, padded bounding box of the glyph in unit
// square coordinates, so every icon size frames the letter identically.
func glyphBox() (x0, y0, x1, y1 float64) {
	x0, y0 = bowlCX-bowlROut, bowlCY-bowlROut
	x1, y1 = bowlCX+bowlROut, bowlCY+bowlROut
	x0 = math.Min(x0, math.Min(tailX0, tailX1)-tailR)
	y0 = math.Min(y0, math.Min(tailY0, tailY1)-tailR)
	x1 = math.Max(x1, math.Max(tailX0, tailX1)+tailR)
	y1 = math.Max(y1, math.Max(tailY0, tailY1)+tailR)

	// Square the box around its centre, keeping the glyph centred, then pad it.
	side := math.Max(x1-x0, y1-y0) * (1 + 2*glyphPad)
	cx := (x0 + x1) / 2
	cy := (y0 + y1) / 2
	return cx - side/2, cy - side/2, cx + side/2, cy + side/2
}

// render draws the glyph covering the given box of the unit square into a
// size x size image with a transparent background.
func render(size int, x0, y0, x1, y1 float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	w := x1 - x0
	h := y1 - y0
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			covered := 0
			for sy := 0; sy < samples; sy++ {
				fy := (float64(py) + (float64(sy)+0.5)/samples) / float64(size)
				for sx := 0; sx < samples; sx++ {
					fx := (float64(px) + (float64(sx)+0.5)/samples) / float64(size)
					if inGlyph(x0+fx*w, y0+fy*h) {
						covered++
					}
				}
			}
			img.SetNRGBA(px, py, color.NRGBA{
				R: letterGreen.R,
				G: letterGreen.G,
				B: letterGreen.B,
				A: uint8(covered * 255 / (samples * samples)),
			})
		}
	}
	return img
}

// inGlyph reports whether the point is covered by the bowl or the tail.
func inGlyph(x, y float64) bool {
	if d := math.Hypot(x-bowlCX, y-bowlCY); d <= bowlROut && d >= bowlRIn {
		return true
	}
	return pointToSegment(x, y, tailX0, tailY0, tailX1, tailY1) <= tailR
}

// pointToSegment returns the distance from (px, py) to the segment ends.
func pointToSegment(px, py, x0, y0, x1, y1 float64) float64 {
	dx := x1 - x0
	dy := y1 - y0
	lengthSq := dx*dx + dy*dy
	t := 0.0
	if lengthSq > 0 {
		t = ((px-x0)*dx + (py-y0)*dy) / lengthSq
		t = math.Max(0, math.Min(1, t))
	}
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "icongen:", err)
	os.Exit(1)
}
