#!/usr/bin/env python3
"""Writes index.html and compositions/*.html of the 404tui presentation film.

    python3 build.py        # then: npm run check ; npx hyperframes snapshot --at 5,20 ; npm run render

The feature scenes are one template filled from FEATURES below: a clip, the stretch of it to show, and the
beats (a key, a caption, a part of the screen to punch in on). Times of beats are seconds of the clip, so
the captions stay in step with the footage whatever the speed is.
"""
import json, os

CW, CH = 1920 / 140, 21.82                  # one terminal cell in clip pixels (the clips are 1920 wide)
OVERLAP = 0.45                              # a scene starts this long before the one before ends
WIN_W, WIN_H = 1840, 941                    # the footage window; the clip's terminal part is 1920 x 982
S0 = WIN_W / 1920

# ---- the feature scenes: clip, from, to, speed, tab, title, beats = (clip time, key, caption, rect cols/rows or None)
FEATURES = [
    dict(id="files", clip="files", a=3.0, b=17.4, rate=1.1, tab="TAB 2", name="FILES", enter="wipe", beats=[
        (3.2, "Tab", "Tab opens a folder in the next column", (0, 1, 90, 14)),
        (8.6, "j", "Details: length, rate, waveform", (86, 1, 54, 23)),
        (11.0, "Space", "Space listens before it goes anywhere", (0, 43, 70, 2)),
        (13.8, "]", "[ ] chooses the target bank", (0, 6, 100, 12))]),
    dict(id="pads", clip="pads", a=7.4, b=20.4, rate=1.0, tab="TAB 1", name="PADS", enter="push", beats=[
        (7.7, "Enter", "Enter previews the pad", (80, 33, 60, 9)),
        (9.9, "S", "S opens the pad's settings", (24, 10, 92, 23)),
        (13.8, "l", "h l changes it: pan moves to the centre", (24, 12, 48, 9)),
        (17.8, "0", "0 restores the default", (24, 12, 48, 9))]),
    dict(id="slicer", clip="slicer", a=2.4, b=15.2, rate=1.0, tab="TAB 4", name="SLICER", enter="zoom", beats=[
        (2.6, "t", "t cuts at the onsets", None),
        (4.6, None, "32 slices, each on a hit", (0, 4, 72, 22)),
        (6.9, "l", "h l selects a slice", (0, 24, 100, 11)),
        (9.7, "r", "r plays it backwards", (0, 22, 70, 12)),
        (12.0, "x", "x rolls: the slice repeats", (50, 24, 90, 8))]),
    dict(id="sequencer", clip="sequencer", a=11.8, b=27.0, rate=1.1, tab="TAB 5", name="SEQUENCER", enter="wipe", beats=[
        (12.4, None, "A lane per pad", (0, 5, 76, 9)),
        (15.4, "l", "h j k l moves the cursor", (0, 5, 76, 9)),
        (18.4, "x", "x puts a note on", (0, 5, 76, 9)),
        (20.7, "+", "+ makes it longer", (0, 5, 76, 9)),
        (23.0, "<", "< makes it softer", (0, 5, 76, 9))]),
    dict(id="looper", clip="looper", a=8.2, b=18.0, rate=1.0, tab="TAB 8", name="LOOPER", enter="push", beats=[
        (8.4, "[", "Bank A holds three loops", (0, 1, 140, 12)),
        (10.6, "p", "p: all 16 pads start on beat 1", (0, 43, 100, 2)),
        (14.3, "x", "x remixes a pad into slices", (0, 1, 140, 27))]),
    dict(id="perform", clip="perform", a=0.3, b=11.4, rate=1.0, tab="TAB 7", name="PERFORM", enter="wipe", beats=[
        (0.6, None, "A dashboard for the stage", None),
        (3.9, None, "The pads of the bank the unit is on", (58, 1, 82, 43)),
        (7.2, None, "Clock, level and the bus effects", (0, 2, 60, 41))]),
    dict(id="config", clip="config", a=3.2, b=15.6, rate=1.1, tab="TAB 6", name="CONFIG", enter="zoom", beats=[
        (3.9, "Enter", "Enter edits a text setting", (20, 16, 95, 11)),
        (8.8, "j", "j k moves between settings", (0, 1, 115, 12)),
        (12.2, "l", "h l changes a colour in place", (0, 4, 115, 6))]),
]

