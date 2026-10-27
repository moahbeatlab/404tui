#!/usr/bin/env python3
"""Record captioned 1080p clips of 404tui by driving it in a private tmux socket.

    python3 docs/video/record.py slicer pads            # some clips
    python3 docs/video/record.py --all                  # every clip in clips/
    python3 docs/video/record.py --list

The program is started in a tmux server of its own (-L clips, never the default one), the clip's keys
are sent on a timeline, the screen is captured with its colours about 25 times a second, and then
frames are rendered at 15 fps: the terminal at 1920x982 plus a caption strip (a title typed out, a
line of explanation, and the key that was pressed as a badge). ffmpeg joins them into an MP4.

Everything here is documented in docs/video/README.md. A clip is a small file in clips/ (see there).
"""
import argparse, hashlib, importlib.util, os, re, shutil, subprocess, sys, tempfile, time

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
sys.path.insert(0, os.path.join(HERE, "..", "img"))
try:
    from PIL import Image, ImageDraw, ImageFont
    import render as fonts  # docs/img/render.py: font lookup and the Braille check
except ImportError as e:
    sys.exit("record.py needs Pillow (pip install pillow): %s" % e)

SOCK, SESS = "clips", "ui"
COLS, ROWS, STRIP = 140, 45, 98          # the terminal is 140x45 cells; the strip is 98 px; 1920x1080 in all
W, CELL_H = 1920, 21.82
CELL_W = W / COLS
FPS = 15
BG, FG = (18, 18, 22), (222, 222, 226)
AMBER, STRIP_BG, STRIP_LINE = (255, 176, 0), (10, 10, 13), (52, 52, 58)
ANSI16 = [(0,0,0),(205,49,49),(13,188,121),(229,229,16),(36,114,200),(188,63,188),(17,168,205),(229,229,229),
          (102,102,102),(241,76,76),(35,209,139),(245,245,67),(59,142,234),(214,112,214),(41,184,219),(255,255,255)]


def need(path, what):
    if not path:
        sys.exit("record.py: no font for %s found (DejaVu Sans Mono, FreeMono or DejaVu Sans)" % what)
    return path


class Fonts:
    def __init__(self):
        mono = need(fonts.find(["DejaVuSansMono.ttf"]), "the terminal text")
        bold = fonts.find(["DejaVuSansMono-Bold.ttf"]) or mono
        braille = need(fonts.find(fonts.BRAILLE), "Braille")
        if not fonts.has_braille(braille):
            sys.exit("record.py: %s has no Braille glyphs, the Slicer waveform would be boxes" % braille)
        sans_b = need(fonts.find(["DejaVuSans-Bold.ttf"]), "captions")
        sans = need(fonts.find(["DejaVuSans.ttf"]), "captions")
        L = ImageFont.truetype
        self.text, self.bold, self.braille = L(mono, 22), L(bold, 22), L(braille, 24)
        self.title, self.sub, self.key = L(sans_b, 28), L(sans, 22), L(bold, 29)


