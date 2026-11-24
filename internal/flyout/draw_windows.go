//go:build amd64 || 386

package flyout

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The panel is drawn with GDI+, which ships with Windows, into a bitmap the
// window shows with per-pixel transparency: rounded corners and a soft
// shadow over whatever is behind it.

var (
	gdiplus                     = windows.NewLazySystemDLL("gdiplus.dll")
	gdiplusStartup              = gdiplus.NewProc("GdiplusStartup")
	gdipCreateBitmapFromScan0   = gdiplus.NewProc("GdipCreateBitmapFromScan0")
	gdipGetImageGraphicsContext = gdiplus.NewProc("GdipGetImageGraphicsContext")
	gdipDeleteGraphics          = gdiplus.NewProc("GdipDeleteGraphics")
	gdipDisposeImage            = gdiplus.NewProc("GdipDisposeImage")
	gdipSetSmoothingMode        = gdiplus.NewProc("GdipSetSmoothingMode")
	gdipSetPixelOffsetMode      = gdiplus.NewProc("GdipSetPixelOffsetMode")
	gdipSetTextRenderingHint    = gdiplus.NewProc("GdipSetTextRenderingHint")
	gdipSetInterpolationMode    = gdiplus.NewProc("GdipSetInterpolationMode")
	gdipCreateSolidFill         = gdiplus.NewProc("GdipCreateSolidFill")
	gdipDeleteBrush             = gdiplus.NewProc("GdipDeleteBrush")
	gdipSetSolidFillColor       = gdiplus.NewProc("GdipSetSolidFillColor")
	gdipCreatePen1              = gdiplus.NewProc("GdipCreatePen1")
	gdipDeletePen               = gdiplus.NewProc("GdipDeletePen")
	gdipCreatePath              = gdiplus.NewProc("GdipCreatePath")
	gdipDeletePath              = gdiplus.NewProc("GdipDeletePath")
	gdipAddPathArc              = gdiplus.NewProc("GdipAddPathArc")
	gdipClosePathFigure         = gdiplus.NewProc("GdipClosePathFigure")
	gdipFillPath                = gdiplus.NewProc("GdipFillPath")
	gdipDrawPath                = gdiplus.NewProc("GdipDrawPath")
	gdipFillRectangle           = gdiplus.NewProc("GdipFillRectangle")
	gdipFillEllipse             = gdiplus.NewProc("GdipFillEllipse")
	gdipDrawImageRect           = gdiplus.NewProc("GdipDrawImageRect")
	gdipCreateFontFamilyByName  = gdiplus.NewProc("GdipCreateFontFamilyFromName")
	gdipDeleteFontFamily        = gdiplus.NewProc("GdipDeleteFontFamily")
	gdipCreateFont              = gdiplus.NewProc("GdipCreateFont")
	gdipDeleteFont              = gdiplus.NewProc("GdipDeleteFont")
	gdipCreateStringFormat      = gdiplus.NewProc("GdipCreateStringFormat")
	gdipDeleteStringFormat      = gdiplus.NewProc("GdipDeleteStringFormat")
	gdipSetStringFormatAlign    = gdiplus.NewProc("GdipSetStringFormatAlign")
	gdipSetStringFormatLine     = gdiplus.NewProc("GdipSetStringFormatLineAlign")
	gdipSetStringFormatTrimming = gdiplus.NewProc("GdipSetStringFormatTrimming")
	gdipDrawString              = gdiplus.NewProc("GdipDrawString")
)

const (
	pixelFormatPARGB         = 0xE200B // 32 bits, alpha premultiplied, as a layered window wants
	smoothingAntiAlias       = 4
	pixelOffsetHalf          = 4
	textAntiAliasGridFit     = 3
	interpolationHighBicubic = 7
	unitPixel                = 2
	fontRegular              = 0
	stringNoWrap             = 0x1000
	trimEllipsis             = 3
)