def focus_panels(fid, i, x, y, w, h, W, H):
    """four blurred, dimmed panels round the part in focus (cam pixels), plus a hairline edge; the part stays sharp"""
    x, y, w, h = round(x), round(y), round(w), round(h)
    x0, y0 = max(0, x), max(0, y)
    x1, y1 = min(W, x + w), min(H, y + h)
    box = [(0, 0, W, y0), (0, y1, W, H - y1), (0, y0, x0, y1 - y0), (x1, y0, W - x1, y1 - y0)]
    out = [f'<div class="dim {fid}-f{i}" style="left:{a}px;top:{b}px;width:{c}px;height:{d}px"></div>' for a, b, c, d in box if c > 0 and d > 0]
    out.append(f'<div class="edge {fid}-f{i}" style="left:{x}px;top:{y}px;width:{w}px;height:{h}px"></div>')
    return "".join(out)

def cam_for(rect):
    """camera x, y, scale to show a rectangle of cells in the window"""
    if rect is None:
        return 0, 0, S0
    c, r, w, h = rect
    rx, ry, rw, rh = c * CW, r * CH, w * CW, h * CH
    px, py = rw * 0.06 + 20, rh * 0.10 + 24          # room round the part, so its ring shows whole
    rx, ry, rw, rh = rx - px, ry - py, rw + 2 * px, rh + 2 * py
    s = max(S0, min(WIN_W / rw, WIN_H / rh, 2.6))
    x = min(0, max(WIN_W - 1920 * s, (WIN_W - rw * s) / 2 - rx * s))
    y = min(0, max(WIN_H - 982 * s, (WIN_H - rh * s) / 2 - ry * s))
    return round(x, 1), round(y, 1), round(s, 4)

ENTER = {   # how a scene arrives over the one before it
    "wipe": 'tl.fromTo("#root", { clipPath: "inset(0 100% 0 0)" }, { clipPath: "inset(0 0% 0 0)", duration: 0.55, ease: "power3.inOut" }, 0);',
    "push": 'tl.fromTo("#root", { xPercent: 100 }, { xPercent: 0, duration: 0.55, ease: "power3.inOut" }, 0);',
    "zoom": 'tl.fromTo("#root", { opacity: 0, scale: 1.12 }, { opacity: 1, scale: 1, duration: 0.5, ease: "power3.out" }, 0);',
    "fade": 'tl.fromTo("#root", { opacity: 0 }, { opacity: 1, duration: 0.45, ease: "power2.out" }, 0);',
}

HEAD = """<template>
  <style>
    #root { position: absolute; inset: 0; background: #0d0d10; overflow: hidden; color: #ececf0; font-family: "Inter", sans-serif; }
    .mono { font-family: "IBM Plex Mono", monospace; }
    .amber { color: #ffb000; }
"""

