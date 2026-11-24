package flyout

import "testing"

func TestEveryControlCanBeClickedAndTabbedTo(t *testing.T) {
	for _, withStartup := range []bool{true, false} {
		l := arrange(withStartup)
		centre := func(r rect) (float32, float32) { return r.x + r.w/2, r.y + r.h/2 }
		controls := map[target]rect{openButton: l.open, settingsButton: l.settings, quitButton: l.quit}
		if withStartup {
			controls[startupSwitch] = l.startupSwitch
			// The whole row toggles it, label included.
			if x, y := centre(l.startupLabel); l.hit(x, y) != startupSwitch {
				t.Fatal("the switch's label does not toggle it")
			}
		}
		for control, area := range controls {
			if x, y := centre(area); l.hit(x, y) != control {
				t.Fatalf("clicking control %d hit %d", control, l.hit(x, y))
			}
			if area.y+area.h > l.height || area.x+area.w > l.width {
				t.Fatalf("control %d is off the panel", control)
			}
		}
		if x, y := centre(l.headline); l.hit(x, y) != nothing {
			t.Fatal("the status line is clickable")
		}
		// Tab visits each control once and comes back round; Shift+Tab goes back.
		seen := map[target]bool{}
		focus := nothing
		for range controls {
			focus = l.next(focus, false)
			seen[focus] = true
		}
		if len(seen) != len(controls) || l.next(focus, false) != openButton || l.next(openButton, true) != quitButton {
			t.Fatalf("tab order %v", seen)
		}
	}
	// An update link in the corner is clickable, and first in Tab order.
	l := arrange(true)
	if x, y := l.version.x+l.version.w-10, l.version.y+10; l.hit(x, y) != nothing {
		t.Fatal("the version number is clickable")
	}
	l.update = true
	if x, y := l.version.x+l.version.w-10, l.version.y+10; l.hit(x, y) != updateLink || l.next(nothing, false) != updateLink {
		t.Fatal("the update link can't be reached")
	}
	// Once drawn, only its text is clickable.
	l.updateWidth = 60
	if l.hit(l.version.x+5, l.version.y+10) != nothing || l.hit(l.version.x+l.version.w-30, l.version.y+10) != updateLink {
		t.Fatal("the update link's area is not its text")
	}
	if arrange(false).height >= arrange(true).height {
		t.Fatal("hiding the switch did not shorten the panel")
	}
}

func TestPlacedAgainstTheTaskbarNearTheCursor(t *testing.T) {
	screen := box{0, 0, 1920, 1080}
	// Taskbar at the bottom, icon near the right.
	x, y := place(1800, 1060, box{0, 0, 1920, 1040}, box{0, 1040, 1920, 1080}, edgeBottom, 300, 250, 10)
	if x != 1920-300-10 || y != 1040-250-10 {
		t.Fatalf("bottom: %d,%d", x, y)
	}
	// Icon in the middle: centred on it.
	if x, _ := place(900, 1060, box{0, 0, 1920, 1040}, box{0, 1040, 1920, 1080}, edgeBottom, 300, 250, 10); x != 750 {
		t.Fatalf("centred: %d", x)
	}
	// Auto-hiding taskbar: the work area is the whole screen, so the bar decides.
	if _, y := place(1800, 1070, screen, box{0, 1032, 1920, 1080}, edgeBottom, 300, 250, 10); y != 1032-250-10 {
		t.Fatalf("auto-hide: %d", y)
	}
	if x, y := place(1800, 20, box{0, 40, 1920, 1080}, box{0, 0, 1920, 40}, edgeTop, 300, 250, 10); y != 50 || x != 1610 {
		t.Fatalf("top: %d,%d", x, y)
	}
	if x, y := place(20, 1000, box{60, 0, 1920, 1080}, box{0, 0, 60, 1080}, edgeLeft, 300, 250, 10); x != 70 || y != 1080-250-10 {
		t.Fatalf("left: %d,%d", x, y)
	}
	if x, _ := place(1900, 500, box{0, 0, 1860, 1080}, box{1860, 0, 1920, 1080}, edgeRight, 300, 250, 10); x != 1860-300-10 {
		t.Fatalf("right: %d", x)
	}
	// A second screen to the left, at negative coordinates.
	if x, y := place(-100, 1060, box{-1920, 0, 0, 1040}, box{-1920, 1040, 0, 1080}, edgeBottom, 300, 250, 10); x != -310 || y != 780 {
		t.Fatalf("second screen: %d,%d", x, y)
	}
}
