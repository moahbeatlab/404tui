#!/usr/bin/env python3
"""Render a tmux capture (`tmux capture-pane -e -p`) to a PNG, one cell per character.

    python3 render.py pads.ansi pads.png [--cols 140 --rows 45]

Needs Pillow. Two fonts are used: a monospaced one for the text, and one with the Braille block
(U+2800-28FF) for the Slicer's waveform and the other dot graphs. A font without Braille draws every
dot cell as an empty box (the "tofu" glyph), which makes a waveform look like a solid wall. The script
stops with a message when it cannot find such a font, rather than writing that picture.
"""
import argparse, glob, os, re, sys
from PIL import Image, ImageDraw, ImageFont

TEXT = ["DejaVuSansMono.ttf", "NotoSansMono-Regular.ttf", "LiberationMono-Regular.ttf"]
TEXT_BOLD = ["DejaVuSansMono-Bold.ttf", "NotoSansMono-Bold.ttf", "LiberationMono-Bold.ttf"]
BRAILLE = ["FreeMono.ttf", "DejaVuSans.ttf", "NotoSansSymbols2-Regular.ttf"]
BG, FG = (18, 18, 22), (222, 222, 226)
ANSI16 = [(0, 0, 0), (205, 49, 49), (13, 188, 121), (229, 229, 16), (36, 114, 200), (188, 63, 188),
          (17, 168, 205), (229, 229, 229), (102, 102, 102), (241, 76, 76), (35, 209, 139),
          (245, 245, 67), (59, 142, 234), (214, 112, 214), (41, 184, 219), (255, 255, 255)]


def find(names):
    for root in ("/usr/share/fonts", "/usr/local/share/fonts", os.path.expanduser("~/.fonts"),
                 "/System/Library/Fonts", "/Library/Fonts"):
        for n in names:
            hit = glob.glob(os.path.join(root, "**", n), recursive=True)
            if hit:
                return hit[0]
    return None


def has_braille(path):
    """True if the font draws U+28FF as dots and U+2800 as nothing; a font without the block draws both as a box."""
    def ink(ch):
        img = Image.new("L", (40, 40), 0)
        ImageDraw.Draw(img).text((4, 4), ch, font=ImageFont.truetype(path, 24), fill=255)
        return img.getbbox()
    return ink("⠀") is None and ink("⣿") is not None


def c256(n):
    if n < 16:
        return ANSI16[n]
    if n < 232:
        n -= 16
        f = lambda v: 0 if v == 0 else 55 + 40 * v
        return (f(n // 36), f((n // 6) % 6), f(n % 6))
    v = 8 + 10 * (n - 232)
    return (v, v, v)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("src")
    ap.add_argument("out")
    ap.add_argument("--cols", type=int, default=140)
    ap.add_argument("--rows", type=int, default=45)
    ap.add_argument("--cell", default="10.4x20.98", help="cell size in pixels, WxH (140x45 cells -> 1456x944)")
    a = ap.parse_args()
    cw, ch = (float(v) for v in a.cell.split("x"))
    text, bold, braille = find(TEXT), find(TEXT_BOLD), find(BRAILLE)
    if not text or not braille or not has_braille(braille):
        sys.exit("render.py: needs a monospaced font (DejaVu Sans Mono) and one with the Braille block (FreeMono or DejaVu Sans)")
    ft, fb, fr = (ImageFont.truetype(p, s) for p, s in ((text, 17), (bold or text, 17), (braille, 18)))
    img = Image.new("RGB", (round(cw * a.cols), round(ch * a.rows)), BG)
    d = ImageDraw.Draw(img)
    fg = bg = None
    bd = rev = False
    for y, line in enumerate(open(a.src, encoding="utf8").read().split("\n")[: a.rows]):
        x = 0
        for m in re.finditer(r"\x1b\[([0-9;]*)m|(.)", line):
            if m.group(2) is None:
                p = [int(v) if v else 0 for v in m.group(1).split(";")]
                i = 0
                while i < len(p):
                    v = p[i]
                    if v == 0: fg = bg = None; bd = rev = False
                    elif v == 1: bd = True
                    elif v == 7: rev = True
                    elif v == 22: bd = False
                    elif v == 27: rev = False
                    elif 30 <= v <= 37: fg = c256(v - 30)
                    elif 90 <= v <= 97: fg = c256(v - 90 + 8)
                    elif 40 <= v <= 47: bg = c256(v - 40)
                    elif 100 <= v <= 107: bg = c256(v - 100 + 8)
                    elif v == 39: fg = None
                    elif v == 49: bg = None
                    elif v in (38, 48):
                        if p[i + 1] == 5: col = c256(p[i + 2]); i += 2
                        else: col = tuple(p[i + 2:i + 5]); i += 4
                        if v == 38: fg = col
                        else: bg = col
                    i += 1
                continue
            if x >= a.cols:
                continue
            c = m.group(2)
            f_, b_ = (fg or FG), (bg or BG)
            if rev: f_, b_ = b_, f_
            X, Y = x * cw, y * ch
            if b_ != BG: d.rectangle([X, Y, X + cw, Y + ch], fill=b_)
            if c != " ": d.text((X, Y), c, font=fr if "⠀" <= c <= "⣿" else (fb if bd else ft), fill=f_)
            x += 1
    img.save(a.out, optimize=True)


if __name__ == "__main__":
    main()
