//go:build amd64 || 386

package flyout

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	registerClassEx              = user32.NewProc("RegisterClassExW")
	createWindowEx               = user32.NewProc("CreateWindowExW")
	defWindowProc                = user32.NewProc("DefWindowProcW")
	destroyWindow                = user32.NewProc("DestroyWindow")
	showWindow                   = user32.NewProc("ShowWindow")
	setForegroundWindow          = user32.NewProc("SetForegroundWindow")
	getMessage                   = user32.NewProc("GetMessageW")
	translateMessage             = user32.NewProc("TranslateMessage")
	dispatchMessage              = user32.NewProc("DispatchMessageW")
	postMessage                  = user32.NewProc("PostMessageW")
	postQuitMessage              = user32.NewProc("PostQuitMessage")
	updateLayeredWindow          = user32.NewProc("UpdateLayeredWindow")
	setTimer                     = user32.NewProc("SetTimer")
	killTimer                    = user32.NewProc("KillTimer")
	getCursorPos                 = user32.NewProc("GetCursorPos")
	monitorFromPoint             = user32.NewProc("MonitorFromPoint")
	getMonitorInfo               = user32.NewProc("GetMonitorInfoW")
	loadCursor                   = user32.NewProc("LoadCursorW")
	setCursor                    = user32.NewProc("SetCursor")
	trackMouseEvent              = user32.NewProc("TrackMouseEvent")
	getKeyState                  = user32.NewProc("GetKeyState")
	getAsyncKeyState             = user32.NewProc("GetAsyncKeyState")
	setThreadDpiAwarenessContext = user32.NewProc("SetThreadDpiAwarenessContext")
	gdi32                        = windows.NewLazySystemDLL("gdi32.dll")
	createCompatibleDC           = gdi32.NewProc("CreateCompatibleDC")
	createDIBSection             = gdi32.NewProc("CreateDIBSection")
	selectObject                 = gdi32.NewProc("SelectObject")
	deleteObject                 = gdi32.NewProc("DeleteObject")
	deleteDC                     = gdi32.NewProc("DeleteDC")
	shcore                       = windows.NewLazySystemDLL("shcore.dll")
	getDpiForMonitor             = shcore.NewProc("GetDpiForMonitor")
	shell32                      = windows.NewLazySystemDLL("shell32.dll")
	shAppBarMessage              = shell32.NewProc("SHAppBarMessage")
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	getModuleHandle              = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy     = 0x0002
	wmActivate    = 0x0006
	wmClose       = 0x0010
	wmActivateApp = 0x001C
	wmSetCursor   = 0x0020
	wmKeyDown     = 0x0100
	wmTimer       = 0x0113
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmMouseLeave  = 0x02A3
	wmDpiChanged  = 0x02E0
	wmToggle      = 0x8000 + 1 // WM_APP + 1
	wmStatus      = 0x8000 + 2 // a new status has been read

	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExLayered     = 0x00080000
	swHide          = 0
	swShow          = 5
	ulwAlpha        = 2
	htClient        = 1
	idcArrow        = 32512
	idcHand         = 32649
	tmeLeave        = 0x2
	vkTab           = 0x09
	vkReturn        = 0x0D
	vkShift         = 0x10
	vkEscape        = 0x1B
	vkSpace         = 0x20
	refreshTimer    = 1
	fadeTimer       = 2
	perMonitorAware = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, -4

	// gap is the space between the panel and the taskbar, at 100%.
	gap = 10
	// fadeLength is how long the panel takes to appear.
	fadeLength = 110 * time.Millisecond
	// statusWait is the longest opening waits for a fresh status. Reading
	// one takes milliseconds, unless the library is busy writing; then the
	// panel opens with the last status and updates when the new one comes.
	statusWait = 100 * time.Millisecond
	// clickLength is the longest a click on the tray icon can take: one
	// that closed the panel is let through to the icon after it.
	clickLength = 3 * time.Second
)

type point struct{ x, y int32 }

type monitorInfo struct {
	size          uint32
	monitor, work box
	flags         uint32
}