// Colors, as 0xAARRGGBB, matching the app's dark theme.
const (
	colorPanel       = 0xFF202020
	colorBorder      = 0x18FFFFFF
	colorDivider     = 0x18FFFFFF
	colorStrong      = 0xE6FFFFFF
	colorText        = 0xCFFFFFFF
	colorSecondary   = 0x80FFFFFF
	colorFaint       = 0x52FFFFFF
	colorHover       = 0x0EFFFFFF
	colorPressed     = 0x16FFFFFF
	colorAccent      = 0xFF1B73CF
	colorAccentHover = 0xFF155FAE
	colorFocus       = 0x8C2383E2
	colorSwitchOff   = 0x29FFFFFF
	colorWhite       = 0xFFFFFFFF
)

var toneColors = map[Tone]uint32{Good: 0xFF529E72, Busy: 0xFF5E87C9, Warning: 0xFFC77D48, Quiet: 0xFF9B9B9B}

// call runs a GDI+ function and turns its status into an error. Like the
// call it wraps, it keeps any pointer passed as a uintptr on the heap and
// alive until it returns, so Windows can write through it.
//
//go:uintptrescapes
func call(proc *windows.LazyProc, args ...uintptr) error {
	status, _, _ := proc.Call(args...)
	if status != 0 {
		return fmt.Errorf("%s failed with status %d", proc.Name, status)
	}
	return nil
}

// float passes a float to GDI+. Go's calls into Windows place the first four
// arguments in both kinds of register, so this works in any position.
func float(value float32) uintptr { return uintptr(math.Float32bits(value)) }

type rectF struct{ x, y, w, h float32 }

func startGDIPlus() error {
	input := struct {
		version          uint32
		debugCallback    uintptr
		noBackground     int32
		noExternalCodecs int32
	}{version: 1}
	var token uintptr
	return call(gdiplusStartup, uintptr(unsafe.Pointer(&token)), uintptr(unsafe.Pointer(&input)), 0)
}

// fonts are the panel's typefaces at one scale.
type fonts struct {
	scale                         float32
	title, body, small, smallBold uintptr
	families                      []uintptr
}

func newFonts(scale float32) (*fonts, error) {
	f := &fonts{scale: scale}
	family := func(name string) (uintptr, error) {
		text, _ := windows.UTF16PtrFromString(name)
		var handle uintptr
		err := call(gdipCreateFontFamilyByName, uintptr(unsafe.Pointer(text)), 0, uintptr(unsafe.Pointer(&handle)))
		if err == nil {
			f.families = append(f.families, handle)
		}
		return handle, err
	}
	regular, err := family("Segoe UI")
	if err != nil {
		return nil, err
	}
	semibold, err := family("Segoe UI Semibold")
	if err != nil {
		semibold = regular
	}
	font := func(family uintptr, size float32) uintptr {
		var handle uintptr
		_ = call(gdipCreateFont, family, float(size*scale), fontRegular, unitPixel, uintptr(unsafe.Pointer(&handle)))
		return handle
	}
	f.title = font(semibold, 14)
	f.body = font(regular, 13)
	f.small = font(regular, 12)
	f.smallBold = font(semibold, 13)
	return f, nil
}

func (f *fonts) close() {
	for _, font := range []uintptr{f.title, f.body, f.small, f.smallBold} {
		if font != 0 {
			gdipDeleteFont.Call(font)
		}
	}
	for _, family := range f.families {
		gdipDeleteFontFamily.Call(family)
	}
}

// canvas draws into a premultiplied 32-bit pixel buffer of the window's size.
type canvas struct {
	graphics, bitmap uintptr
	scale            float32
	// origin is where the panel's top-left corner is in the buffer.
	origin float32
	brush  uintptr
	format uintptr
}

func newCanvas(pixels unsafe.Pointer, width, height int32, scale, origin float32) (*canvas, error) {
	c := &canvas{scale: scale, origin: origin}
	if err := call(gdipCreateBitmapFromScan0, uintptr(width), uintptr(height), uintptr(width*4), pixelFormatPARGB, uintptr(pixels), uintptr(unsafe.Pointer(&c.bitmap))); err != nil {
		return nil, err
	}
	if err := call(gdipGetImageGraphicsContext, c.bitmap, uintptr(unsafe.Pointer(&c.graphics))); err != nil {
		gdipDisposeImage.Call(c.bitmap)
		return nil, err
	}
	gdipSetSmoothingMode.Call(c.graphics, smoothingAntiAlias)
	gdipSetPixelOffsetMode.Call(c.graphics, pixelOffsetHalf)
	gdipSetTextRenderingHint.Call(c.graphics, textAntiAliasGridFit)
	gdipSetInterpolationMode.Call(c.graphics, interpolationHighBicubic)
	_ = call(gdipCreateSolidFill, colorWhite, uintptr(unsafe.Pointer(&c.brush)))
	_ = call(gdipCreateStringFormat, stringNoWrap, 0, uintptr(unsafe.Pointer(&c.format)))
	gdipSetStringFormatTrimming.Call(c.format, trimEllipsis)
	return c, nil
}

