package whiteboard

import (
	"image"
	"image/color"
	"testing"

	"github.com/gowtham472/handoffproof/internal/agent"
)

// checker draws a 1-pixel black/white checkerboard: any pixelation of it
// averages to a flat gray.
func checker(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.White)
			} else {
				img.Set(x, y, color.Black)
			}
		}
	}
	return img
}

func TestPixelateOnlyTouchesBoxes(t *testing.T) {
	img := checker(64, 64)
	out := Pixelate(img, []agent.Box{{X1: 0, Y1: 0, X2: 32, Y2: 32}}, 8)
	inside := out.RGBAAt(3, 4)
	if inside.R == 0 || inside.R == 255 {
		t.Errorf("pixel inside box = %v, want averaged gray", inside)
	}
	if out.RGBAAt(40, 40) != img.RGBAAt(40, 40) {
		t.Error("pixel outside every box changed")
	}
	if img.RGBAAt(3, 4).R != 0 && img.RGBAAt(3, 4).R != 255 {
		t.Error("source image was mutated")
	}
}

func TestCropAndPrepare(t *testing.T) {
	img := checker(100, 80)
	for _, tc := range []struct {
		name    string
		box     agent.Box
		w, h    int
		wantErr bool
	}{
		{"padded inside", agent.Box{X1: 20, Y1: 20, X2: 40, Y2: 30}, 36, 26, false},
		{"clipped at edge", agent.Box{X1: 90, Y1: 70, X2: 120, Y2: 100}, 18, 18, false},
		{"outside image", agent.Box{X1: 300, Y1: 300, X2: 310, Y2: 310}, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := Crop(img, tc.box, 8)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tc.wantErr {
				return
			}
			c, err := Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if c.Bounds().Dx() != tc.w || c.Bounds().Dy() != tc.h {
				t.Errorf("crop = %v, want %dx%d", c.Bounds(), tc.w, tc.h)
			}
		})
	}
	notes := make([]agent.Box, MaxCrops+3)
	for i := range notes {
		notes[i] = agent.Box{ObjectID: i, X1: 1, Y1: 1, X2: 10, Y2: 10}
	}
	p, err := Prepare(img, notes, []agent.Box{{X1: 0, Y1: 0, X2: 20, Y2: 20}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Crops) != MaxCrops || len(p.Full) == 0 {
		t.Errorf("prepared %d crops, full %d bytes", len(p.Crops), len(p.Full))
	}
	// The note sits inside the screen box, so its crop must be pixelated.
	c, _ := Decode(p.Crops[0])
	r, _, _, _ := c.At(4, 4).RGBA()
	if r == 0 || r == 0xffff {
		t.Error("note crop overlapping a screen was not pixelated")
	}
}
