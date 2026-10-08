"""Draws the Sortwise icon (web/public/logo.svg) as PNGs for the browser
extension and the README. Run: python scripts/make-icons.py (needs Pillow)."""

from pathlib import Path

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parent.parent
SCALE = 8  # draw large, then downsample for smooth edges


def lerp(a, b, t):
    return tuple(round(x + (y - x) * t) for x, y in zip(a, b))


def draw(size):
    s = 64 * SCALE
    k = SCALE
    tile = Image.new("RGBA", (s, s))
    top, bottom = (0x6B, 0x5C, 0xF0), (0x3B, 0x2A, 0x9E)
    pixels = tile.load()
    for y in range(s):
        for x in range(s):
            pixels[x, y] = lerp(top, bottom, (x + y) / (2 * s)) + (255,)
    mask = Image.new("L", (s, s), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, s - 1, s - 1), radius=15 * k, fill=255)
    image = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    image.paste(tile, (0, 0), mask)

    # Three bars, longest to shortest: a list already in order.
    bars = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    d = ImageDraw.Draw(bars)
    for y, width, alpha in ((16, 36, 255), (28, 26, 209), (40, 16, 163)):
        d.rounded_rectangle((14 * k, y * k, (14 + width) * k, (y + 8) * k), radius=4 * k, fill=(255, 255, 255, alpha))
    d.ellipse((39 * k, 39 * k, 49 * k, 49 * k), fill=(0xFF, 0xC9, 0x4D, 255))
    image = Image.alpha_composite(image, bars)
    return image.resize((size, size), Image.LANCZOS)


def main():
    icons = ROOT / "extension" / "icons"
    icons.mkdir(exist_ok=True)
    for size in (16, 32, 48, 128):
        draw(size).save(icons / f"icon-{size}.png")
    (ROOT / "docs").mkdir(exist_ok=True)
    draw(256).save(ROOT / "docs" / "logo.png")
    # The tray icon and the Windows program icon.
    draw(256).save(ROOT / "cmd" / "sortwise" / "icon.ico", sizes=[(16, 16), (20, 20), (24, 24), (32, 32), (40, 40), (48, 48), (64, 64), (256, 256)])
    print("icons written")


if __name__ == "__main__":
    main()