def c256(n):
    if n < 16:
        return ANSI16[n]
    if n < 232:
        n -= 16
        f = lambda v: 0 if v == 0 else 55 + 40 * v
        return (f(n // 36), f((n // 6) % 6), f(n % 6))
    v = 8 + 10 * (n - 232)
    return (v, v, v)


def render_screen(ansi, F):
    """One tmux capture (with -e) as an image: the terminal on top, an empty strip underneath."""
    img = Image.new("RGB", (W, round(CELL_H * ROWS) + STRIP), BG)
    d = ImageDraw.Draw(img)
    fg = bg = None
    bd = rev = False
    for y, line in enumerate(ansi.split("\n")[:ROWS]):
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
            if x >= COLS:
                continue
            c = m.group(2)
            f_, b_ = (fg or FG), (bg or BG)
            if rev: f_, b_ = b_, f_
            X, Y = x * CELL_W, y * CELL_H
            if b_ != BG: d.rectangle([X, Y, X + CELL_W, Y + CELL_H], fill=b_)
            if c != " ":
                font = F.braille if "⠀" <= c <= "⣿" else (F.bold if bd else F.text)
                d.text((X, Y), c, font=font, fill=f_)
            x += 1
    return img


def mix(c, k):
    return tuple(round(10 + (v - 10) * k) for v in c)


def add_strip(base, F, caption, t_caption, key, t_key):
    """The caption strip: the title typed out with a cursor, the line under it, the key badge."""
    img = base.copy()
    d = ImageDraw.Draw(img)
    y0 = round(CELL_H * ROWS)
    d.rectangle([0, y0, W, img.height], fill=STRIP_BG)
    d.line([0, y0, W, y0], fill=STRIP_LINE)
    title, _, sub = caption.partition("|")
    nt = len(title) * min(1.0, t_caption / 0.28)
    ns = len(sub) * min(1.0, max(0.0, t_caption - 0.2) / 0.9)
    shown_t, shown_s = title[:int(nt + 0.99)], sub[:int(ns)]
    d.text((32, y0 + 12), shown_t, font=F.title, fill=mix(AMBER, min(1.0, 0.35 + t_caption / 0.25)))
    d.text((32, y0 + 53), shown_s, font=F.sub, fill=(190, 190, 198))
    typing = len(shown_t) < len(title) or len(shown_s) < len(sub)
    if caption and (typing or (int(t_caption * 5) % 2 == 0 and t_caption < 1.6)):
        in_title = len(shown_t) < len(title)
        cx = 32 + d.textlength(shown_t if in_title else shown_s, font=F.title if in_title else F.sub) + 4
        cy, ch = (y0 + 14, 30) if in_title else (y0 + 53, 24)
        d.rectangle([cx, cy, cx + 10, cy + ch], fill=AMBER)
    if key and t_key < 1.2:
        k = 1.0 if t_key < 0.8 else max(0.0, (1.2 - t_key) / 0.4)
        grow = 9 * max(0.0, 1 - t_key / 0.12)
        w = d.textlength(key, font=F.key) + 42 + 2 * grow
        d.rounded_rectangle([W - 32 - w, y0 + 20 - grow, W - 32 + grow, y0 + STRIP - 20 + grow], 11, fill=mix(AMBER, k))
        d.text((W - 32 - w + 21 + grow, y0 + 27), key, font=F.key, fill=(22, 22, 24) if k > 0.5 else mix(AMBER, 0.0))
    return img


class Tmux:
    def __init__(self, sock):
        self.sock = sock

    def run(self, *a):
        return subprocess.run(["tmux", "-L", self.sock, *a], capture_output=True, text=True)

    def pane(self, colour=False):
        return self.run("capture-pane", *(["-e"] if colour else []), "-t", SESS, "-p").stdout

    def send(self, k):
        if k in {"Space", "Enter", "Tab", "Escape", "Left", "Right", "Up", "Down", "BTab"}:
            self.run("send-keys", "-t", SESS, k)
        else:
            self.run("send-keys", "-t", SESS, "-l", k)

    def wait_for(self, rx, secs=40):
        end = time.time() + secs
        while time.time() < end:
            if re.search(rx, self.pane()):
                return
            time.sleep(0.3)
        sys.exit("record.py: waited %ds for /%s/ and it never came. The screen:\n%s" % (secs, rx, self.pane()))


KEYNAMES = {"Space": "Space", "Enter": "Enter", "Tab": "Tab", "Escape": "Esc", "Left": "←", "Right": "→", "Up": "↑", "Down": "↓"}


def load_clip(name):
    path = os.path.join(HERE, "clips", name + ".py")
    if not os.path.exists(path):
        sys.exit("record.py: no clip %s (see --list)" % name)
    spec = importlib.util.spec_from_file_location("clip_" + name, path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def record(name, args, F):
    clip = load_clip(name)
    tmux = Tmux(SOCK)
    cfg = tempfile.mkdtemp(prefix="404clip-cfg-")           # a throwaway config: the slicer remembers its slices there
    env = "XDG_CONFIG_HOME=%s %s" % (cfg, ("HOME=%s " % clip.HOME) if getattr(clip, "HOME", None) else "")
    cmd = env + clip.CMD.format(bin=args.bin, lib=args.lib)
    tmux.run("kill-server")                                  # only our own server: -L clips
    tmux.run("new-session", "-d", "-s", SESS, "-x", str(COLS), "-y", str(ROWS), cmd)
    time.sleep(3)
    for s in clip.SETUP:
        if s[0] == "wait": tmux.wait_for(s[1])
        else: tmux.send(s[0]); time.sleep(s[1])
    # phase 1: play the keys on their timeline and store the raw screen with its time (fast, so the clock stays true)
    events, t = [], 0.0
    for key, secs, cap in clip.STEPS:
        events.append((t, key, cap)); t += secs
    total = t + getattr(clip, "TAIL", 1.5)
    raw, log, i = [], [], 0
    t0 = time.monotonic()
    while True:
        now = time.monotonic() - t0
        if now > total: break
        while i < len(events) and events[i][0] <= now:
            _, key, cap = events[i]; i += 1
            if key: tmux.send(key)
            log.append((now, key, cap))
        raw.append((now, tmux.pane(True)))
        time.sleep(0.04)
    tmux.run("kill-server")
    shutil.rmtree(cfg, ignore_errors=True)
    # phase 2: a frame every 1/FPS from what was on screen then
    frames = tempfile.mkdtemp(prefix="404clip-frames-")
    cache, n, j = {}, 0, 0
    while n / FPS < total:
        ft = n / FPS
        while j + 1 < len(raw) and raw[j + 1][0] <= ft: j += 1
        h = hashlib.md5(raw[j][1].encode()).hexdigest()
        if h not in cache: cache[h] = render_screen(raw[j][1], F)
        cap, tc, key, tk = "", 9.0, "", 9.0
        for tt, k, c in log:
            if tt <= ft:
                if c: cap, tc = c, ft - tt
                if k: key, tk = KEYNAMES.get(k, k), ft - tt
        add_strip(cache[h], F, cap, tc, key, tk).save(os.path.join(frames, "f%05d.png" % n))
        n += 1
    os.makedirs(args.out, exist_ok=True)
    mp4 = os.path.join(args.out, name + ".mp4")
    subprocess.run(["ffmpeg", "-y", "-loglevel", "error", "-framerate", str(FPS), "-i", os.path.join(frames, "f%05d.png"),
                    "-vf", "format=yuv420p", "-c:v", "libx264", "-crf", "24", "-preset", "slow", "-movflags", "+faststart", mp4], check=True)
    if args.frames:
        shutil.move(frames, os.path.join(args.out, name + "-frames"))
    else:
        shutil.rmtree(frames, ignore_errors=True)
    print("%s: %d frames, %d KB, %.1f s -> %s" % (name, n, os.path.getsize(mp4) // 1024, total, mp4))


def make_lib(args):
    """The folder the offline clips run on, made once: a blank project and the break to slice."""
    if os.path.isdir(os.path.join(args.lib, "PROJECT_01")):
        return
    if not args.break_wav:
        sys.exit("record.py: %s does not exist; give --break drum_break.wav (a WAV of a drum break) to make it" % args.lib)
    subprocess.run(["go", "run", "./docs/video/mklib", args.lib, args.break_wav], cwd=REPO, check=True)


def main():
    here = sorted(f[:-3] for f in os.listdir(os.path.join(HERE, "clips")) if f.endswith(".py"))
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("clips", nargs="*")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--list", action="store_true")
    ap.add_argument("--bin", default="/tmp/sp404", help="the program, built with: go build -o /tmp/sp404 ./cmd/sp404")
    ap.add_argument("--lib", default="/tmp/404lib", help="folder for the offline clips (made from --break when missing)")
    ap.add_argument("--break", dest="break_wav", help="a WAV of a drum break, for the Slicer and Files clips")
    ap.add_argument("--out", default="/tmp/404clips")
    ap.add_argument("--frames", action="store_true", help="keep the rendered frames next to the MP4")
    args = ap.parse_args()
    if args.list:
        print("\n".join(here)); return
    names = here if args.all else args.clips
    if not names:
        ap.print_help(); return
    for tool in ("tmux", "ffmpeg"):
        if not shutil.which(tool): sys.exit("record.py: %s is not installed" % tool)
    if not os.path.exists(args.bin): sys.exit("record.py: %s not found; build it first (see --bin)" % args.bin)
    if any(getattr(load_clip(n), "OFFLINE", False) for n in names): make_lib(args)
    F = Fonts()
    for n in names:
        record(n, args, F)


if __name__ == "__main__":
    main()