type message struct {
	window         uintptr
	message        uint32
	wParam, lParam uintptr
	time           uint32
	point          point
	private        uint32
}

// Flyout is the tray panel. It runs on a thread of its own.
type Flyout struct {
	options Options
	window  uintptr
	done    chan struct{}

	// latest is the last status read; reads happen off the panel's thread.
	latest atomic.Pointer[Status]

	// The rest belong to the panel's thread.
	visible bool
	// reading is closed when the status being read arrives; nil when none is.
	reading chan struct{}
	// closedByIconClick is when and where a click on the taskbar closed the
	// panel. That click goes on to the tray icon and asks to open it again.
	closedByIconClick time.Time
	closedAt          point
	shownAt           time.Time
	origin            point // the window's top-left corner on screen
	painter           *painter
	surface           *surface
	state             view
	tracking          bool
	gdiplusOn         bool
}

// surface is the bitmap the window shows.
type surface struct {
	dc, bitmap, old uintptr
	pixels          unsafe.Pointer
}

var (
	// current is the one panel; Windows calls back into it by window.
	current     *Flyout
	windowProc  = syscall.NewCallback(handle)
	errNoWindow = errors.New("could not create the tray panel")
	registered  sync.Once
)

// Start creates the panel, hidden. It is shown by Toggle.
func Start(options Options) (*Flyout, error) {
	f := &Flyout{options: options, done: make(chan struct{})}
	f.state.version = options.Version
	created := make(chan error, 1)
	go f.run(created)
	if err := <-created; err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Flyout) run(created chan<- error) {
	runtime.LockOSThread()
	defer close(f.done)
	// Drawn at the screen's real resolution, so it stays sharp when Windows
	// scales the display.
	if setThreadDpiAwarenessContext.Find() == nil {
		setThreadDpiAwarenessContext.Call(perMonitorAware)
	}
	instance, _, _ := getModuleHandle.Call(0)
	className, _ := windows.UTF16PtrFromString("SortwiseFlyout")
	registered.Do(func() {
		arrow, _, _ := loadCursor.Call(0, idcArrow)
		class := struct {
			size, style             uint32
			proc                    uintptr
			classExtra, windowExtra int32
			instance, icon, cursor  uintptr
			background              uintptr
			menuName, className     *uint16
			smallIcon               uintptr
		}{proc: windowProc, instance: instance, cursor: arrow, className: className}
		class.size = uint32(unsafe.Sizeof(class))
		registerClassEx.Call(uintptr(unsafe.Pointer(&class)))
	})
	title, _ := windows.UTF16PtrFromString("Sortwise")
	window, _, _ := createWindowEx.Call(wsExLayered|wsExToolWindow|wsExTopmost, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 0, 0, 0, 0, instance, 0)
	if window == 0 {
		created <- errNoWindow
		return
	}
	f.window = window
	current = f
	created <- nil

	var msg message
	for {
		result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	f.release()
}

// Toggle opens the panel, or closes it if open. Any thread may call it.
func (f *Flyout) Toggle() {
	postMessage.Call(f.window, wmToggle, 0, 0)
}

// Close removes the panel for good and waits for its thread to end.
func (f *Flyout) Close() {
	postMessage.Call(f.window, wmClose, 0, 0)
	<-f.done
}

func handle(window, msg, wParam, lParam uintptr) uintptr {
	f := current
	if f == nil || window != f.window {
		result, _, _ := defWindowProc.Call(window, msg, wParam, lParam)
		return result
	}
	switch msg {
	case wmToggle:
		closing, at := f.closedByIconClick, f.closedAt
		f.closedByIconClick = time.Time{}
		switch {
		case f.visible:
			f.hide()
		case time.Since(closing) > clickLength || !near(cursorAt(), at):
			f.show()
		}
		return 0
	case wmActivate, wmActivateApp:
		lostFocus := (msg == wmActivate && wParam&0xFFFF == 0) || (msg == wmActivateApp && wParam == 0)
		if lostFocus && f.visible {
			f.hide()
			if clickingTaskbar() {
				f.closedByIconClick, f.closedAt = time.Now(), cursorAt()
			}
		}
		return 0
	case wmStatus:
		f.reading = nil
		if latest := f.latest.Load(); latest != nil && f.visible && *latest != f.state.status {
			f.setStatus(*latest)
			f.redraw()
		}
		return 0
	case wmTimer:
		switch wParam {
		case refreshTimer:
			f.readStatus()
		case fadeTimer:
			f.fade()
		}
		return 0
	case wmMouseMove:
		if !f.tracking {
			track := struct {
				size, flags uint32
				window      uintptr
				hoverTime   uint32
			}{flags: tmeLeave, window: window}
			track.size = uint32(unsafe.Sizeof(track))
			trackMouseEvent.Call(uintptr(unsafe.Pointer(&track)))
			f.tracking = true
		}
		f.setHover(f.hitAt(lParam))
		return 0
	case wmMouseLeave:
		f.tracking = false
		f.state.pressed = nothing
		f.setHover(nothing)
		return 0
	case wmLButtonDown:
		f.state.pressed = f.hitAt(lParam)
		f.state.showFocus = false
		f.redraw()
		return 0
	case wmLButtonUp:
		pressed := f.state.pressed
		f.state.pressed = nothing
		if pressed != nothing && pressed == f.hitAt(lParam) {
			f.activate(pressed)
		} else {
			f.redraw()
		}
		return 0
	case wmSetCursor:
		if lParam&0xFFFF == htClient {
			shape := uintptr(idcArrow)
			if f.state.hover != nothing {
				shape = idcHand
			}
			cursor, _, _ := loadCursor.Call(0, shape)
			setCursor.Call(cursor)
			return 1
		}
	case wmKeyDown:
		f.key(wParam)
		return 0
	case wmDpiChanged:
		return 0 // sized for its screen each time it opens
	case wmClose:
		destroyWindow.Call(window)
		return 0
	case wmDestroy:
		current = nil
		postQuitMessage.Call(0)
		return 0
	}
	result, _, _ := defWindowProc.Call(window, msg, wParam, lParam)
	return result
}

// hitAt is the control under the mouse position in lParam.
func (f *Flyout) hitAt(lParam uintptr) target {
	if f.painter == nil {
		return nothing
	}
	x := float32(int16(lParam & 0xFFFF))
	y := float32(int16(lParam >> 16 & 0xFFFF))
	p := f.painter
	return p.layout.hit((x-p.margin)/p.scale, (y-p.margin)/p.scale)
}

func (f *Flyout) setHover(hover target) {
	if hover != f.state.hover {
		f.state.hover = hover
		f.redraw()
	}
}

func (f *Flyout) key(code uintptr) {
	// A key pressed just before the panel closed can arrive after.
	if !f.visible || f.painter == nil {
		return
	}
	switch code {
	case vkEscape:
		f.hide()
	case vkTab:
		shift, _, _ := getKeyState.Call(vkShift)
		f.state.focus = f.painter.layout.next(f.state.focus, int16(shift) < 0)
		f.state.showFocus = true
		f.redraw()
	case vkReturn, vkSpace:
		if f.state.focus != nothing {
			f.activate(f.state.focus)
		}
	}
}

func (f *Flyout) activate(control target) {
	switch control {
	case openButton:
		f.hide()
		go f.options.Open()
	case settingsButton:
		f.hide()
		go f.options.OpenSettings()
	case updateLink:
		f.hide()
		if f.options.OpenUpdate != nil {
			go f.options.OpenUpdate()
		}
	case quitButton:
		f.hide()
		go f.options.Quit()
	case startupSwitch:
		if f.options.SetStartup != nil {
			_ = f.options.SetStartup(!f.state.startup)
			f.state.startup = f.options.Startup()
		}
		f.redraw()
	}
}

// show places the panel above the tray icon, on the screen the mouse is
// on, and fades it in.
func (f *Flyout) show() {
	var cursor point
	getCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	packed := uintptr(uint32(cursor.x)) | uintptr(uint32(cursor.y))<<32
	const nearest = 2
	monitor, _, _ := monitorFromPoint.Call(packed, nearest)
	info := monitorInfo{}
	info.size = uint32(unsafe.Sizeof(info))
	getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	scale := float32(1)
	var dpiX, dpiY uint32
	if getDpiForMonitor.Find() == nil {
		if result, _, _ := getDpiForMonitor.Call(monitor, 0, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY))); result == 0 && dpiX > 0 {
			scale = float32(dpiX) / 96
		}
	}

	select {
	case <-f.readStatus():
	case <-time.After(statusWait):
	}
	status := Status{Headline: "Checking…", Tone: Quiet}
	if latest := f.latest.Load(); latest != nil {
		status = *latest
	}
	withStartup := f.options.Startup != nil
	if withStartup {
		f.state.startup = f.options.Startup()
	}
	f.state.hover, f.state.pressed, f.state.focus, f.state.showFocus = nothing, nothing, nothing, false
	if !f.gdiplusOn {
		if startGDIPlus() != nil {
			return
		}
		f.gdiplusOn = true
	}
	l := arrange(withStartup)
	painter, err := newPainter(scale, l, f.options.Icon)
	if err != nil {
		return
	}
	f.painter = painter
	f.setStatus(status)
	if !f.makeSurface() {
		f.release()
		return
	}

	side, bar := taskbar(info.monitor)
	panelWidth := int32(l.width * scale)
	panelHeight := int32(l.height * scale)
	x, y := place(cursor.x, cursor.y, info.work, bar, side, panelWidth, panelHeight, int32(gap*scale))
	margin := int32(f.painter.margin)
	f.origin = point{x - margin, y - margin}

	f.visible = true
	f.shownAt = time.Now()
	f.present(0)
	showWindow.Call(f.window, swShow)
	setForegroundWindow.Call(f.window)
	setTimer.Call(f.window, fadeTimer, 10, 0)
	setTimer.Call(f.window, refreshTimer, 2000, 0)
}

