// Package whiteboard prepares photos of off-repo handover knowledge
// (whiteboards, sticky notes, screens) for a vision model: screen regions
// found by segmentation are pixelated, and note regions are cropped so the
// model reads one note at a time. It never calls a model and never decides
// a verdict. Experimental: verified on a synthetic fixture only.
package whiteboard

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // decode JPEG photos
	"image/png"

	"github.com/gowtham472/handoffproof/internal/agent"
)

// MaxCrops caps how many note crops go to the vision model in one request
// (the API allows 50 images; a whiteboard rarely has more notes).
const MaxCrops = 12

// Decode reads a PNG or JPEG image.
func Decode(raw []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

// EncodePNG encodes img as PNG.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rect converts a box to an image rectangle grown by pad pixels and
// clipped to bounds.
func rect(b agent.Box, pad int, bounds image.Rectangle) image.Rectangle {
	return image.Rect(b.X1-pad, b.Y1-pad, b.X2+pad, b.Y2+pad).Intersect(bounds)
}

// Pixelate returns a copy of img with every box replaced by coarse blocks
// of block pixels, so text on screens cannot be read.
func Pixelate(img image.Image, boxes []agent.Box, block int) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	if block < 2 {
		block = 2
	}
	for _, b := range boxes {
		r := rect(b, 0, out.Bounds())
		for y := r.Min.Y; y < r.Max.Y; y += block {
			for x := r.Min.X; x < r.Max.X; x += block {
				cell := image.Rect(x, y, x+block, y+block).Intersect(r)
				var sr, sg, sb, n uint32
				for yy := cell.Min.Y; yy < cell.Max.Y; yy++ {
					for xx := cell.Min.X; xx < cell.Max.X; xx++ {
						c := out.RGBAAt(xx, yy)
						sr, sg, sb, n = sr+uint32(c.R), sg+uint32(c.G), sb+uint32(c.B), n+1
					}
				}
				if n == 0 {
					continue
				}
				avg := color.RGBA{R: uint8(sr / n), G: uint8(sg / n), B: uint8(sb / n), A: 255}
				draw.Draw(out, cell, &image.Uniform{C: avg}, image.Point{}, draw.Src)
			}
		}
	}
	return out
}

// Crop returns the box region (plus pad) of img as PNG bytes.
func Crop(img image.Image, b agent.Box, pad int) ([]byte, error) {
	r := rect(b, pad, img.Bounds())
	if r.Empty() {
		return nil, fmt.Errorf("box %d lies outside the image", b.ObjectID)
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return EncodePNG(out)
}

// Prepared is what the vision model may see.
type Prepared struct {
	Full  []byte   // whole image with screens pixelated
	Crops [][]byte // note crops taken from the pixelated image, in box order
}

// Prepare pixelates screens, then crops up to MaxCrops notes from the
// pixelated image so a note overlapping a screen stays unreadable there.
func Prepare(img image.Image, notes, screens []agent.Box) (Prepared, error) {
	safe := Pixelate(img, screens, 16)
	full, err := EncodePNG(safe)
	if err != nil {
		return Prepared{}, err
	}
	p := Prepared{Full: full}
	for i, b := range notes {
		if i >= MaxCrops {
			break
		}
		c, err := Crop(safe, b, 8)
		if err != nil {
			return Prepared{}, err
		}
		p.Crops = append(p.Crops, c)
	}
	return p, nil
}
