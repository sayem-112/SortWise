//go:build amd64 || 386

package flyout

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"
)

func readIcon(t *testing.T) []byte {
	t.Helper()
	ico, err := os.ReadFile(filepath.Join("..", "..", "cmd", "sortwise", "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	return ico
}

// TestPanelIsDrawn draws the panel at two scales and checks it has a solid
// body, rounded see-through corners, and a shadow below it. Set
// SORTWISE_FLYOUT_PNG to a folder to save the pictures.
func TestPanelIsDrawn(t *testing.T) {
	if err := startGDIPlus(); err != nil {
		t.Skip("GDI+ is not available:", err)
	}
	ico := readIcon(t)
	for _, scale := range []float32{1, 1.5} {
		l := arrange(true)
		l.update = true
		p, err := newPainter(scale, l, ico)
		if err != nil {
			t.Fatal(err)
		}
		pixels := make([]byte, p.width*p.height*4)
		state := view{
			status:  Status{Headline: "Organizing 12 new posts", Tone: Busy, Detail: "2,547 bookmarks on this PC · 8 saved today", Update: "0.9.0"},
			version: "v0.8.0", startup: true, hover: settingsButton, focus: updateLink, showFocus: true,
		}
		started := time.Now()
		if err := p.paint(unsafe.Pointer(&pixels[0]), state); err != nil {
			t.Fatal(err)
		}
		t.Logf("drawn at %.0f%% in %v", scale*100, time.Since(started))
		alpha := func(x, y float32) byte { return pixels[(int(y)*int(p.width)+int(x))*4+3] }
		panel := p.panel()
		if alpha(panel.x+panel.w/2, panel.y+panel.h/2) != 255 {
			t.Fatal("the panel is see-through")
		}
		if alpha(panel.x+0.5, panel.y+0.5) > 128 {
			t.Fatal("the corner is not rounded")
		}
		if alpha(1, 1) != 0 {
			t.Fatal("the window's corner is not clear")
		}
		if below := alpha(panel.x+panel.w/2, panel.y+panel.h+4*scale); below < 20 || below == 255 {
			t.Fatalf("no soft shadow below the panel: %d", below)
		}
		if folder := os.Getenv("SORTWISE_FLYOUT_PNG"); folder != "" {
			save(t, filepath.Join(folder, "flyout-"+map[float32]string{1: "100", 1.5: "150"}[scale]+".png"), pixels, int(p.width), int(p.height))
		}
		p.close()
	}
}

// save writes the premultiplied BGRA buffer as a PNG over a desktop-like
// grey, so the shadow shows.
func save(t *testing.T, path string, pixels []byte, width, height int) {
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for index := 0; index < len(pixels); index += 4 {
		under := 255 - int(pixels[index+3])
		backdrop := 0x3a * under / 255
		picture.Pix[index] = byte(int(pixels[index+2]) + backdrop)
		picture.Pix[index+1] = byte(int(pixels[index+1]) + backdrop)
		picture.Pix[index+2] = byte(int(pixels[index]) + backdrop)
		picture.Pix[index+3] = 255
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, picture); err != nil {
		t.Fatal(err)
	}
}

func TestIconPictureIsPickedBySize(t *testing.T) {
	ico := readIcon(t)
	for size, want := range map[int]int{20: 20, 30: 32, 25: 32, 40: 40, 300: 256} {
		picture := iconPicture(ico, size)
		if picture == nil || picture.Bounds().Dx() != want {
			t.Fatalf("for %d px got %v, want %d", size, picture, want)
		}
	}
	if iconPicture([]byte("nope"), 20) != nil {
		t.Fatal("read a picture from nothing")
	}
}