// setStatus shows status, with the update link when it offers one.
func (f *Flyout) setStatus(status Status) {
	f.state.status = status
	if f.painter != nil {
		f.painter.layout.update = status.Update != ""
		if !f.painter.layout.update && (f.state.focus == updateLink || f.state.hover == updateLink) {
			f.state.focus, f.state.hover = nothing, nothing
		}
	}
}

// readStatus reads the status off the panel's thread, so a busy library
// never freezes the panel, and posts wmStatus when it arrives. The channel
// closes then. Only one read runs at a time.
func (f *Flyout) readStatus() <-chan struct{} {
	if f.reading != nil {
		return f.reading
	}
	done := make(chan struct{})
	f.reading = done
	go func() {
		status := f.options.Status()
		f.latest.Store(&status)
		close(done)
		postMessage.Call(f.window, wmStatus, 0, 0)
	}()
	return done
}

func cursorAt() point {
	var cursor point
	getCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	return cursor
}

// near reports whether two cursor positions are the same click's: a few
// pixels apart at most.
func near(a, b point) bool {
	const slack = 6
	return a.x-b.x <= slack && b.x-a.x <= slack && a.y-b.y <= slack && b.y-a.y <= slack
}

// clickingTaskbar reports whether a mouse button is down over the taskbar,
// as when a click on the tray icon takes focus from the panel.
func clickingTaskbar() bool {
	const leftButton, rightButton = 0x01, 0x02
	left, _, _ := getAsyncKeyState.Call(leftButton)
	right, _, _ := getAsyncKeyState.Call(rightButton)
	if left&0x8000 == 0 && right&0x8000 == 0 {
		return false
	}
	cursor := cursorAt()
	_, bar := taskbar(box{-1 << 30, -1 << 30, 1 << 30, 1 << 30})
	return cursor.x >= bar.left && cursor.x < bar.right && cursor.y >= bar.top && cursor.y < bar.bottom
}