func (c *canvas) close() {
	gdipDeleteStringFormat.Call(c.format)
	gdipDeleteBrush.Call(c.brush)
	gdipDeleteGraphics.Call(c.graphics)
	gdipDisposeImage.Call(c.bitmap)
}

// at turns a rectangle in the panel's 100% pixels into buffer pixels.
func (c *canvas) at(r rect) rectF {
	return rectF{c.origin + r.x*c.scale, c.origin + r.y*c.scale, r.w * c.scale, r.h * c.scale}
}

// solid is the canvas's one brush, set to color.
func (c *canvas) solid(color uint32) uintptr {
	gdipSetSolidFillColor.Call(c.brush, uintptr(color))
	return c.brush
}

// roundedPath outlines r (in buffer pixels) with corners of the given radius.
func roundedPath(r rectF, radius float32) uintptr {
	var path uintptr
	_ = call(gdipCreatePath, 0, uintptr(unsafe.Pointer(&path)))
	d := min(radius*2, r.w, r.h)
	gdipAddPathArc.Call(path, float(r.x), float(r.y), float(d), float(d), float(180), float(90))
	gdipAddPathArc.Call(path, float(r.x+r.w-d), float(r.y), float(d), float(d), float(270), float(90))
	gdipAddPathArc.Call(path, float(r.x+r.w-d), float(r.y+r.h-d), float(d), float(d), float(0), float(90))
	gdipAddPathArc.Call(path, float(r.x), float(r.y+r.h-d), float(d), float(d), float(90), float(90))
	gdipClosePathFigure.Call(path)
	return path
}

func (c *canvas) fillRounded(r rect, radius float32, color uint32) {
	if color == 0 {
		return
	}
	path := roundedPath(c.at(r), radius*c.scale)
	gdipFillPath.Call(c.graphics, c.solid(color), path)
	gdipDeletePath.Call(path)
}

func (c *canvas) strokeRounded(r rectF, radius, width float32, color uint32) {
	path := roundedPath(r, radius)
	var pen uintptr
	if call(gdipCreatePen1, uintptr(color), float(width), unitPixel, uintptr(unsafe.Pointer(&pen))) == nil {
		gdipDrawPath.Call(c.graphics, pen, path)
		gdipDeletePen.Call(pen)
	}
	gdipDeletePath.Call(path)
}

// focusRing outlines r the way the app marks the focused control.
func (c *canvas) focusRing(r rect, radius float32) {
	at := c.at(r)
	grow := 1.5 * c.scale
	c.strokeRounded(rectF{at.x - grow, at.y - grow, at.w + 2*grow, at.h + 2*grow}, (radius+1.5)*c.scale, 2*c.scale, colorFocus)
}

func (c *canvas) line(y float32, color uint32) {
	at := c.at(rect{0, y, panelWidth, 1})
	at.h = max(1, float32(math.Round(float64(c.scale))))
	gdipFillRectangle.Call(c.graphics, c.solid(color), float(at.x), float(float32(math.Round(float64(at.y)))), float(at.w), float(at.h))
}

func (c *canvas) circle(r rect, color uint32) {
	at := c.at(r)
	gdipFillEllipse.Call(c.graphics, c.solid(color), float(at.x), float(at.y), float(at.w), float(at.h))
}

type align int

const (
	alignStart align = iota
	alignCenter
	alignEnd
)

func (c *canvas) text(r rect, value string, font uintptr, color uint32, horizontal align) {
	if value == "" || font == 0 {
		return
	}
	gdipSetStringFormatAlign.Call(c.format, uintptr(horizontal))
	gdipSetStringFormatLine.Call(c.format, uintptr(alignCenter))
	text, _ := windows.UTF16FromString(value)
	at := c.at(r)
	gdipDrawString.Call(c.graphics, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1), font, uintptr(unsafe.Pointer(&at)), c.format, c.solid(color))
}

