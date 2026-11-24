//go:build amd64 || 386

package flyout

import (
	"math"
	"unsafe"
)

// view is what the panel shows at one moment.
type view struct {
	status    Status
	version   string
	startup   bool
	hover     target
	pressed   target
	focus     target
	showFocus bool
}

// painter draws the panel at one scale into a buffer the size of the
// window: the panel plus room around it for the shadow.
type painter struct {
	scale         float32
	layout        layout
	fonts         *fonts
	icon          uintptr
	iconPixels    []byte
	width, height int32
	margin        float32
	shadow        []byte
}

func newPainter(scale float32, l layout, ico []byte) (*painter, error) {
	f, err := newFonts(scale)
	if err != nil {
		return nil, err
	}
	p := &painter{scale: scale, layout: l, fonts: f}
	p.margin = float32(math.Round(float64(shadow * scale)))
	p.width = int32(math.Ceil(float64(l.width*scale + 2*p.margin)))
	p.height = int32(math.Ceil(float64(l.height*scale + 2*p.margin)))
	p.icon, p.iconPixels = iconBitmap(ico, int(math.Round(float64(l.icon.w*scale))))
	p.shadow = make([]byte, p.width*p.height*4)
	paintShadow(p.shadow, int(p.width), int(p.height), p.panel(), corner*scale, scale)
	return p, nil
}

func (p *painter) close() {
	p.fonts.close()
	if p.icon != 0 {
		gdipDisposeImage.Call(p.icon)
	}
}

// panel is the panel's rectangle in buffer pixels.
func (p *painter) panel() rectF {
	return rectF{p.margin, p.margin, p.layout.width * p.scale, p.layout.height * p.scale}
}

// paint draws v into pixels, a premultiplied 32-bit buffer of p's size.
func (p *painter) paint(pixels unsafe.Pointer, v view) error {
	copy(unsafe.Slice((*byte)(pixels), len(p.shadow)), p.shadow)
	c, err := newCanvas(pixels, p.width, p.height, p.scale, p.margin)
	if err != nil {
		return err
	}
	defer c.close()
	if p.layout.update {
		p.layout.updateWidth = c.measure("Update to v"+v.status.Update, p.fonts.small)
	}
	l := p.layout
	panel := p.panel()
	c.fillRounded(rect{0, 0, l.width, l.height}, corner, colorPanel)
	c.strokeRounded(rectF{panel.x + 0.5, panel.y + 0.5, panel.w - 1, panel.h - 1}, corner*p.scale-0.5, 1, colorBorder)

	c.image(p.icon, l.icon)
	c.text(l.title, "Sortwise", p.fonts.title, colorStrong, alignStart)
	if l.update {
		link := uint32(colorLink)
		if v.hover == updateLink || v.pressed == updateLink {
			link = colorLinkHover
		}
		c.text(l.version, "Update to v"+v.status.Update, p.fonts.small, link, alignEnd)
	} else {
		c.text(l.version, v.version, p.fonts.small, colorFaint, alignEnd)
	}

	c.circle(l.dot, toneColors[v.status.Tone])
	c.text(l.headline, v.status.Headline, p.fonts.body, colorStrong, alignStart)
	c.text(l.detail, v.status.Detail, p.fonts.small, colorSecondary, alignStart)

	fill := uint32(colorAccent)
	if v.hover == openButton || v.pressed == openButton {
		fill = colorAccentHover
	}
	c.fillRounded(l.open, 6, fill)
	c.text(l.open, "Open Sortwise", p.fonts.smallBold, colorWhite, alignCenter)

	for _, y := range l.dividers {
		c.line(y, colorDivider)
	}

	if l.withStartup() {
		c.fillRounded(l.startupRow, 6, background(v, startupSwitch))
		c.text(l.startupLabel, "Start with Windows", p.fonts.body, colorText, alignStart)
		track := uint32(colorSwitchOff)
		thumb := l.startupSwitch.x + 2
		if v.startup {
			track, thumb = colorAccent, thumb+14
		}
		c.fillRounded(l.startupSwitch, 9, track)
		c.circle(rect{thumb, l.startupSwitch.y + 2, 14, 14}, colorWhite)
	}

	c.fillRounded(l.settings, 6, background(v, settingsButton))
	c.text(l.settings, "Settings", p.fonts.body, colorText, alignCenter)
	c.fillRounded(l.quit, 6, background(v, quitButton))
	c.text(l.quit, "Quit", p.fonts.body, colorText, alignCenter)

	if v.showFocus {
		switch v.focus {
		case openButton:
			c.focusRing(l.open, 6)
		case startupSwitch:
			c.focusRing(l.startupSwitch, 9)
		case settingsButton:
			c.focusRing(l.settings, 6)
		case quitButton:
			c.focusRing(l.quit, 6)
		case updateLink:
			c.focusRing(l.updateArea(), 4)
		}
	}
	return nil
}

// background is a quiet control's fill: clear, or lit under the mouse.
func background(v view, control target) uint32 {
	switch {
	case v.pressed == control:
		return colorPressed
	case v.hover == control:
		return colorHover
	}
	return 0
}