// fade steps the panel in: it rises a few pixels as it becomes opaque.
func (f *Flyout) fade() {
	progress := float64(time.Since(f.shownAt)) / float64(fadeLength)
	if progress >= 1 {
		killTimer.Call(f.window, fadeTimer)
		progress = 1
	}
	eased := 1 - (1-progress)*(1-progress)*(1-progress)
	f.present(eased)
}

// hide closes the panel and lets go of everything drawn for it: nothing
// runs and next to nothing is kept while it is closed.
func (f *Flyout) hide() {
	if !f.visible {
		return
	}
	f.visible = false
	killTimer.Call(f.window, refreshTimer)
	killTimer.Call(f.window, fadeTimer)
	showWindow.Call(f.window, swHide)
	f.release()
}

func (f *Flyout) release() {
	f.freeSurface()
	if f.painter != nil {
		f.painter.close()
		f.painter = nil
	}
}

func (f *Flyout) makeSurface() bool {
	p := f.painter
	header := struct {
		size                 uint32
		width, height        int32
		planes, bitCount     uint16
		compression, image   uint32
		xPerMeter, yPerMeter int32
		used, important      uint32
		colors               [4]byte
	}{width: p.width, height: -p.height, planes: 1, bitCount: 32}
	header.size = 40
	dc, _, _ := createCompatibleDC.Call(0)
	if dc == 0 {
		return false
	}
	var pixels unsafe.Pointer
	bitmap, _, _ := createDIBSection.Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 || pixels == nil {
		deleteDC.Call(dc)
		return false
	}
	old, _, _ := selectObject.Call(dc, bitmap)
	f.surface = &surface{dc: dc, bitmap: bitmap, old: old, pixels: pixels}
	return f.paint()
}

