// Command mkicon makes the app icon's two masters from the source art
// (packaging/art/llama-swap-launcher.png), each 1024×1024 on Apple's macOS
// icon grid: the art scaled onto a rounded-square plate, 824×824 px, centred,
// corner radius 185 px.
//
//   - AppIcon.png: the whole picture, for 64 pt and up.
//   - AppIcon-small.png: just the centre llama, for 16 and 32 pt, where the
//     whole picture's nine heads are too small to make out.
//
// The plate matters on dark backgrounds too, where the art's navy arrows
// would otherwise disappear. Run it with `make icons`. It's its own module so
// its image library stays out of the app's dependencies.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/vector"
)

func main() {
	src := flag.String("src", "", "source art: a PNG with a transparent background")
	out := flag.String("out", ".", "directory for AppIcon.png and AppIcon-small.png")
	small := flag.String("small", "400,345,618,665", "the part of the art for the small sizes: x0,y0,x1,y1")
	flag.Parse()

	art, err := load(*src)
	if err != nil {
		fail(err)
	}
	var r image.Rectangle
	if _, err := fmt.Sscanf(*small, "%d,%d,%d,%d", &r.Min.X, &r.Min.Y, &r.Max.X, &r.Max.Y); err != nil {
		fail(fmt.Errorf("-small: %w", err))
	}

	whole := opaqueBounds(art)
	for _, icon := range []struct {
		name   string
		part   image.Rectangle
		height int // of the art on the plate, in px
	}{
		{"AppIcon.png", whole, 700},
		{"AppIcon-small.png", r, 720},
	} {
		if err := save(filepath.Join(*out, icon.name), plate(art, icon.part, icon.height)); err != nil {
			fail(err)
		}
	}
}

// plate draws part of art, scaled to height px tall, centred on the plate.
func plate(art image.Image, part image.Rectangle, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	r := vector.NewRasterizer(1024, 1024)
	roundRect(r, 100, 100, 924, 924, 185)
	r.Draw(dst, dst.Bounds(), image.NewUniform(color.RGBA{0xFB, 0xF8, 0xF3, 0xFF}), image.Point{})

	width := height * part.Dx() / part.Dy()
	at := image.Rect(512-width/2, 512-height/2, 512-width/2+width, 512-height/2+height)
	xdraw.CatmullRom.Scale(dst, at, art, part, draw.Over, nil)
	return dst
}

// roundRect adds a rounded rectangle to r, with each corner a quarter
// circle approximated by a cubic Bézier.
func roundRect(r *vector.Rasterizer, x0, y0, x1, y1, rad float32) {
	k := rad * 0.5523
	r.MoveTo(x0+rad, y0)
	r.LineTo(x1-rad, y0)
	r.CubeTo(x1-rad+k, y0, x1, y0+rad-k, x1, y0+rad)
	r.LineTo(x1, y1-rad)
	r.CubeTo(x1, y1-rad+k, x1-rad+k, y1, x1-rad, y1)
	r.LineTo(x0+rad, y1)
	r.CubeTo(x0+rad-k, y1, x0, y1-rad+k, x0, y1-rad)
	r.LineTo(x0, y0+rad)
	r.CubeTo(x0, y0+rad-k, x0+rad-k, y0, x0+rad, y0)
	r.ClosePath()
}

// opaqueBounds is the smallest rectangle holding every visible pixel.
func opaqueBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	r := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a>>8 > 16 {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

func load(path string) (image.Image, error) {
	f, err := os.Open(path) // #nosec G304 -- the art file named on the command line
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func save(path string, img image.Image) error {
	f, err := os.Create(path) // #nosec G304 -- an output file named on the command line
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close() // the encoding error is the one to report
		return err
	}
	return f.Close()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mkicon:", err)
	os.Exit(1)
}
