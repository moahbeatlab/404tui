#!/usr/bin/env python3
"""A short, plain backing track for the film: pad chords, bass, a light beat, an arp. Generated, no samples.
    python3 music.py assets/music.wav 112
"""
import sys, wave
import numpy as np
out, total = sys.argv[1], float(sys.argv[2])
sr, bpm = 44100, 96
beat = 60 / bpm
n = int(total * sr)
rng = np.random.default_rng(404)          # seeded: the same file every time
L = np.zeros(n); R = np.zeros(n)
def add(sig, t0, gain=1.0, pan=0.0):
    i = int(t0 * sr)
    if i >= n: return
    sig = sig[: n - i]
    L[i:i+len(sig)] += sig * gain * (1 - max(pan, 0)); R[i:i+len(sig)] += sig * gain * (1 + min(pan, 0))
def tone(f, d, kind="sine", a=0.01, r=0.2, vib=0.0):
    t = np.arange(int(d * sr)) / sr
    ph = 2 * np.pi * f * t + vib * np.sin(2 * np.pi * 5 * t)
    w = np.sin(ph) if kind == "sine" else (2 * np.abs(2 * ((f * t) % 1) - 1) - 1 if kind == "tri" else np.sign(np.sin(ph)) * 0.6 + np.sin(ph) * 0.4)
    env = np.minimum(1, t / a) * np.minimum(1, (d - t) / r).clip(0, 1)
    return w * env
def kick(): t = np.arange(int(.28 * sr)) / sr; return np.sin(2*np.pi*(48 + 90*np.exp(-t*28))*t) * np.exp(-t*9)
def hat(): t = np.arange(int(.05 * sr)) / sr; return rng.standard_normal(len(t)) * np.exp(-t*70) * 0.5
def clap(): t = np.arange(int(.2 * sr)) / sr; return rng.standard_normal(len(t)) * np.exp(-t*22) * 0.6
hz = lambda m: 440 * 2 ** ((m - 69) / 12)
chords = [(57, 60, 64), (53, 57, 60), (48, 52, 55), (55, 59, 62)]   # Am F C G
bar = 4 * beat
def groove_on(t):   # which parts play when: intro sparse, problem tense, the tour full, the end soft
    return "intro" if t < 6 else "problem" if t < 19 else "tour" if t < total - 9 else "end"
for b in range(int(total / bar) + 1):
    t0 = b * bar; ch = chords[b % 4]; sec = groove_on(t0)
    for m in ch: add(tone(hz(m), bar + .3, "tri", .6, .8), t0, .055 if sec != "end" else .07, pan=((m % 5) - 2) * .08)
    add(tone(hz(ch[0] - 12), bar, "sine", .02, .3), t0, .16 if sec in ("tour", "end") else .10)
    for s in range(16):
        t = t0 + s * beat / 4
        if sec == "tour":
            if s % 4 == 0: add(kick(), t, .55)
            if s in (4, 12): add(clap(), t, .30)
            if s % 2 == 1: add(hat(), t, .16 + .05 * (s % 4 == 3))
            if s in (3, 10, 14): add(tone(hz(ch[0]), .2, "sine", .01, .15), t, .10)
        elif sec == "intro" and s in (0, 8): add(kick(), t, .35 if s == 0 else .2)
        elif sec == "problem" and s % 8 == 4: add(hat(), t, .10)
        if sec in ("tour", "end") and s % 2 == 0: add(tone(hz(ch[(s // 2) % 3] + 12), .16, "tri", .005, .12), t, .045, pan=.3 if s % 4 else -.3)
fi, fo = int(2 * sr), int(4 * sr)
L[:fi] *= np.linspace(0, 1, fi); R[:fi] *= np.linspace(0, 1, fi)
L[-fo:] *= np.linspace(1, 0, fo); R[-fo:] *= np.linspace(1, 0, fo)
pk = max(abs(L).max(), abs(R).max()); L, R = L / pk * .8, R / pk * .8
st = (np.stack([L, R], 1) * 32767).astype("<i2")
w = wave.open(out, "wb"); w.setnchannels(2); w.setsampwidth(2); w.setframerate(sr); w.writeframes(st.tobytes()); w.close()
print("wrote", out)
