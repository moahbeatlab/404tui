#!/usr/bin/env python3
"""Make a page to review recorded clips in a browser: one section per clip, with its poster and a player.

    python3 docs/video/review.py /tmp/404clips          # writes /tmp/404clips/review/index.html

Posters are a frame from the middle of each clip (ffmpeg). The page is plain HTML and CSS with no scripts
and no outside files, so it opens from disk or from any web host.
"""
import html, os, shutil, subprocess, sys

def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    src = sys.argv[1]
    clips = sorted(f for f in os.listdir(src) if f.endswith(".mp4"))
    if not clips:
        sys.exit("review.py: no .mp4 in %s" % src)
    out = os.path.join(src, "review")
    os.makedirs(os.path.join(out, "videos"), exist_ok=True)
    os.makedirs(os.path.join(out, "posters"), exist_ok=True)
    sections = []
    for f in clips:
        name = f[:-4]
        shutil.copy(os.path.join(src, f), os.path.join(out, "videos", f))
        dur = float(subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0",
                                    os.path.join(src, f)], capture_output=True, text=True).stdout or 0)
        subprocess.run(["ffmpeg", "-y", "-loglevel", "error", "-ss", str(dur / 2), "-i", os.path.join(src, f), "-frames:v", "1",
                        "-vf", "scale=1280:-2", os.path.join(out, "posters", name + ".jpg")], check=True)
        sections.append('<section id="%s"><h2>%s <small>%.0f s</small></h2><video controls preload="metadata" poster="posters/%s.jpg" src="videos/%s"></video></section>'
                        % (name, html.escape(name), dur, name, f))
    page = """<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>404tui clips</title>
<style>body{margin:0;padding:24px max(16px,env(safe-area-inset-left));background:#121216;color:#ececf0;font:16px/1.5 system-ui,sans-serif}
main{max-width:1100px;margin:0 auto;display:flex;flex-direction:column;gap:40px}h1{margin:0 0 4px}h2{margin:0 0 10px;font-size:22px}
small{color:#a0a0aa;font-weight:400;font-size:15px}video{display:block;width:100%%;aspect-ratio:16/9;background:#0d0d10;border-radius:10px}
a{color:#ffb000}nav{display:flex;flex-wrap:wrap;gap:8px}nav a{background:#2a2a31;color:#ececf0;text-decoration:none;padding:6px 12px;border-radius:6px;font-family:monospace}</style>
<main><header><h1>404tui clips</h1><nav>%s</nav></header>%s</main>""" % (
        "".join('<a href="#%s">%s</a>' % (f[:-4], f[:-4]) for f in clips), "".join(sections))
    with open(os.path.join(out, "index.html"), "w") as fh:
        fh.write(page)
    print("review page:", os.path.join(out, "index.html"))

if __name__ == "__main__":
    main()