def feature(f):
    dur = round((f["b"] - f["a"]) / f["rate"], 3)
    t = lambda m: round((m - f["a"]) / f["rate"], 3)
    beats = f["beats"]
    for i, (m, key, cap, rect) in enumerate(f["beats"]):
        end = t(f["beats"][i + 1][0]) if i + 1 < len(f["beats"]) else dur - 0.5
        need = 0.5 + 0.3 * len(cap.split())
        if end - t(m) + 0.05 < need:
            print("  too quick: %s beat %d shows %.1f s, wants %.1f s: %r" % (f["id"], i, end - t(m), need, cap))
    rings, caps, tl = [], [], []
    tl.append(ENTER[f["enter"]])
    tl.append(f'gsap.set("#cam", {{ transformOrigin: "0 0", x: 0, y: 0, scale: {S0:.4f} }});')
    tl.append(f'tl.fromTo("#band", {{ opacity: 0, y: 24 }}, {{ opacity: 1, y: 0, duration: 0.4, ease: "power3.out" }}, 0.25);')
    for i, (m, key, cap, rect) in enumerate(beats):
        t0 = max(0.05, t(m)); t1 = t(beats[i + 1][0]) if i + 1 < len(beats) else dur - 0.5
        x, y, s = cam_for(rect)
        caps.append(f'<div class="cap" id="{f["id"]}-cap{i}"><span class="key mono" id="{f["id"]}-key{i}">{key or ""}</span><span class="txt">{cap}</span></div>')
        tl.append(f'tl.fromTo("#{f["id"]}-cap{i}", {{ opacity: 0, y: 18 }}, {{ opacity: 1, y: 0, duration: 0.3, ease: "power3.out" }}, {t0:.3f});')
        if key:
            tl.append(f'tl.fromTo("#{f["id"]}-key{i}", {{ scale: 1.5 }}, {{ scale: 1, duration: 0.3, ease: "back.out(2)" }}, {t0:.3f});')
        if i + 1 < len(beats):
            tl.append(f'tl.to("#{f["id"]}-cap{i}", {{ opacity: 0, y: -14, duration: 0.2, ease: "power2.in" }}, {t1 - 0.2:.3f});')
        else:
            tl.append(f'tl.to("#{f["id"]}-cap{i}", {{ opacity: 0, duration: 0.25 }}, {dur - 0.3:.3f});')
        # the camera arrives with the caption, holds while the screen acts, and the ring draws round the part
        tl.append(f'tl.to("#cam", {{ x: {x}, y: {y}, scale: {s}, duration: 0.55, ease: "power3.inOut" }}, {max(0.0, t0 - 0.25):.3f});')
        if rect:
            c, r, w, h = rect
            rings.append(focus_panels(f['id'], i, c*CW-12, r*CH, w*CW+24, h*CH, 1920, 982))
            tl.append(f'tl.fromTo(".{f["id"]}-f{i}", {{ opacity: 0 }}, {{ opacity: 1, duration: 0.4, ease: "power2.out" }}, {t0 + 0.3:.3f});')
            tl.append(f'tl.to(".{f["id"]}-f{i}", {{ opacity: 0, duration: 0.25 }}, {t1 - 0.25:.3f});')
    tl.append(f'tl.to("#cam", {{ x: 0, y: 0, scale: {S0:.4f}, duration: 0.5, ease: "power3.inOut" }}, {dur - 0.55:.3f});')
    html = HEAD + f"""    #win {{ position: absolute; left: 40px; top: 20px; width: {WIN_W}px; height: {WIN_H}px; border-radius: 14px; overflow: hidden; background: #121216; box-shadow: 0 0 0 2px #2a2a31; }}
    #cam {{ position: absolute; left: 0; top: 0; width: 1920px; height: 982px; overflow: hidden; }}
    #cam video {{ position: absolute; left: 0; top: 0; width: 1920px; height: 1080px; }}
    .dim {{ position: absolute; opacity: 0; background: rgba(10,10,14,.42); backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); }}
    .edge {{ position: absolute; opacity: 0; border: 2px solid rgba(255,255,255,.55); border-radius: 10px; }}
    #band {{ position: absolute; left: 40px; right: 40px; top: 982px; height: 86px; display: flex; align-items: center; gap: 30px; }}
    .tag {{ font: 600 28px "IBM Plex Mono", monospace; color: #ffb000; letter-spacing: .06em; white-space: nowrap; width: 400px; }}
    .tag b {{ color: #9a9aa6; font-weight: 400; }}
    #caps {{ position: relative; flex: 1; height: 60px; }}
    .cap {{ position: absolute; left: 0; right: 0; top: 0; height: 60px; display: flex; align-items: center; gap: 22px; opacity: 0; }}
    .cap .txt {{ font: 600 36px "Inter", sans-serif; color: #ececf0; white-space: nowrap; }}
    .key {{ font-weight: 700; font-size: 30px; background: #ffb000; color: #161618; padding: 4px 18px; border-radius: 10px; min-width: 56px; text-align: center; }}
    .key:empty {{ display: none; }}
  </style>
  <div id="root" data-composition-id="{f['id']}" data-width="1920" data-height="1080">
    <div id="win"><div id="cam">
      <video id="{f['id']}-vid" class="clip" data-start="0" data-duration="{dur}" data-media-start="{f['a']}" data-playback-rate="{f['rate']}" src="assets/clips/{f['clip']}.mp4" muted playsinline></video>
      {''.join(rings)}
    </div></div>
    <div id="band"><div class="tag">{f['tab']} <b>·</b> {f['name']}</div><div id="caps">{''.join(caps)}</div></div>
  </div>
  <script>
    const tl = gsap.timeline({{ paused: true }});
    {chr(10).join('    ' + l for l in tl).strip()}
    window.__timelines["{f['id']}"] = tl;
  </script>
</template>
"""
    return html, dur

