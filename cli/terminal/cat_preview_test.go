package terminal

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Optional review artifacts from the exact pixels rendered by the terminal.
// Run with VIBECODE_PREVIEW_DIR set; ordinary tests do not write artifacts.
func TestCatPreviewArtifacts(t *testing.T) {
	dir := os.Getenv("VIBECODE_PREVIEW_DIR")
	if dir == "" {
		t.Skip("optional pixel-art preview")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	palette := color.Palette{color.RGBA{16, 21, 29, 255}}
	for state := postureRelaxed; state <= postureTense; state++ {
		for _, ch := range []byte{'#', 'o', 'w', 'd', '!'} {
			c := catInk(state, ch)
			palette = append(palette, color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 255})
		}
	}
	drawCat := func(img draw.Image, view postureView, x, y, scale int) {
		pixels := catFrame(view)
		for yy, row := range pixels {
			for xx, ch := range row {
				if ch == 0 {
					continue
				}
				c := catInk(view.state, ch)
				ink := image.NewUniform(color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 255})
				draw.Draw(img, image.Rect(x+xx*scale, y+yy*scale, x+(xx+1)*scale, y+(yy+1)*scale), ink, image.Point{}, draw.Src)
			}
		}
	}
	contact := image.NewRGBA(image.Rect(0, 0, 760, 400))
	draw.Draw(contact, contact.Bounds(), image.NewUniform(palette[0]), image.Point{}, draw.Src)
	for i, state := range []postureState{postureRelaxed, postureUpright, postureTense} {
		ticks := 16 - i*4
		for action := 0; action < 5; action++ {
			drawCat(contact, postureView{state: state, motion: uint8(action*ticks + ticks/2)}, 10+action*150, 10+i*130, 8)
		}
	}
	f, err := os.Create(filepath.Join(dir, "cat-poses.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, contact); err != nil {
		t.Fatal(err)
	}
	f.Close()
	animation := gif.GIF{LoopCount: 0}
	for step := 0; step < 240; step++ {
		frame := image.NewPaletted(image.Rect(0, 0, 600, 150), palette)
		for i, state := range []postureState{postureRelaxed, postureUpright, postureTense} {
			cycle := 80 - i*20
			if state == postureTense {
				cycle = 40
			}
			drawCat(frame, postureView{state: state, motion: uint8(step % cycle), blink: step%40 >= 38}, 10+i*200, 15, 10)
		}
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 10)
	}
	f, err = os.Create(filepath.Join(dir, "cat-motion.gif"))
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(f, &animation); err != nil {
		t.Fatal(err)
	}
	f.Close()
}