func (f *Flyout) freeSurface() {
	if f.surface == nil {
		return
	}
	selectObject.Call(f.surface.dc, f.surface.old)
	deleteObject.Call(f.surface.bitmap)
	deleteDC.Call(f.surface.dc)
	f.surface = nil
}

func (f *Flyout) paint() bool {
	return f.painter.paint(f.surface.pixels, f.state) == nil
}

// redraw paints the panel again and shows the result.
func (f *Flyout) redraw() {
	if !f.visible || f.surface == nil || !f.paint() {
		return
	}
	progress := float64(time.Since(f.shownAt)) / float64(fadeLength)
	f.present(min(progress, 1))
}

// present shows the bitmap at the given point of the fade, 0 to 1.
func (f *Flyout) present(progress float64) {
	if f.surface == nil {
		return
	}
	rise := int32((1 - progress) * 6 * float64(f.painter.scale))
	position := point{f.origin.x, f.origin.y + rise}
	size := point{f.painter.width, f.painter.height}
	source := point{}
	blend := [4]byte{0, 0, byte(255 * progress), 1} // over, flags, opacity, per-pixel alpha
	updateLayeredWindow.Call(f.window, 0, uintptr(unsafe.Pointer(&position)), uintptr(unsafe.Pointer(&size)), f.surface.dc, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
}

// taskbar returns which side of the screen the taskbar is on and where it
// is. A screen without it gets the bottom edge.
func taskbar(screen box) (edge, box) {
	data := struct {
		size     uint32
		window   uintptr
		callback uint32
		edge     uint32
		rect     box
		lParam   uintptr
	}{}
	data.size = uint32(unsafe.Sizeof(data))
	const getTaskbarPosition = 5
	if result, _, _ := shAppBarMessage.Call(getTaskbarPosition, uintptr(unsafe.Pointer(&data))); result == 0 {
		return edgeBottom, box{screen.left, screen.bottom, screen.right, screen.bottom}
	}
	bar := data.rect
	onScreen := bar.left < screen.right && bar.right > screen.left && bar.top < screen.bottom && bar.bottom > screen.top
	if !onScreen {
		return edgeBottom, box{screen.left, screen.bottom, screen.right, screen.bottom}
	}
	return edge(data.edge), bar
}