def title_scene():
    pads = "".join(f'<i id="in-p{i}"></i>' for i in range(16))
    seq = [0, 5, 10, 15, 3, 6, 9, 12, 1, 4, 11, 14, 2, 7, 8, 13]
    tl = [
        'tl.fromTo("#in-pads", { y: 40 }, { y: 0, duration: 0.8, ease: "power3.out" }, 0);',
        'tl.fromTo(".in-l", { filter: "blur(18px)", y: 24 }, { filter: "blur(0px)", y: 0, duration: 0.6, ease: "power3.out", stagger: 0.07 }, 0);',
        'tl.fromTo("#in-by", { opacity: 0 }, { opacity: 1, duration: 0.5 }, 1.5);',
        'tl.fromTo("#in-sub", { opacity: 0, y: 20 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 2.0);',
        'tl.fromTo("#in-chip", { opacity: 0, y: 20 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 2.7);',
    ]
    for k, p in enumerate(seq):      # the pads light in turn, on the beat of the music (0.625 s)
        tl.append(f'tl.fromTo("#in-p{p}", {{ backgroundColor: "#2c2c30" }}, {{ backgroundColor: "#ffb000", duration: 0.12 }}, {0.9 + k * 0.3125:.4f});')
        tl.append(f'tl.to("#in-p{p}", {{ backgroundColor: "#2c2c30", duration: 0.5 }}, {1.05 + k * 0.3125:.4f});')
    tl.append('tl.to("#root", { opacity: 0, duration: 0.4 }, 5.55);')
    html = HEAD + """    #in-pads { position: absolute; left: 1250px; top: 250px; width: 520px; height: 520px; display: grid; grid-template-columns: repeat(4, 1fr); gap: 20px; }
    #in-pads i { background: #2c2c30; border-radius: 16px; }
    #in-word { position: absolute; left: 120px; top: 300px; font: 700 250px "IBM Plex Mono", monospace; letter-spacing: -0.04em; line-height: 1; }
    .in-l { display: inline-block; }
    #in-by { position: absolute; left: 130px; top: 560px; font: 500 34px "IBM Plex Mono", monospace; color: #9a9aa6; letter-spacing: .06em; }
    #in-sub { position: absolute; left: 124px; top: 650px; width: 1050px; font: 700 62px "Inter", sans-serif; line-height: 1.12; }
    #in-chip { position: absolute; left: 124px; top: 860px; font: 500 30px "IBM Plex Mono", monospace; color: #ffb000; }
  </style>
  <div id="root" data-composition-id="intro" data-width="1920" data-height="1080">
    <div id="in-pads">""" + pads + """</div>
    <div id="in-word"><span class="in-l amber">4</span><span class="in-l amber">0</span><span class="in-l amber">4</span><span class="in-l">t</span><span class="in-l">u</span><span class="in-l">i</span></div>
    <div id="in-by">by moahbeatlab</div>
    <div id="in-sub">A terminal cockpit for the Roland SP-404MKII</div>
    <div id="in-chip">v0.1-alpha · open source · Go · GPLv3</div>
  </div>
  <script>
    const tl = gsap.timeline({ paused: true });
    """ + "\n    ".join(tl) + """
    window.__timelines["intro"] = tl;
  </script>
</template>
"""
    return html, 6.0