// image draws a picture held as a GDI+ bitmap into r.
func (c *canvas) image(picture uintptr, r rect) {
	if picture == 0 {
		return
	}
	at := c.at(r)
	gdipDrawImageRect.Call(c.graphics, picture, float(at.x), float(at.y), float(at.w), float(at.h))
}

// iconBitmap picks the icon's smallest picture at least size pixels wide,
// or its largest, and makes a GDI+ bitmap of it. pixels must stay alive as
// long as the bitmap does.
func iconBitmap(ico []byte, size int) (bitmap uintptr, pixels []byte) {
	picture := iconPicture(ico, size)
	if picture == nil {
		return 0, nil
	}
	bounds := picture.Bounds()
	rgba := image.NewRGBA(bounds) // premultiplied, as PARGB wants
	draw.Draw(rgba, bounds, picture, bounds.Min, draw.Src)
	pixels = make([]byte, len(rgba.Pix))
	for index := 0; index < len(pixels); index += 4 {
		pixels[index], pixels[index+1], pixels[index+2], pixels[index+3] = rgba.Pix[index+2], rgba.Pix[index+1], rgba.Pix[index], rgba.Pix[index+3]
	}
	width, height := bounds.Dx(), bounds.Dy()
	if call(gdipCreateBitmapFromScan0, uintptr(width), uintptr(height), uintptr(width*4), pixelFormatPARGB, uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bitmap))) != nil {
		return 0, nil
	}
	return bitmap, pixels
}

// iconPicture reads the PNG picture closest above size from an .ico file.
func iconPicture(ico []byte, size int) image.Image {
	if len(ico) < 6 {
		return nil
	}
	count := int(binary.LittleEndian.Uint16(ico[4:]))
	best, bestWidth := -1, 0
	for index := 0; index < count; index++ {
		entry := 6 + 16*index
		if entry+16 > len(ico) {
			break
		}
		width := int(ico[entry])
		if width == 0 {
			width = 256
		}
		better := best < 0 ||
			(width >= size && (bestWidth < size || width < bestWidth)) ||
			(width < size && bestWidth < size && width > bestWidth)
		if better {
			best, bestWidth = index, width
		}
	}
	if best < 0 {
		return nil
	}
	entry := 6 + 16*best
	length := int(binary.LittleEndian.Uint32(ico[entry+8:]))
	offset := int(binary.LittleEndian.Uint32(ico[entry+12:]))
	if offset < 0 || length < 0 || offset+length > len(ico) {
		return nil
	}
	picture, err := png.Decode(bytes.NewReader(ico[offset : offset+length]))
	if err != nil {
		return nil
	}
	return picture
}

// paintShadow fills a premultiplied buffer of the window's size with the
// panel's shadow: a close, darker one and a wide, soft one, both a little
// below the panel, as the app's menus have.
func paintShadow(pixels []byte, width, height int, panel rectF, radius, scale float32) {
	type layer struct{ offset, reach, strength float32 }
	layers := []layer{{2 * scale, 6 * scale, 0.32}, {8 * scale, 20 * scale, 0.38}}
	halfW, halfH := panel.w/2, panel.h/2
	centerX, centerY := panel.x+halfW, panel.y+halfH
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			clear := float32(1)
			for _, layer := range layers {
				// Distance from the panel's rounded outline, moved down.
				qx := float32(math.Abs(float64(float32(x)+0.5-centerX))) - (halfW - radius)
				qy := float32(math.Abs(float64(float32(y)+0.5-centerY-layer.offset))) - (halfH - radius)
				outside := float32(math.Hypot(float64(max(qx, 0)), float64(max(qy, 0))))
				distance := outside + min(max(qx, qy), 0) - radius
				fade := 1 - min(max((distance+layer.reach/2)/layer.reach, 0), 1)
				clear *= 1 - layer.strength*fade*fade
			}
			at := (y*width + x) * 4
			pixels[at], pixels[at+1], pixels[at+2], pixels[at+3] = 0, 0, 0, byte(255*(1-clear)+0.5)
		}
	}
}
