// Package flyout is the small panel that opens above the tray icon: what
// Sortwise is doing, a button to open the library, starting with Windows,
// and quitting.
//
// It is drawn with Windows' own drawing calls inside the Sortwise process,
// not a web view, so it adds no processes and next to no memory. Nothing
// runs while it is closed; while it is open it checks the library every
// two seconds.
package flyout

// Tone colors the dot beside the headline.
type Tone int

const (
	Good    Tone = iota // up to date
	Busy                // organizing new posts
	Warning             // needs something from you
	Quiet               // paused or starting
)

// Status is what Sortwise is doing, in two short lines.
type Status struct {
	Headline string
	Tone     Tone
	Detail   string
}

// Options connect the panel to the running app. Its functions are called
// on the panel's own thread, so they should return quickly.
type Options struct {
	// Icon is the app icon, an .ico file.
	Icon    []byte
	Version string
	// Status is read when the panel opens, and every two seconds while open.
	Status       func() Status
	Open         func()
	OpenSettings func()
	Quit         func()
	// Startup reports whether Sortwise starts with Windows. Nil hides the
	// switch, for a copy that can't be set up.
	Startup    func() bool
	SetStartup func(on bool) error
}

// target is something in the panel that can be clicked.
type target int

const (
	nothing target = iota
	openButton
	startupSwitch
	settingsButton
	quitButton
)

type rect struct{ x, y, w, h float32 }

func (r rect) contains(x, y float32) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

// layout is where everything sits, in pixels at 100% scale.
type layout struct {
	width, height float32
	icon, title   rect
	version       rect
	dot           rect
	headline      rect
	detail        rect
	open          rect
	dividers      []float32
	// startupRow is the whole clickable row; zero when the switch is hidden.
	startupRow, startupLabel, startupSwitch rect
	settings, quit                          rect
}

const (
	panelWidth = 296
	pad        = 16
	// corner is the panel's corner radius.
	corner = 10
	// shadow is how far the shadow reaches past the panel.
	shadow = 22
)

func arrange(withStartup bool) layout {
	l := layout{width: panelWidth}
	inner := float32(panelWidth - 2*pad)
	l.icon = rect{pad, pad, 20, 20}
	l.title = rect{pad + 28, pad, 150, 20}
	l.version = rect{panelWidth - pad - 110, pad, 110, 20}
	l.headline = rect{pad + 16, 52, inner - 16, 20}
	l.dot = rect{pad, l.headline.y + 6, 8, 8}
	l.detail = rect{pad + 16, 72, inner - 16, 18}
	l.open = rect{pad, 104, inner, 34}
	y := l.open.y + l.open.h + pad
	l.dividers = append(l.dividers, y)
	if withStartup {
		l.startupRow = rect{6, y + 5, panelWidth - 12, 36}
		middle := l.startupRow.y + l.startupRow.h/2
		l.startupLabel = rect{pad, middle - 10, inner - 48, 20}
		l.startupSwitch = rect{panelWidth - pad - 32, middle - 9, 32, 18}
		y += 46
		l.dividers = append(l.dividers, y)
	}
	l.settings = rect{8, y + 8, 76, 28}
	l.quit = rect{panelWidth - 8 - 56, y + 8, 56, 28}
	l.height = y + 44
	return l
}

func (l layout) withStartup() bool { return l.startupRow.w > 0 }

// hit is what is under the point (x, y), in the panel's own pixels at 100%.
func (l layout) hit(x, y float32) target {
	switch {
	case l.open.contains(x, y):
		return openButton
	case l.withStartup() && l.startupRow.contains(x, y):
		return startupSwitch
	case l.settings.contains(x, y):
		return settingsButton
	case l.quit.contains(x, y):
		return quitButton
	}
	return nothing
}

// order is the order Tab moves through the controls.
func (l layout) order() []target {
	if l.withStartup() {
		return []target{openButton, startupSwitch, settingsButton, quitButton}
	}
	return []target{openButton, settingsButton, quitButton}
}

// next is the control after current in Tab order, or before it when back.
func (l layout) next(current target, back bool) target {
	order := l.order()
	at := -1
	for index, item := range order {
		if item == current {
			at = index
		}
	}
	switch {
	case at < 0 && back:
		return order[len(order)-1]
	case at < 0:
		return order[0]
	case back:
		return order[(at-1+len(order))%len(order)]
	}
	return order[(at+1)%len(order)]
}

// Edge is the side of the screen the taskbar is on.
type edge int

const (
	edgeLeft edge = iota
	edgeTop
	edgeRight
	edgeBottom
)

type box struct{ left, top, right, bottom int32 }

// place returns where the panel's top-left corner goes, in screen pixels:
// against the taskbar, as close to the cursor as the screen allows, gap
// pixels from the taskbar and the screen's edges. area is the screen
// without the taskbar; bar is the taskbar, which an auto-hiding taskbar
// overlaps area with.
func place(cursorX, cursorY int32, area, bar box, side edge, width, height, gap int32) (int32, int32) {
	clamp := func(value, low, high int32) int32 {
		if value > high {
			value = high
		}
		if value < low {
			value = low
		}
		return value
	}
	x := clamp(cursorX-width/2, area.left+gap, area.right-width-gap)
	y := clamp(cursorY-height/2, area.top+gap, area.bottom-height-gap)
	switch side {
	case edgeBottom:
		y = min(area.bottom, bar.top) - height - gap
	case edgeTop:
		y = max(area.top, bar.bottom) + gap
	case edgeLeft:
		x = max(area.left, bar.right) + gap
	case edgeRight:
		x = min(area.right, bar.left) - width - gap
	}
	return x, y
}