def problem_scene():
    rows = [
        ("pr-1", "Samples are files on an SD card.", "Moving them to pads means shuffling files and navigating the unit.",
         '<div class="file mono">kick_808.wav</div><div class="file mono">break.wav</div><div class="file mono">vox_phrase.wav</div><div class="file mono">bass.wav</div>'),
        ("pr-2", "Pads, patterns and settings live on the unit.", "Every edit happens on its own small screen.",
         '<div class="screen"><b></b><b></b><b></b><b></b><b></b></div>'),
        ("pr-3", "Slicing and chopping happen somewhere else.", "Another tool, another round trip, another folder.",
         '<div class="wave">' + "".join(f'<i style="height:{h}px"></i>' for h in [30, 80, 120, 90, 50, 110, 140, 70, 40, 100, 130, 60, 30, 90, 70]) + '</div>'),
    ]
    html_rows = "".join(
        f'<div class="row" id="{rid}"><div class="txt"><h2>{h}</h2><p>{p}</p><u id="{rid}-strike"></u></div><div class="art" id="{rid}-art">{art}</div></div>'
        for rid, h, p, art in rows)
    tl = ['tl.fromTo("#pr-kicker", { opacity: 0 }, { opacity: 1, duration: 0.5 }, 0.3);']
    for i, (rid, *_rest) in enumerate(rows):
        t0 = 0.9 + i * 3.1
        tl += [f'tl.fromTo("#{rid}", {{ opacity: 0, x: -60 }}, {{ opacity: 1, x: 0, duration: 0.6, ease: "power3.out" }}, {t0});',
               f'tl.fromTo("#{rid}-art > *", {{ opacity: 0, y: 24 }}, {{ opacity: 1, y: 0, duration: 0.45, ease: "power3.out", stagger: 0.12 }}, {t0 + 0.35});']
    tl += ['tl.fromTo(".row u", { scaleX: 0 }, { scaleX: 1, duration: 0.45, ease: "power2.out", stagger: 0.15 }, 9.6);',
           'tl.to(".row", { opacity: 0, duration: 0.45 }, 10.7);',
           'tl.to("#pr-kicker", { opacity: 0, duration: 0.4 }, 10.7);',
           'tl.fromTo("#pr-answer", { opacity: 0, scale: 0.92 }, { opacity: 1, scale: 1, duration: 0.7, ease: "back.out(1.6)" }, 11.1);']
    html = HEAD + """    #pr-kicker { position: absolute; left: 120px; top: 92px; font: 600 28px "IBM Plex Mono", monospace; color: #ffb000; letter-spacing: .14em; }
    .row { position: absolute; left: 120px; width: 1680px; height: 240px; display: flex; align-items: center; justify-content: space-between; opacity: 0; }
    #pr-1 { top: 170px; } #pr-2 { top: 440px; } #pr-3 { top: 710px; }
    .txt { position: relative; width: 1000px; }
    .txt h2 { font: 700 54px "Inter", sans-serif; line-height: 1.1; margin: 0 0 14px; }
    .txt p { font: 400 32px "Inter", sans-serif; color: #9a9aa6; margin: 0; }
    .txt u { position: absolute; left: 0; top: 36px; width: 100%; height: 6px; background: #ff5f5f; transform-origin: 0 50%; transform: scaleX(0); }
    .art { width: 520px; display: flex; flex-direction: column; gap: 10px; align-items: flex-start; }
    .file { background: #1b1b20; border-radius: 10px; padding: 8px 18px; font-size: 28px; color: #ececf0; }
    .screen { width: 360px; height: 190px; border: 5px solid #34343b; border-radius: 16px; padding: 18px; display: flex; flex-direction: column; gap: 14px; }
    .screen b { height: 16px; border-radius: 8px; background: #2c2c30; display: block; width: 100%; } .screen b:nth-child(2) { width: 70%; } .screen b:nth-child(4) { width: 55%; }
    .wave { display: flex; align-items: center; gap: 8px; height: 150px; flex-direction: row; }
    .wave i { display: block; width: 14px; background: #ffb000; border-radius: 6px; opacity: .85; }
    #pr-answer { position: absolute; left: 0; right: 0; top: 400px; text-align: center; font: 700 96px "Inter", sans-serif; line-height: 1.1; opacity: 0; }
  </style>
  <div id="root" data-composition-id="problem" data-width="1920" data-height="1080">
    <div id="pr-kicker">THE SLOW PART</div>
    """ + html_rows + """
    <div id="pr-answer"><span class="amber">404tui</span> puts all of it<br>on your computer's keyboard</div>
  </div>
  <script>
    const tl = gsap.timeline({ paused: true });
    """ + "\n    ".join(tl) + """
    window.__timelines["problem"] = tl;
  </script>
</template>
"""
    return html, 14.0

def what_scene():
    tl = [
        'tl.fromTo("#wh-title", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 0.2);',
        'tl.fromTo("#wh-pc", { opacity: 0, x: -60 }, { opacity: 1, x: 0, duration: 0.6, ease: "power3.out" }, 0.9);',
        'tl.fromTo("#wh-sp", { opacity: 0, x: 60 }, { opacity: 1, x: 0, duration: 0.6, ease: "power3.out" }, 1.5);',
        'tl.fromTo("#wh-cable", { strokeDashoffset: 1 }, { strokeDashoffset: 0, duration: 0.9, ease: "power2.inOut" }, 2.1);',
        'tl.fromTo("#wh-cap1", { opacity: 0, y: 14 }, { opacity: 1, y: 0, duration: 0.5 }, 2.6);',
        'tl.fromTo("#wh-sim", { opacity: 0, y: 40 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 4.8);',
        'tl.fromTo("#wh-phone", { opacity: 0, y: 40 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 6.6);',
        'tl.fromTo("#wh-foot", { opacity: 0 }, { opacity: 1, duration: 0.6 }, 8.6);',
    ]
    html = HEAD + """    #wh-title { position: absolute; left: 120px; top: 90px; font: 700 64px "Inter", sans-serif; opacity: 0; }
    .node { position: absolute; background: #1b1b20; border: 3px solid #34343b; border-radius: 22px; padding: 26px 30px; opacity: 0; }
    .node h3 { margin: 0 0 8px; font: 700 40px "Inter", sans-serif; } .node p { margin: 0; font: 400 28px "Inter", sans-serif; color: #9a9aa6; line-height: 1.3; }
    #wh-pc { left: 120px; top: 270px; width: 640px; height: 340px; }
    #wh-pc .term { margin-top: 14px; background: #0d0d10; border-radius: 10px; padding: 14px 18px; font: 500 28px "IBM Plex Mono", monospace; line-height: 1.5; color: #ececf0; }
    #wh-sp { left: 1160px; top: 270px; width: 640px; height: 340px; }
    #wh-sp .grid { display: grid; grid-template-columns: repeat(4, 56px); gap: 12px; margin-top: 18px; } #wh-sp .grid i { height: 56px; background: #2c2c30; border-radius: 10px; }
    #wh-sp .grid i:nth-child(1), #wh-sp .grid i:nth-child(6), #wh-sp .grid i:nth-child(11) { background: #ffb000; }
    #wh-link { position: absolute; left: 0; top: 0; width: 1920px; height: 1080px; pointer-events: none; }
    #wh-cap1 { position: absolute; left: 760px; width: 400px; top: 470px; text-align: center; font: 600 28px "IBM Plex Mono", monospace; color: #ffb000; opacity: 0; line-height: 1.35; }
    #wh-sim { left: 120px; top: 700px; width: 800px; border-style: dashed; }
    #wh-phone { left: 1000px; top: 700px; width: 800px; }
    #wh-foot { position: absolute; left: 120px; right: 120px; top: 985px; font: 400 28px "Inter", sans-serif; color: #9a9aa6; opacity: 0; }
    .code { font-family: "IBM Plex Mono", monospace; color: #ffb000; }
  </style>
  <div id="root" data-composition-id="what" data-width="1920" data-height="1080">
    <div id="wh-title">One program, over USB</div>
    <svg id="wh-link" viewBox="0 0 1920 1080"><path id="wh-cable" d="M 760 440 L 1160 440" stroke="#ffb000" stroke-width="8" fill="none" stroke-linecap="butt" stroke-dasharray="1" pathLength="1" stroke-dashoffset="1"/></svg>
    <div class="node" id="wh-pc"><h3>Your computer</h3><p>Windows, macOS or Linux, in a terminal</p><div class="term"><span class="amber">$</span> sp404<br>PROJECT_01 · A1 · 404tui</div></div>
    <div class="node" id="wh-sp"><h3>Roland SP-404MKII</h3><p>Projects, pads, samples and the files on its storage and SD card</p><div class="grid"><i></i><i></i><i></i><i></i><i></i><i></i><i></i><i></i></div></div>
    <div id="wh-cap1">USB serial link</div>
    <div class="node" id="wh-sim"><h3>No unit? <span class="code">sp404 -sim</span></h3><p>The same program against a simulated device, and <span class="code">-offline &lt;folder&gt;</span> on exported projects</p></div>
    <div class="node" id="wh-phone"><h3>On a phone: no install</h3><p>A web page over Web MIDI, at <span class="code">moahbeatlab.github.io/404tui</span></p></div>
    <div id="wh-foot">A Go port of SparkyMK2, whose authors reverse-engineered the protocol.</div>
  </div>
  <script>
    const tl = gsap.timeline({ paused: true });
    """ + "\n    ".join(tl) + """
    window.__timelines["what"] = tl;
  </script>
</template>
"""
    return html, 12.6

def outro_scene():
    cmd = "sp404 -sim"
    chars = "".join(f'<span class="ch" id="ou-c{i}">{c if c != " " else "&nbsp;"}</span>' for i, c in enumerate(cmd))
    tl = ['tl.fromTo("#ou-word", { opacity: 0, y: 40 }, { opacity: 1, y: 0, duration: 0.7, ease: "power4.out" }, 0.2);',
          'tl.fromTo("#ou-try", { opacity: 0 }, { opacity: 1, duration: 0.5 }, 1.2);']
    tl += [f'tl.fromTo("#ou-c{i}", {{ opacity: 0 }}, {{ opacity: 1, duration: 0.05 }}, {1.7 + i * 0.09:.2f});' for i in range(len(cmd))]
    tl += ['tl.fromTo("#ou-links", { opacity: 0, y: 20 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 3.3);',
           'tl.fromTo("#ou-status", { opacity: 0, y: 20 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 4.8);']
    html = HEAD + """    #ou-word { position: absolute; left: 0; right: 0; top: 130px; text-align: center; font: 700 190px "IBM Plex Mono", monospace; letter-spacing: -0.04em; opacity: 0; }
    #ou-try { position: absolute; left: 0; right: 0; top: 410px; text-align: center; font: 500 34px "IBM Plex Mono", monospace; color: #9a9aa6; opacity: 0; }
    #ou-cmd { position: absolute; left: 0; right: 0; top: 470px; text-align: center; font: 700 96px "IBM Plex Mono", monospace; color: #ffb000; }
    #ou-cmd .pr { color: #9a9aa6; }
    .ch { opacity: 0; } #ou-cmd .pr { opacity: 1; }
    #ou-links { position: absolute; left: 0; right: 0; top: 660px; text-align: center; font: 500 38px "IBM Plex Mono", monospace; line-height: 1.6; opacity: 0; }
    #ou-status { position: absolute; left: 260px; right: 260px; top: 860px; text-align: center; font: 400 31px "Inter", sans-serif; color: #9a9aa6; line-height: 1.4; opacity: 0; border-top: 2px solid #34343b; padding-top: 28px; }
    #ou-status b { color: #ececf0; font-weight: 600; }
  </style>
  <div id="root" data-composition-id="outro" data-width="1920" data-height="1080">
    <div id="ou-word"><span class="amber">404</span>tui</div>
    <div id="ou-try">Try it without a unit</div>
    <div id="ou-cmd"><span class="pr">$ </span>""" + chars + """</div>
    <div id="ou-links">github.com/moahbeatlab/404tui<br>moahbeatlab.github.io/404tui</div>
    <div id="ou-status"><b>v0.1-alpha.</b> Tested against a simulated unit; not yet verified on real hardware.</div>
  </div>
  <script>
    const tl = gsap.timeline({ paused: true });
    """ + "\n    ".join(tl) + """
    window.__timelines["outro"] = tl;
  </script>
</template>
"""
    return html, 9.0

def main():
    here = os.path.dirname(os.path.abspath(__file__))
    os.makedirs(os.path.join(here, "compositions"), exist_ok=True)
    scenes = [("intro",) + title_scene(), ("problem",) + problem_scene(), ("what",) + what_scene()]
    scenes += [(f["id"],) + feature(f) for f in FEATURES] + [("outro",) + outro_scene()]
    hosts, t = [], 0.0
    for i, (sid, html, dur) in enumerate(scenes):
        open(os.path.join(here, "compositions", sid + ".html"), "w").write(html)
        hosts.append(f'    <div id="{sid}" data-composition-id="{sid}" data-composition-src="compositions/{sid}.html" data-start="{t:.3f}" data-duration="{dur}" data-track-index="1" data-track-kind="graphics" data-width="1920" data-height="1080" style="z-index:{i + 1}"></div>')
        t += dur - (OVERLAP if i + 1 < len(scenes) else 0)
    total = round(t, 3)
    index = f"""<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=1920, height=1080" />
    <script src="https://cdn.jsdelivr.net/npm/gsap@3.14.2/dist/gsap.min.js"></script>
    <style>
      * {{ margin: 0; padding: 0; box-sizing: border-box; }}
      html, body {{ width: 1920px; height: 1080px; overflow: hidden; background: #0d0d10; }}
    </style>
  </head>
  <body>
    <div id="root" data-composition-id="main" data-start="0" data-duration="{total}" data-width="1920" data-height="1080">
{chr(10).join(hosts)}
      <audio id="music" src="assets/music.wav" data-start="0" data-duration="{total}" data-track-index="2" data-volume="0.7" data-fade-out="3"></audio>
    </div>
    <script>
      window.__timelines = window.__timelines || {{}};
      const tl = gsap.timeline({{ paused: true }});
      window.__timelines["main"] = tl;
      tl.seek(0);
    </script>
  </body>
</html>
"""
    open(os.path.join(here, "index.html"), "w").write(index)
    print("scenes:", [(s[0], s[2]) for s in scenes]); print("total:", total)

if __name__ == "__main__":
    main()
