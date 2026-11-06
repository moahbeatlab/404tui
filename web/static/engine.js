'use strict';
// The page without a server: this is what `sp404 serve` does for Perform (Motion, Punch, Morph, LFO, the effects of the four
// buses, the clock, the patterns), written for the browser and talking to the unit by Web MIDI. It is a port of tui/motion.go,
// punch.go, morph.go, lfo.go and what midi.go does with a message, so that the same page works with it. What needs the
// unit's USB serial link (the pad names, levels and samples, the Looper, the Slicer's pads) is not here.
//
// The page gives it commands (as the WebSocket does) and gets the state back as objects:
//   const e = SP404Local.start({ demo: false }); const socket = e.socket();  // like a WebSocket: send(text), onmessage({data})
(() => {
  const FX = window.SP404_FX;
  const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v));
  const TICKS_BAR = 96;
  const PUNCH_CCS = [16, 17, 18, 80, 81, 82];
  const CC_SW = 19, CC_FX = 83;
  const MORPH_CHAN = 14, EC4_FIRST_NOTE = 60, EC4_FIRST_CC = 60, MORPH_STEP = 2;
  const SLOTS = 4;
  const LFO_COUNT = 4, LFO_SWITCH = 6;
  const LFO_RATES = [0.25, 0.5, 1, 2, 4, 8, 16, 32];
  const OVERDUB_HOLD = 12;
  // The notes of the 16 pads of a bank, from the first pad: the unit's own order.
  const PAD_OFFSET = [12, 13, 14, 15, 8, 9, 10, 11, 4, 5, 6, 7, 0, 1, 2, 3];
  const LS = 'sp404.local.v1';

  const fxTable = (bus) => (bus >= 2 ? FX.fx34 : FX.fx12);
  const fxNum = (bus, name) => fxTable(bus).indexOf(name);
  const fxNameOf = (bus, num) => { const t = fxTable(bus); return num >= 0 && num < t.length ? t[num] : '?'; };
  const emptyBus = () => ({ set: false, on: -1, num: -1, knobs: [-1, -1, -1, -1, -1, -1] });
  const emptySnap = () => [0, 1, 2, 3].map(emptyBus);
  const cloneSnap = (s) => s.map((b) => ({ set: b.set, on: b.on, num: b.num, knobs: b.knobs.slice() }));
  const snapSet = (s) => s.some((p) => p.set);
  const sign = (v) => (v > 0 ? 1 : v < 0 ? -1 : 0);
  const plural = (n, w) => n + ' ' + w + (n === 1 ? '' : 's');

  // ---- the knobs as the unit shows them (tui/fxparams.go) ----
  function knobsOf(bus, num) {
    const p = FX.params[fxNameOf(bus, num)];
    if (p) return p;
    return [0, 1, 2, 3, 4, 5].map((k) => ({ n: 'CTRL ' + (k + 1), k: 'c', lo: 0, hi: 127 }));
  }
  function valueText(p, v) {
    if (v < 0) return '–';
    switch (p.k) {
      case 'b': return v >= 64 ? 'ON' : 'OFF';
      case 'e': if (p.o && p.o.length) return p.o[Math.min(p.o.length - 1, Math.floor(v * p.o.length / 128))]; break;
      case 'bal': { const w = Math.round(v * 100 / 127); return (100 - w) + ':' + w; }
      case 'pan': { const n = Math.round(v * 100 / 127) - 50; return n < 0 ? 'L' + (-n) : n > 0 ? 'R' + n : 'CTR'; }
      case 'nt': return ['C', 'C#', 'D', 'D#', 'E', 'F', 'F#', 'G', 'G#', 'A', 'A#', 'B'][v % 12] + (Math.floor(v / 12) - 1);
    }
    if (p.inf && v === 0) return '-INF';
    const lo = p.lo || 0, hi = p.hi || 0;
    const x = lo + (hi - lo) * v / 127, span = Math.abs(hi - lo);
    const dec = span >= 20 ? 0 : span >= 2 ? 1 : span >= 0.5 ? 2 : 3;
    const f = Math.pow(10, dec);
    let s = String(Math.round(x * f) / f);
    if (p.u) s += ' ' + p.u;
    return s;
  }

  // ---- Web MIDI (the ports are chosen as 404jam does: the SP-404 wins as soon as it shows up) ----
  class Midi {
    constructor(onEvent, onChange) { this.onEvent = onEvent; this.onChange = onChange; this.access = null; this.out = null; this.inp = null; this.error = ''; }
    async connect() {
      if (!navigator.requestMIDIAccess) { this.error = 'This browser has no Web MIDI (use Chrome or Edge on a computer, or MIDIWeb Browser on an iPhone).'; return false; }
      try { this.access = await navigator.requestMIDIAccess({ sysex: false }); } catch (e) { this.error = 'MIDI access was refused: allow it in the browser.'; return false; }
      this.access.onstatechange = () => this.refresh();
      this.refresh();
      return true;
    }
    live(p) { return p && p.state !== 'disconnected'; }
    refresh() {
      const outs = [], ins = [];
      this.access.outputs.forEach((p) => { if (this.live(p)) outs.push(p); });
      this.access.inputs.forEach((p) => { if (this.live(p)) ins.push(p); });
      const sp = /sp-?404/i;
      this.out = outs.find((p) => sp.test(p.name)) || (outs.length === 1 ? outs[0] : this.out && outs.includes(this.out) ? this.out : null);
      this.inp = ins.find((p) => sp.test(p.name)) || (ins.length === 1 ? ins[0] : this.inp && ins.includes(this.inp) ? this.inp : null);
      ins.forEach((p) => { p.onmidimessage = p === this.inp ? (e) => this.message(e.data) : null; });
      this.onChange();
    }
    get name() { return this.out ? this.out.name : this.inp ? this.inp.name : ''; }
    get ok() { return !!this.out; }
    send(bytes) { if (!this.out) return false; try { this.out.send(bytes); return true; } catch (e) { return false; } }
    // A message may hold several, or realtime bytes between the bytes of others.
    message(d) {
      let i = 0;
      while (i < d.length) {
        const st = d[i];
        if (st >= 0xF8) { this.onEvent(realtime(st)); i++; continue; }
        if (st < 0x80) { i++; continue; }
        const kind = st & 0xF0, ch = (st & 0x0F) + 1;
        const n = kind === 0xC0 || kind === 0xD0 ? 1 : st >= 0xF0 ? 0 : 2;
        const body = [];
        for (let k = 1; k <= n && i + k < d.length && d[i + k] < 0x80; k++) body.push(d[i + k]);
        i += 1 + body.length;
        if (body.length < n) continue;
        const e = { ch, d1: body[0], d2: body[1] || 0 };
        if (kind === 0xB0) e.kind = 'cc'; else if (kind === 0x90) e.kind = e.d2 > 0 ? 'on' : 'off'; else if (kind === 0x80) e.kind = 'off'; else if (kind === 0xC0) e.kind = 'pc'; else continue;
        this.onEvent(e);
      }
    }
  }
  function realtime(st) { return { kind: st === 0xF8 ? 'clock' : st === 0xFA ? 'start' : st === 0xFB ? 'continue' : st === 0xFC ? 'stop' : 'other' }; }

  // ---- a unit that is not there, for a try: a clock, four buses that have said what they have, and a click for a pad ----
  // The demo unit: four banks of different kits (name, bus, voice, pitch) and the patterns of each (a row of 16 steps for every pad).
  const DEMO_BANKS = [
    [['KICK', 1, 'kick', 0], ['SNARE', 1, 'snare', 0], ['CLAP', 0, 'clap', 0], ['HAT', 2, 'hat', 0], ['OPEN HAT', 2, 'ohat', 0], ['RIM', 0, 'rim', 0], ['TOM LO', 0, 'tom', 110], ['TOM HI', 0, 'tom', 190],
      ['BASS', 0, 'bass', 55], ['STAB', 1, 'stab', 330], ['CHORD', 1, 'chord', 262], ['PERC', 0, 'perc', 520], ['SHAKER', 2, 'shaker', 0], ['FX', 2, 'zap', 0], ['VOX', 0, 'vox', 300], ['NOISE', 2, 'riser', 0]],
    [['CONGA LO', 0, 'conga', 190], ['CONGA HI', 0, 'conga', 270], ['BONGO LO', 0, 'conga', 340], ['BONGO HI', 0, 'conga', 460], ['CLAVE', 0, 'clave', 0], ['COWBELL', 1, 'cowbell', 0], ['GUIRO', 2, 'guiro', 0], ['AGOGO', 1, 'cowbell', 1],
      ['MARACAS', 2, 'shaker', 0], ['TAMBOURINE', 2, 'hat', 0], ['CABASA', 2, 'guiro', 1], ['TRIANGLE', 1, 'bell', 1500], ['WOODBLOCK', 0, 'clave', 1], ['SNAP', 0, 'clap', 0], ['SHAKER', 2, 'shaker', 0], ['VIBRASLAP', 1, 'guiro', 2]],
    [['SUB', 0, 'bass', 41], ['ACID', 1, 'acid', 110], ['PLUCK', 1, 'pluck', 330], ['WARM PAD', 1, 'chord', 220], ['STAB', 1, 'stab', 392], ['MINOR CHORD', 1, 'chord', 196], ['ARP', 2, 'arp', 440], ['LEAD', 1, 'lead', 523],
      ['RISER', 2, 'riser', 0], ['IMPACT', 0, 'impact', 0], ['ZAP', 2, 'zap', 0], ['NOISE', 2, 'riser', 1], ['VOX CHOP', 0, 'vox', 260], ['BELL', 1, 'bell', 880], ['ORGAN', 1, 'chord', 330], ['FX HIT', 2, 'impact', 1]],
    [['KICK 2', 1, 'kick', 1], ['SNARE 2', 1, 'snare', 1], ['GHOST', 1, 'snare', 2], ['HAT 2', 2, 'hat', 0], ['CRASH', 2, 'ohat', 1], ['RIDE', 2, 'bell', 3200], ['TOM', 0, 'tom', 150], ['BREAK', 0, 'perc', 400],
      ['RIM 2', 0, 'rim', 0], ['CLAP 2', 0, 'clap', 0], ['SHAKER 2', 2, 'shaker', 0], ['SCRATCH', 0, 'zap', 1], ['VOX', 0, 'vox', 340], ['BASS 2', 0, 'bass', 49], ['STAB 2', 1, 'stab', 294], ['FX', 2, 'riser', 0]],
  ];
  const R = (steps) => steps.replace(/\s+/g, '');
  const DEMO_PTNS = [
    [ // A: drums
      { 0: R('x...x...x...x...'), 1: R('....x.......x...'), 3: R('..x...x...x...x.'), 4: R('..............x.'), 8: R('x..x..x...x.....'), 11: R('.......x.......x') },
      { 0: R('x...x...x...x...'), 3: R('x.x.x.x.x.x.x.x.'), 2: R('....x.......x...'), 8: R('x.......x.......') },
      { 0: R('x.......x.x.....'), 1: R('....x.......x...'), 3: R('x.x.x.x.x.x.x.x.'), 5: R('.......x.......x') },
      { 0: R('x.....x...x.....'), 1: R('....x..x....x...'), 3: R('x.x.x.x.x.x.x.xx'), 4: R('......x.......x.'), 6: R('..............x.') },
      { 0: R('x..x..x...x..x..'), 2: R('....x.......x...'), 4: R('..x...x...x...x.'), 12: R('x.x.x.x.x.x.x.x.') },
      { 0: R('x...x...x...x...'), 1: R('................'), 2: R('....x.......x...'), 3: R('.x.x.x.x.x.x.x.x'), 11: R('..x...x...x...x.'), 8: R('x...........x...') },
      { 0: R('x.......x.......'), 1: R('....x.......x..x'), 3: R('x...x...x...x...'), 7: R('..........x.x...') },
      { 0: R('x..x.x..x..x.x..'), 1: R('....x.......x...'), 3: R('..x...x...x...x.'), 6: R('.............x.x'), 9: R('x.....x.........') },
    ],
    [ // B: latin
      { 0: R('x..x..x...x...x.'), 1: R('..x...x.x.....x.'), 2: R('....x.......x...'), 4: R('x..x..x...x..x..'), 8: R('x.x.x.x.x.x.x.x.') },
      { 0: R('x.......x.......'), 1: R('..x...x...x...x.'), 5: R('x..x..x.x..x..x.'), 6: R('....x.......x.x.') },
      { 2: R('x.xx.x.xx.xx.x.x'), 3: R('.x..x..x.x..x..x'), 11: R('x.......x.......'), 12: R('..x...x...x...x.') },
      { 0: R('x..x..x.....x...'), 1: R('......x...x.....'), 4: R('x..x..x...x...x.'), 5: R('..x.....x.....x.'), 7: R('x.......x.......'), 14: R('x.x.x.x.x.x.x.x.') },
      { 9: R('x.x.x.x.x.x.x.x.'), 10: R('.x..x..x.x..x..x'), 13: R('....x.......x...'), 0: R('x...x...x...x...') },
      { 2: R('x..x..x...x...x.'), 3: R('..x...x...x...x.'), 5: R('x.......x.......'), 15: R('......x.......x.') },
    ],
    [ // C: synths and bass
      { 0: R('x.......x.......'), 1: R('..x...x...x...x.'), 4: R('....x.......x...') },
      { 0: R('x..x..x...x..x..'), 6: R('x.x.x.x.x.x.x.x.'), 13: R('x...............') },
      { 0: R('x...x...x...x...'), 2: R('..x..x..x..x..x.'), 8: R('x...............'), 9: R('............x...') },
      { 3: R('x...............'), 5: R('........x.......'), 7: R('..x...x...x.x...'), 12: R('....x.......x...') },
      { 0: R('x.x...x.x.x...x.'), 1: R('.x...x...x...x..'), 10: R('x...............'), 14: R('....x.......x...') },
    ],
    [ // D: breaks
      { 0: R('x.....x...x.....'), 1: R('....x.......x...'), 2: R('.......x...x..x.'), 3: R('x.x.x.x.x.x.x.x.'), 4: R('..............x.') },
      { 0: R('x..x......x.....'), 1: R('....x..x....x..x'), 3: R('x.xxx.x.x.xxx.x.'), 5: R('x...............') },
      { 0: R('x.x.....x.x.....'), 1: R('....x.......x...'), 3: R('x...x...x...x...'), 7: R('..x.....x.....x.') },
      { 0: R('x......x..x.....'), 1: R('....x.......x...'), 2: R('.....x.....x...x'), 3: R('x.x.x.x.x.x.x.x.'), 11: R('...........x....') },
      { 0: R('x.x.......x.....'), 1: R('....x.......x...'), 3: R('.x.x.x.x.x.x.x.x'), 6: R('..............x.'), 13: R('x.......x.......') },
      { 0: R('x...x...x...x...'), 1: R('..........x.....'), 3: R('x.x.x.x.x.x.x.x.'), 8: R('....x.......x...') },
    ],
  ];
  const demoKit = (bank, i) => (DEMO_BANKS[bank] || [])[i] || null;
  const demoPtn = (bank, i) => (DEMO_PTNS[bank] || [])[i] || null;
  // What the demo unit holds. At first it is the data above; once the tools (demo.wasm) are running it is what that unit holds (the
  // pads with their names and samples, the patterns with their notes), so a bounced pad or a pattern tool's notes show in the Pads and
  // Patterns pages, and are played. Patterns are kept as notes by clock tick (24 to a beat).
  // One audio context for the whole page (the pads, the tools in the frame): a browser lets only a touch unlock it, once.
  const sharedCtx = () => { window.__sp404ctx = window.__sp404ctx || new (window.AudioContext || window.webkitAudioContext)(); return window.__sp404ctx; };
  window.SP404SharedCtx = sharedCtx;
  const unlockCtx = (ctx) => { // the old iOS trick: a silent sound started in the touch makes the context run
    try { const b = ctx.createBuffer(1, 1, 22050), s = ctx.createBufferSource(); s.buffer = b; s.connect(ctx.destination); s.start(0); } catch (e) {}
    return ctx.state === 'running' ? Promise.resolve() : ctx.resume();
  };
  window.SP404UnlockCtx = unlockCtx;
  const DU = { pads: null, pat: new Map(), buf: new Map(), fromUnit: false };
  const duFill = (bank, i, steps) => { // a pattern of the data above in the same form
    const map = new Map(), pads = new Set();
    for (const k in steps) { pads.add(bank * 16 + +k); for (let s = 0; s < 16; s++) if (steps[k][s] === 'x') { const t = s * 6; map.set(t, (map.get(t) || []).concat([[bank * 16 + +k, 100]])); } }
    return { exists: true, len: 96, map, pads };
  };
  DEMO_PTNS.forEach((list, b) => list.forEach((steps, i) => DU.pat.set(b * 16 + i, duFill(b, i, steps))));
  const duPad = (bank, i) => {
    const k = demoKit(bank, i);
    if (DU.pads) { const p = DU.pads[bank * 16 + i]; return p && p.has ? { name: p.name, bus: k && k[0] === p.name ? k[1] : 0 } : null; }
    return k ? { name: k[0], bus: k[1] } : null;
  };
  const duPtn = (bank, i) => { const d = DU.pat.get(bank * 16 + i); return d && d.exists ? d : null; };
  const duSetPattern = (slot, o) => {
    if (!o || !o.exists) { DU.pat.set(slot, { exists: false }); return; }
    const len = Math.max(1, Math.round(o.length / 20)), map = new Map(), pads = new Set();
    for (const [tick, pad, vel] of o.notes) { const t = Math.round(tick / 20) % len; map.set(t, (map.get(t) || []).concat([[pad, vel]])); pads.add(pad); }
    DU.pat.set(slot, { exists: true, len, map, pads });
  };
  class Demo {
    constructor(onEvent, onChange) { this.onEvent = onEvent; this.onChange = onChange; this.ok = true; this.name = 'demo'; this.error = ''; this.ctx = null; this.running = true; this.where = () => ({ bank: 0, i: 0 }); }
    // The play and stop of the demo's clock (a real unit is started on the unit).
    toggle() { this.running = !this.running; this.onEvent({ kind: this.running ? 'start' : 'stop' }); if (!this.running) this.timeNow = 0; return this.running; }
    async connect() {
      this.onChange();
      setTimeout(() => {
        // What the unit says when something is changed on it: the effects and some knobs.
        const set = (ch, cc, v) => this.onEvent({ kind: 'cc', ch, d1: cc, d2: v });
        [[1, 16, 1, 57], [2, 22, 1, 30], [3, 20, 1, 80], [4, 35, 0, 40]].forEach(([ch, fx, on, k]) => {
          set(ch, CC_FX, fx); set(ch, CC_SW, on ? 127 : 0);
          [16, 17, 18, 80, 81, 82].forEach((cc, i) => set(ch, cc, clamp(k + i * 13, 0, 127)));
        });
      }, 400);
      this.unlockOnce = () => this.unlock();
      ['pointerdown', 'touchend', 'click', 'keydown'].forEach((t) => document.addEventListener(t, this.unlockOnce)); // iOS counts touchend and click, not pointerdown
      if (!this.hint) {
        this.hint = document.createElement('div'); this.hint.textContent = 'Tap anywhere to turn the sound on';
        this.hint.style.cssText = 'position:fixed;left:50%;bottom:calc(76px + env(safe-area-inset-bottom));transform:translateX(-50%);background:#ffb000;color:#161618;font:700 14px system-ui;padding:10px 16px;border-radius:12px;z-index:50;pointer-events:none';
        document.body.appendChild(this.hint);
      }
      let t = performance.now(), n = 0;
      this.onEvent({ kind: 'start' });
      this.timer = setInterval(() => {
        const per = 60000 / 125 / 24, now = performance.now();
        if (!this.running) { t = now; return; }
        while (t + per <= now) {
          t += per; this.onEvent({ kind: 'clock' }); n++;
          // the pattern that plays: its pads hit on their notes (the unit sends no notes for a pattern, the demo lets the pads flash)
          const w = this.where(), d = duPtn(w.bank, w.i);
          if (d) {
            const hits = d.map.get((n - 1) % d.len);
            if (hits) for (const [pad, vel] of hits) { this.hit(pad, vel); this.onEvent({ kind: 'on', ch: (pad >> 4) + 1, d1: 36 + PAD_OFFSET[pad & 15], d2: vel }); }
          }
        }
      }, 10);
      return true;
    }
    send(bytes) {
      if ((bytes[0] & 0xF0) === 0x90 && bytes[2] > 0) { const p = padOfNote(bytes[1]); if (p) this.hit((bytes[0] & 15) * 16 + p - 1, bytes[2]); else this.blip(bytes[1]); }
      return true;
    }
    // The sound is on after the first touch (a browser keeps a page silent until then).
    unlock() {
      try {
        if (navigator.audioSession) navigator.audioSession.type = 'playback'; // iOS 17: sound although the ringer is on silent
        if (!this.keep) { // an audio element that plays nothing keeps older iOS in the playback mode too
          this.keep = new Audio('data:audio/wav;base64,UklGRiQAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQAAAAA='); this.keep.loop = true; this.keep.play().catch(() => {});
        }
        this.ctx = sharedCtx();
        const done = () => {
          if (this.ctx.state === 'running') {
            if (this.hint) { this.hint.remove(); this.hint = null; }
            ['pointerdown', 'touchend', 'click', 'keydown'].forEach((t) => document.removeEventListener(t, this.unlockOnce));
            this.hit(0, 100); // a first sound, so that it is clear it works
          } else if (this.hint) this.hint.textContent = 'No sound yet: tap again (and switch the ringer on)';
        };
        unlockCtx(this.ctx).then(done, done);
      } catch (e) { /* no audio: the demo is silent */ }
    }
    // A pad is hit: its sample from the unit when the tools have it, else a sound made of oscillators and noise.
    hit(pad, vel) {
      const ctx = this.ctx; if (!ctx || ctx.state !== 'running') return;
      const p = DU.pads && DU.pads[pad];
      if (p) {
        if (!p.has) return;
        const b = this.buffer(pad, p);
        if (b) { const s = ctx.createBufferSource(), g = ctx.createGain(); s.buffer = b; g.gain.value = Math.min(1, (vel || 100) / 110); s.connect(g); g.connect(ctx.destination); s.start(); return; }
      }
      this.voice(pad >> 4, pad & 15);
    }
    buffer(pad, p) {
      const key = pad + '/' + (p.secs || 0);
      if (DU.buf.has(key)) return DU.buf.get(key);
      if (!window.sp404PadAudio) return null;
      DU.buf.set(key, null); // asked for once; the buffer comes when it is read
      const o = window.sp404PadAudio(pad);
      if (!o) return null;
      const n = o.pcm.length >> 1, ch = o.channels, frames = Math.floor(n / ch), buf = this.ctx.createBuffer(ch, frames, o.rate), v = new DataView(o.pcm.buffer, o.pcm.byteOffset, o.pcm.length);
      for (let c = 0; c < ch; c++) { const d = buf.getChannelData(c); for (let i = 0; i < frames; i++) d[i] = v.getInt16((i * ch + c) * 2, true) / 32768; }
      DU.buf.set(key, buf);
      return buf;
    }
    // A pad of the demo kit, made of oscillators and noise: a kick, a snare, hats and so on.
    voice(bank, i) {
      const ctx = this.ctx, kit = demoKit(bank, i); if (!ctx || ctx.state !== 'running' || !kit) return;
      const t = ctx.currentTime, [, , v, hz] = kit;
      const out = ctx.createGain(); out.gain.value = 0.9; out.connect(ctx.destination);
      const env = (g, peak, len) => { g.gain.setValueAtTime(peak, t); g.gain.exponentialRampToValueAtTime(0.0008, t + len); };
      const osc = (type, f0, f1, len, peak) => { const o = ctx.createOscillator(), g = ctx.createGain(); o.type = type; o.frequency.setValueAtTime(f0, t); if (f1) o.frequency.exponentialRampToValueAtTime(f1, t + len); env(g, peak, len); o.connect(g); g.connect(out); o.start(t); o.stop(t + len + 0.05); };
      const noise = (len, peak, hp, bp) => {
        const buf = ctx.createBuffer(1, Math.ceil(ctx.sampleRate * len), ctx.sampleRate), d = buf.getChannelData(0);
        for (let k = 0; k < d.length; k++) d[k] = Math.random() * 2 - 1;
        const s = ctx.createBufferSource(), f = ctx.createBiquadFilter(), g = ctx.createGain();
        s.buffer = buf; f.type = bp ? 'bandpass' : 'highpass'; f.frequency.value = bp || hp; env(g, peak, len); s.connect(f); f.connect(g); g.connect(out); s.start(t);
      };
      switch (v) {
        case 'kick': osc('sine', hz ? 120 : 160, hz ? 38 : 45, hz ? 0.5 : 0.35, 0.9); break;
        case 'snare': osc('triangle', hz === 2 ? 300 : 220, 160, 0.14, hz === 2 ? 0.15 : 0.4); noise(hz === 2 ? 0.1 : 0.2, hz === 2 ? 0.25 : 0.5, 1500); break;
        case 'clap': noise(0.18, 0.6, 0, 1400); break;
        case 'hat': noise(0.06, 0.3, 7000); break;
        case 'ohat': noise(hz ? 0.7 : 0.3, 0.25, 6000); break;
        case 'rim': osc('square', 900, 700, 0.04, 0.25); break;
        case 'tom': osc('sine', hz, hz * 0.6, 0.3, 0.6); break;
        case 'bass': osc('sawtooth', hz, 0, 0.3, 0.35); osc('sine', hz, 0, 0.45, 0.4); break;
        case 'stab': osc('sawtooth', hz, 0, 0.18, 0.2); osc('sawtooth', hz * 1.26, 0, 0.18, 0.2); break;
        case 'chord': [1, 1.26, 1.5].forEach((m) => osc('triangle', hz * m, 0, 0.7, 0.15)); break;
        case 'perc': osc('square', hz, hz * 0.6, 0.08, 0.25); break;
        case 'shaker': noise(0.09, 0.2, 5000); break;
        case 'zap': osc('sawtooth', hz ? 1800 : 200, hz ? 100 : 1600, hz ? 0.15 : 0.5, 0.15); break;
        case 'vox': osc('triangle', hz, hz * 0.85, 0.35, 0.3); osc('triangle', hz * 2, hz * 1.7, 0.3, 0.1); break;
        case 'riser': noise(hz ? 0.35 : 0.8, 0.2, hz ? 2500 : 900); break;
        case 'conga': osc('sine', hz, hz * 0.75, 0.2, 0.7); noise(0.02, 0.2, 2000); break;
        case 'clave': osc('sine', hz ? 1700 : 2400, 0, 0.06, 0.4); break;
        case 'cowbell': osc('square', hz ? 800 : 560, 0, 0.25, 0.15); osc('square', hz ? 1190 : 845, 0, 0.25, 0.15); break;
        case 'guiro': noise(hz === 2 ? 0.25 : 0.12, 0.25, 0, hz === 1 ? 5000 : 3000); break;
        case 'bell': osc('sine', hz, 0, 0.9, 0.25); osc('sine', hz * 2.76, 0, 0.6, 0.12); break;
        case 'pluck': osc('sawtooth', hz, 0, 0.2, 0.25); break;
        case 'acid': osc('sawtooth', hz, hz * 0.9, 0.22, 0.3); break;
        case 'arp': osc('square', hz, 0, 0.1, 0.15); break;
        case 'lead': osc('square', hz, 0, 0.3, 0.15); osc('square', hz * 1.005, 0, 0.3, 0.1); break;
        case 'impact': osc('sine', hz ? 160 : 70, hz ? 60 : 35, 0.6, 0.8); noise(0.3, 0.3, 300); break;
        default: noise(0.3, 0.2, 800);
      }
    }
  }

  // ---- the engine ----
  class Engine {
    constructor(opts) {
      this.demo = !!opts.demo;
      this.listeners = [];
      this.msg = '';
      this.bank = 0;
      this.clock = { ticks: 0, running: false, at: 0, stamps: [], bpm: 0 };
      this.fx = [0, 1, 2, 3].map(() => ({ on: false, onSeen: false, num: 0, numSeen: false }));
      this.ccs = new Map();
      this.lastHit = new Map(); // bank*16+pad -> when
      this.mem = this.load();
      this.recallFx();
      this.sc = { bars: 2, overdub: false, bus: [0, 1].map(() => ({ sel: 0, mo: [0, 1, 2, 3].map(() => newMotion()) })),
        punch: { held: 0, was: emptySnap(), arm: false, sel: 0, edit: false, morph: false, active: 0, mix: 0, from: emptySnap(), sent: emptySnap() },
        lfo: [0, 1, 2, 3].map(() => newLfoRun()), lastSent: '' };
      this.ptn = { now: this.demo ? 0 : -1, nowBank: this.demo ? 0 : -1, next: -1, bank: -1, dueTick: 0 };
      this.dirty = false;
      this.midi = this.demo ? new Demo((e) => this.input(e), () => this.changed()) : new Midi((e) => this.input(e), () => this.changed());
      if (this.demo) this.midi.where = () => ({ bank: Math.max(this.ptn.nowBank, 0), i: Math.max(this.ptn.now, 0) });
    }
    async start() {
      const ok = await this.midi.connect();
      this.changed();
      this.timer = setInterval(() => { if (this.dirty) this.publish(); }, 50);
      return ok;
    }
    // A socket as the page expects one.
    socket() {
      const eng = this;
      const sock = {
        readyState: 1,
        onopen: null, onmessage: null, onclose: null, onerror: null,
        send(text) { let c; try { c = JSON.parse(text); } catch (e) { return; } if (c.a === 'ping') { sock.onmessage && sock.onmessage({ data: JSON.stringify({ pong: c.t }) }); return; } eng.command(c); },
        close() { eng.listeners = eng.listeners.filter((l) => l !== sock); sock.readyState = 3; },
      };
      this.listeners.push(sock);
      setTimeout(() => { sock.onopen && sock.onopen(); this.publish(); }, 0);
      return sock;
    }
    changed() { this.dirty = true; }
    publish() {
      this.dirty = false;
      const text = JSON.stringify(this.snapshot());
      this.listeners.forEach((l) => l.onmessage && l.onmessage({ data: text }));
    }

    // ---- what is kept between visits ----
    load() {
      let m = {};
      try { m = JSON.parse(localStorage.getItem(LS) || '{}') || {}; } catch (e) { /* fresh */ }
      m.punch = (m.punch && m.punch.length === 4 ? m.punch : emptySnap().map(() => 0)).map((s) => (s && s.length === 4 ? s : emptySnap()));
      m.lfo = m.lfo && m.lfo.length === 4 ? m.lfo : [0, 1, 2, 3].map(() => ({ set: false, bus: 0, knob: 0, shape: 0, rate: 4, depth: 50, shift: 0 }));
      m.busFx = m.busFx && m.busFx.length === 4 ? m.busFx : [0, 1, 2, 3].map(() => ({ num: 0, on: 0 }));
      return m;
    }
    save() { try { localStorage.setItem(LS, JSON.stringify(this.mem)); } catch (e) { /* private mode */ } }
    recallFx() {
      this.mem.busFx.forEach((r, b) => {
        const f = this.fx[b];
        if (r.num > 0) { f.num = r.num - 1; f.numSeen = true; }
        if (r.on > 0) { f.on = r.on === 2; f.onSeen = true; }
      });
    }
    rememberFx(bus) {
      const f = this.fx[bus], r = this.mem.busFx[bus];
      if (f.numSeen) r.num = f.num + 1;
      if (f.onSeen) r.on = f.on ? 2 : 1;
      this.save();
    }

    // ---- sending ----
    sendMidi(bytes) {
      if (!this.midi.send(bytes)) return false;
      if (bytes.length === 3 && (bytes[0] & 0xF0) === 0xB0) {
        const cc = bytes[1];
        if ((cc >= 16 && cc <= 18) || (cc >= 80 && cc <= 82)) this.ccs.set(((bytes[0] & 15) + 1) + ',' + cc, bytes[2]);
        this.sc.lastSent = 'channel ' + ((bytes[0] & 15) + 1) + ', controller ' + cc + ' = ' + bytes[2];
      }
      return true;
    }
    cc(bus1, cc) { const v = this.ccs.get(bus1 + ',' + cc); return v === undefined ? null : v; }

    // ---- input ----
    input(e) {
      const clk = this.clock;
      switch (e.kind) {
        case 'clock': {
          clk.ticks++; clk.running = true;
          const now = performance.now(); clk.at = now;
          clk.stamps.push(now); if (clk.stamps.length > 49) clk.stamps.shift();
          if (clk.stamps.length > 12 && clk.ticks % 24 === 0) { clk.bpm = 60000 / ((clk.stamps[clk.stamps.length - 1] - clk.stamps[0]) / (clk.stamps.length - 1) * 24); }
          this.lfoTick(); this.motionTick(); this.ptnTick();
          this.changed();
          return;
        }
        case 'start': clk.ticks = 0; clk.running = true; clk.at = performance.now(); clk.stamps = []; this.changed(); return;
        case 'continue': clk.running = true; clk.at = performance.now(); this.changed(); return;
        case 'stop': clk.running = false; this.lfoStop(); this.changed(); return;
        case 'cc':
          if (e.ch >= 1 && e.ch <= 4 && ((e.d1 >= 16 && e.d1 <= 18) || (e.d1 >= 80 && e.d1 <= 82))) this.ccs.set(e.ch + ',' + e.d1, e.d2);
          this.motionIn(e); this.fxEvent(e);
          break;
        case 'on': {
          if (e.ch === MORPH_CHAN) break;
          const pad = padOfNote(e.d1);
          if (pad && e.ch >= 1 && e.ch <= 10) this.lastHit.set((e.ch - 1) * 16 + pad - 1, performance.now());
          break;
        }
      }
      if (e.ch === MORPH_CHAN) this.morphEvent(e);
      this.changed();
    }
    fxEvent(e) {
      if (e.kind !== 'cc' || e.ch < 1 || e.ch > 4) return;
      const bus = e.ch - 1, f = this.fx[bus];
      if (e.d1 === CC_SW) { f.on = e.d2 >= 64; f.onSeen = true; }
      else if (e.d1 === CC_FX) { f.num = e.d2; f.numSeen = true; }
      else return;
      this.rememberFx(bus);
    }

    // ---- commands from the page ----
    command(c) {
      const bus = clamp(c.bus | 0, 0, 1);
      switch (c.a) {
        case 'pad': this.padHit(c.i); break;
        case 'bank': if (c.i >= 0 && c.i < 10) { this.bank = c.i; } break;
        case 'view': break;
        case 'ptn': this.ptnSend(c.i); break;
        case 'ptnbank': this.ptn.bank = ((this.ptnBank() + c.i) % 10 + 10) % 10; break;
        case 'ptnbankset': this.ptn.bank = clamp(c.i | 0, 0, 9); break;
        case 'ptnfollow': this.ptn.bank = -1; break;
        case 'motion': this.motionAct(bus, c.slot | 0); break;
        case 'punch': this.punchAct(c.n === 1 ? 'down' : 'up', c.i | 0); break;
        case 'punchpreset': this.punchPreset(c.i | 0, c.n | 0); break;
        case 'morph': this.morphSet(c.n | 0); break;
        case 'fxdeclare': this.fxDeclare(c.i | 0, c.name, c.n | 0); break;
        case 'lfoset': this.lfoSet(c.i | 0, c.name, c.n | 0); break;
        case 'lfoon': this.lfoToggle(c.i | 0); break;
        case 'punchmode': this.punchAct('mode', c.n | 0); break;
        case 'punchsave': this.punchAct('save', 0); break;
        case 'mclear': this.motionClear(bus); break;
        case 'mspeed': this.motionSpeed(bus); break;
        case 'mrev': this.motionReverse(bus); break;
        case 'bars': if ([1, 2, 3, 4, 8].includes(c.n)) { this.sc.bars = c.n; } break;
        case 'dub': this.motionOverdub(); break;
        default: break; // what needs the unit's serial link (levels, mute, record, Looper, Slicer) is not here
      }
      this.changed();
    }
    padHit(i) {
      if (i < 0 || i > 15) return;
      const note = 36 + PAD_OFFSET[i], ch = this.bank & 15;
      this.lastHit.set(this.bank * 16 + i, performance.now());
      this.sendMidi([0x90 | ch, note, 110]);
      setTimeout(() => this.sendMidi([0x80 | ch, note, 0]), 120);
    }

    // ---- Patterns: a Program Change on the channel of the bank, the number of the pattern ----
    ptnBank() { return this.ptn.bank >= 0 ? this.ptn.bank : this.bank; }
    ptnSend(i) {
      if (i < 0 || i > 15) return;
      const bank = this.ptnBank();
      if (!this.sendMidi([0xC0 | bank, i])) { this.msg = 'the pattern was not sent: no MIDI output to the unit'; return; }
      this.ptn.next = i; this.ptn.nextBank = bank;
      // The unit says nothing: it takes over at the end of the pattern, which is not known here; the bar is the nearest guess.
      this.ptn.dueTick = (Math.floor(this.clock.ticks / TICKS_BAR) + 1) * TICKS_BAR;
      this.msg = 'pattern ' + String.fromCharCode(65 + bank) + (i + 1) + ' is sent: the unit plays it next (the unit needs PC Rx on)';
    }
    ptnTick() {
      if (this.ptn.next >= 0 && this.clock.ticks >= this.ptn.dueTick) { this.ptn.now = this.ptn.next; this.ptn.nowBank = this.ptn.nextBank; this.ptn.next = -1; }
    }

    // ---- fxDeclare: what the unit has said only when it changes, told by hand; nothing is sent ----
    fxDeclare(bus, what, n) {
      if (bus < 0 || bus > 3) return;
      const f = this.fx[bus];
      if (what === 'num') {
        if (n < 0 || n >= fxTable(bus).length) return;
        f.num = n; f.numSeen = true; this.msg = 'BUS ' + (bus + 1) + ': ' + fxNameOf(bus, n) + ' noted (nothing sent to the unit)';
      } else if (what === 'on') {
        f.on = n === 1; f.onSeen = true; this.msg = 'BUS ' + (bus + 1) + ': noted as ' + (n === 1 ? 'on' : 'off') + ' (nothing sent to the unit)';
      } else return;
      this.rememberFx(bus);
    }

    // ---- Motion (tui/motion.go) ----
    motionBusy(bus) { return bus < 2 && this.sc.bus[bus].mo.some((m) => m.state !== 0); }
    motionAct(bus, slot) {
      const sc = this.sc, bs = sc.bus[bus];
      if (slot < 0 || slot >= SLOTS) return;
      bs.sel = slot;
      const mo = bs.mo[slot], running = this.clock.running;
      if (mo.state === 4 || mo.state === 3) { mo.state = 0; this.msg = 'BUS ' + (bus + 1) + ': motion ' + (slot + 1) + ' stopped'; }
      else if (mo.state === 1 || mo.state === 2) { bs.mo[slot] = newMotion(); this.msg = 'BUS ' + (bus + 1) + ': the recording into slot ' + (slot + 1) + ' was cancelled'; }
      else if (!running) this.msg = "motion follows the unit's MIDI clock: start its sequencer";
      else if (motionFilled(mo)) { this.stopMotions(bs); mo.state = 3; mo.pos = 0; this.msg = 'BUS ' + (bus + 1) + ': motion ' + (slot + 1) + ' plays from the next bar'; }
      else { this.stopMotions(bs); bs.mo[slot] = Object.assign(newMotion(), { state: 1, total: sc.bars * TICKS_BAR }); this.msg = 'BUS ' + (bus + 1) + ': motion ' + (slot + 1) + ' records ' + plural(sc.bars, 'bar') + ' from the next bar: turn C1 to C3 on the unit'; }
    }
    stopMotions(bs) { bs.mo.forEach((m) => { if (m.state === 4 || m.state === 3) m.state = 0; }); }
    motionOverdub() { this.sc.overdub = !this.sc.overdub; this.msg = this.sc.overdub ? 'overdub on: move C1 to C3 on the unit while a motion plays and the moves are added to it' : 'overdub off'; }
    motionClear(bus) { const bs = this.sc.bus[bus]; bs.mo[bs.sel] = newMotion(); this.msg = 'BUS ' + (bus + 1) + ': motion ' + (bs.sel + 1) + ' cleared'; }
    motionSpeed(bus) { const bs = this.sc.bus[bus], mo = bs.mo[bs.sel]; mo.speed = { 0: 2, 1: 2, 2: 0.5, 0.5: 1 }[mo.speed] || 1; }
    motionReverse(bus) { const bs = this.sc.bus[bus], mo = bs.mo[bs.sel]; mo.rev = !mo.rev; }
    motionIn(e) {
      if (e.kind !== 'cc' || e.ch < 1 || e.ch > 2 || e.d1 < 16 || e.d1 > 18) return;
      const bs = this.sc.bus[e.ch - 1];
      if (this.sc.overdub) {
        const playing = bs.mo.find((m) => m.state === 4);
        if (playing) { overdub(playing, e.d1 - 16, e.d2); return; }
      }
      const mo = bs.mo[bs.sel];
      if (mo.state !== 2) return;
      mo.events.push({ tick: mo.pos, knob: e.d1 - 16, val: e.d2 });
    }
    motionTick() {
      for (let bus = 0; bus < 2; bus++) {
        this.sc.bus[bus].mo.forEach((mo, slot) => {
          switch (mo.state) {
            case 1:
              if (this.clock.ticks % TICKS_BAR !== 0) return;
              mo.state = 2; mo.pos = 0;
              for (let k = 0; k < 3; k++) { const v = this.cc(bus + 1, 16 + k); if (v !== null) mo.events.push({ tick: 0, knob: k, val: v }); }
              break;
            case 2:
              mo.pos++;
              if (mo.pos >= mo.total) {
                if (!motionFilled(mo)) { this.sc.bus[bus].mo[slot] = newMotion(); return; }
                mo.state = 4; this.motionStart(bus, slot);
              }
              break;
            case 3:
              if (this.clock.ticks % TICKS_BAR === 0) { mo.state = 4; this.motionStart(bus, slot); }
              break;
            case 4: this.motionStep(bus, slot); break;
          }
        });
      }
    }
    motionStart(bus, slot) { const mo = this.sc.bus[bus].mo[slot]; mo.fpos = mo.rev ? mo.total : 0; this.motionStep(bus, slot); }
    motionStep(bus, slot) {
      const mo = this.sc.bus[bus].mo[slot], total = mo.total, from = mo.fpos, rate = mo.speed > 0 ? mo.speed : 1;
      for (let k = 0; k < 3; k++) mo.touch[k] = Math.max(mo.touch[k] - 1, 0);
      if (mo.rev) {
        let to = from - rate;
        if (to < 0) { this.motionSend(bus, slot, 0, from, true); to += total; this.motionSend(bus, slot, to, total, true); }
        else this.motionSend(bus, slot, to, from, true);
        mo.fpos = to; return;
      }
      let to = from + rate;
      if (to >= total) { this.motionSend(bus, slot, from, total, false); to -= total; this.motionSend(bus, slot, 0, to, false); }
      else this.motionSend(bus, slot, from, to, false);
      mo.fpos = to;
    }
    motionSend(bus, slot, lo, hi, back) {
      const mo = this.sc.bus[bus].mo[slot];
      const send = (ev) => { if (mo.touch[ev.knob] > 0) return; this.sendMidi([0xB0 | bus, 16 + ev.knob, clamp(ev.val, 0, 127)]); };
      if (back) { for (let i = mo.events.length - 1; i >= 0; i--) { const t = mo.events[i].tick; if (t >= lo && t < hi) send(mo.events[i]); } return; }
      mo.events.forEach((ev) => { if (ev.tick >= lo && ev.tick < hi) send(ev); });
    }

    // ---- Punch (tui/punch.go) ----
    captureBus(bus) {
      const f = this.fx[bus], p = emptyBus();
      if (f.onSeen) p.on = f.on ? 1 : 0;
      if (f.numSeen) p.num = f.num;
      PUNCH_CCS.forEach((cc, k) => { const v = this.cc(bus + 1, cc); if (v !== null) p.knobs[k] = v; });
      p.set = p.on >= 0 || p.num >= 0 || p.knobs.some((v) => v >= 0);
      return p;
    }
    capturePunch() { return [0, 1, 2, 3].map((b) => this.captureBus(b)); }
    punchSend(bus, p, last) {
      const f = this.fx[bus];
      if (p.num >= 0 && (!last || last.num !== p.num)) {
        this.sendMidi([0xB0 | bus, CC_FX, p.num]); f.num = p.num; f.numSeen = true; if (last) last.num = p.num;
      }
      p.knobs.forEach((v, k) => {
        if (v >= 0 && (!last || last.knobs[k] !== v)) { this.sendMidi([0xB0 | bus, PUNCH_CCS[k], clamp(v, 0, 127)]); if (last) last.knobs[k] = v; }
      });
      if (p.on >= 0 && (!last || last.on !== p.on)) {
        this.sendMidi([0xB0 | bus, CC_SW, p.on === 1 ? 127 : 0]); f.on = p.on === 1; f.onSeen = true; if (last) last.on = p.on;
      }
    }
    punchKnown(bus, p) { const f = this.fx[bus]; return (p.num < 0 || f.numSeen) && (p.on < 0 || f.onSeen); }
    punchBusy(bus) { return this.motionBusy(bus); }
    punchAct(act, slot) {
      const run = this.sc.punch;
      switch (act) {
        case 'save':
          run.arm = !run.arm;
          this.msg = run.arm ? 'SAVE: tap a snapshot to store the effects of all four buses in it' : '';
          return;
        case 'mode':
          run.morph = slot === 1; run.edit = slot === 2;
          this.msg = slot === 1 ? 'MORPH: tap a snapshot, then the slider (or the encoder) moves to it; tap it again to stop'
            : slot === 2 ? 'EDIT: hold a snapshot and turn the knobs on the unit; let go to keep what you changed in it' : 'HOLD: hold a snapshot to punch it';
          return;
        case 'down': {
          if (slot < 0 || slot > 3) return;
          run.sel = slot + 1;
          if (run.morph && !run.arm) { this.morphStart(slot); return; }
          if (run.arm) { run.arm = false; this.mem.punch[slot] = this.capturePunch(); this.save(); this.msg = 'PUNCH ' + (slot + 1) + ' saved: ' + punchText(this.mem.punch[slot]); return; }
          if (run.held !== 0) { if (run.held === slot + 1) this.punchUp(); return; }
          const snap = this.mem.punch[slot];
          if (!snapSet(snap)) { this.msg = 'PUNCH ' + (slot + 1) + ' is empty: tap SAVE, then it'; return; }
          run.held = slot + 1; run.was = this.lfoBase(this.capturePunch());
          const unknown = [];
          snap.forEach((p, b) => {
            if (!p.set || this.punchBusy(b)) return;
            if (!this.punchKnown(b, p)) unknown.push(b + 1);
            this.punchSend(b, p, null);
          });
          this.msg = 'PUNCH ' + (slot + 1) + ' held: let go to go back';
          if (unknown.length) this.msg = 'PUNCH ' + (slot + 1) + ' held. BUS ' + unknown.join(', ') + ': the app does not know what it had, so it cannot be put back exactly (tap its chip at the top and tell it)';
          return;
        }
        case 'up':
          if (run.morph) return;
          if (run.held === slot + 1) this.punchUp();
      }
    }
    punchUp() {
      const run = this.sc.punch;
      if (run.held === 0) return;
      const snap = cloneSnap(this.mem.punch[run.held - 1]);
      let text = 'let go';
      if (run.edit) {
        let n = 0; const cur = this.capturePunch();
        snap.forEach((s, b) => { const [merged, changed] = punchMerge(s, cur[b], run.was[b]); if (changed) { snap[b] = merged; n++; } });
        this.mem.punch[run.held - 1] = snap; this.save();
        text = n > 0 ? 'let go, ' + plural(n, 'bus') + ' kept: ' + punchText(snap) : 'let go, nothing changed';
      }
      snap.forEach((p, b) => {
        if ((p.set || run.edit) && !this.punchBusy(b)) {
          const back = { set: run.was[b].set, on: run.was[b].on, num: run.was[b].num, knobs: run.was[b].knobs.slice() };
          const numLost = p.num >= 0 && back.num < 0, onLost = p.on === 1 && back.on < 0;
          if (onLost) back.on = 0; // an effect the snapshot switched on that was not known to be on goes off
          this.punchSend(b, back, null);
          // What the app does not know of the bus stays unknown: it only sent the snapshot's values.
          const f = this.fx[b];
          if (numLost) f.numSeen = false;
          if (onLost) f.onSeen = false;
        }
      });
      this.msg = 'PUNCH ' + run.held + ' ' + text;
      run.held = 0;
    }
    punchPreset(preset, slot) {
      const p = FX.presets[preset];
      if (!p || slot < 0 || slot > 3) return;
      if (this.sc.punch.held !== 0) this.punchUp();
      this.mem.punch[slot] = p.buses.map((b, i) => (b.fx ? { set: true, on: 1, num: fxNum(i, b.fx), knobs: b.k.slice() } : emptyBus()));
      this.save();
      this.msg = 'PUNCH ' + (slot + 1) + ': ' + p.name;
    }

    // ---- Morph (tui/morph.go) ----
    morphStart(slot) {
      const run = this.sc.punch;
      if (run.active === slot + 1) { run.active = 0; this.msg = 'MORPH ' + (slot + 1) + ' stopped where it is'; return; }
      if (!snapSet(this.mem.punch[slot])) { this.msg = 'PUNCH ' + (slot + 1) + ' is empty: tap SAVE, then it'; return; }
      run.active = slot + 1; run.mix = 0;
      run.from = this.lfoBase(this.capturePunch()); run.sent = cloneSnap(run.from);
      const unknown = [];
      this.mem.punch[slot].forEach((p, b) => { if (p.set && !this.punchBusy(b) && !this.punchKnown(b, p)) unknown.push(b + 1); });
      this.msg = unknown.length ? 'MORPH to ' + (slot + 1) + '. BUS ' + unknown.join(', ') + ': the app does not know what it has, so it cannot go back to it exactly (tap its chip at the top and tell it)' : 'MORPH to ' + (slot + 1) + ': move the slider or the encoder';
    }
    morphSet(mix) {
      const run = this.sc.punch;
      if (run.active === 0) return;
      run.mix = clamp(mix, 0, 127);
      this.mem.punch[run.active - 1].forEach((to, b) => {
        if (!to.set || this.punchBusy(b)) return;
        this.punchSend(b, morphBus(run.from[b], to, run.mix), run.sent[b]);
      });
    }
    morphEvent(e) {
      const run = this.sc.punch;
      if (e.kind === 'on') { const i = e.d1 - EC4_FIRST_NOTE; if (i >= 0 && i < 4) this.morphStart(i); return; }
      if (e.kind !== 'cc') return;
      const i = e.d1 - EC4_FIRST_CC;
      if (i < 0 || i > 3 || e.d2 === 64) return;
      if (run.active !== i + 1) { this.morphStart(i); if (run.active !== i + 1) return; }
      this.morphSet(run.mix + (e.d2 - 64) * MORPH_STEP);
    }

    // ---- LFOs (tui/lfo.go) ----
    lfoSetting(i) { const s = this.mem.lfo[i]; return s.set ? s : Object.assign({}, s, { rate: 4, depth: 50 }); }
    lfoSet(i, name, n) {
      if (i < 0 || i >= LFO_COUNT) return;
      const s = Object.assign({}, this.lfoSetting(i), { set: true });
      switch (name) {
        case 'bus': s.bus = clamp(n, 0, 3); break;
        case 'knob': s.knob = clamp(n, 0, LFO_SWITCH); break;
        case 'shape': s.shape = clamp(n, 0, 6); break;
        case 'rate': s.rate = clamp(n, 0, LFO_RATES.length - 1); break;
        case 'depth': s.depth = clamp(n, 0, 100); break;
        case 'shift': s.shift = clamp(n, 0, 3); break;
        default: return;
      }
      const run = this.sc.lfo[i];
      if (run.on && (name === 'bus' || name === 'knob')) { this.lfoRestore(i); this.mem.lfo[i] = s; this.lfoBegin(i); }
      else this.mem.lfo[i] = s;
      this.save();
    }
    lfoToggle(i) {
      if (i < 0 || i >= LFO_COUNT) return;
      const run = this.sc.lfo[i];
      if (run.on) { this.lfoRestore(i); run.on = false; this.msg = 'LFO ' + (i + 1) + ' off'; return; }
      this.lfoBegin(i);
    }
    lfoBegin(i) {
      const set = this.lfoSetting(i);
      if (this.punchBusy(set.bus)) { this.msg = 'BUS ' + (set.bus + 1) + ' has a motion: stop it first'; return; }
      const run = this.sc.lfo[i] = Object.assign(newLfoRun(), { on: true, center: 64, wasOn: -1, last: -1 });
      if (set.knob === LFO_SWITCH) { const f = this.fx[set.bus]; if (f.onSeen) run.wasOn = f.on ? 1 : 0; }
      else { const v = this.cc(set.bus + 1, PUNCH_CCS[set.knob]); if (v !== null) run.center = v; else { this.msg = 'LFO ' + (i + 1) + ': BUS ' + (set.bus + 1) + ' CTRL ' + (set.knob + 1) + ' is not known yet, so it swings around the middle (turn it once on the unit to start from there)'; return; } }
      this.msg = 'LFO ' + (i + 1) + ' on' + (this.clock.running ? '' : " — it moves with the unit's clock: start its sequencer");
    }
    lfoSend(i, v) {
      const set = this.lfoSetting(i), run = this.sc.lfo[i];
      run.value = v;
      if (v === run.last) return;
      run.last = v;
      const p = emptyBus();
      if (set.knob === LFO_SWITCH) p.on = Math.floor(v / 127); else p.knobs[set.knob] = v;
      this.punchSend(set.bus, p, null);
    }
    lfoRestore(i) {
      const set = this.lfoSetting(i), run = this.sc.lfo[i];
      if (!run.on) return;
      this.lfoSend(i, set.knob === LFO_SWITCH ? Math.max(run.wasOn, 0) * 127 : run.center);
    }
    lfoPaused(bus) {
      const run = this.sc.punch;
      if (run.held !== 0 && (run.edit || this.mem.punch[run.held - 1][bus].set)) return true;
      return run.active !== 0 && this.mem.punch[run.active - 1][bus].set;
    }
    lfoBase(cap) {
      this.sc.lfo.forEach((run, i) => {
        if (!run.on) return;
        const set = this.lfoSetting(i);
        if (set.knob === LFO_SWITCH) { if (run.wasOn >= 0) cap[set.bus].on = run.wasOn; }
        else cap[set.bus].knobs[set.knob] = run.center;
      });
      return cap;
    }
    lfoTick() {
      this.sc.lfo.forEach((run, i) => {
        if (!run.on) return;
        const set = this.lfoSetting(i);
        if (this.punchBusy(set.bus)) return;
        if (this.lfoPaused(set.bus)) { run.last = -1; return; }
        this.lfoSend(i, lfoValue(set, run.center, this.clock.ticks, run));
      });
    }
    lfoStop() { this.sc.lfo.forEach((run) => { if (run.on) { this.lfoRestore(this.sc.lfo.indexOf(run)); run.last = -1; } }); }

    // ---- what the page shows ----
    snapshot() {
      const now = performance.now(), clk = this.clock, sc = this.sc;
      const fresh = clk.at > 0 && now - clk.at < 2000;
      const bank = this.bank;
      const s = {
        unit: this.midi.ok, local: true, midiName: this.midi.name, midiError: this.midi.error, demo: this.demo,
        bank: String.fromCharCode(65 + bank), bankFill: Array(10).fill(16),
        bpm: clk.bpm ? Math.round(clk.bpm * 10) / 10 : 0, running: fresh && clk.running,
        clock: '', bar: fresh ? Math.floor(clk.ticks / TICKS_BAR) + 1 : 0, beat: fresh ? Math.floor(clk.ticks / 24) % 4 : -1,
        rec: false, now: '—', next: '—', msg: this.msg, version: 'web',
        pads: [], focus: { on: false, multi: false, dry: 0 },
        motion: { bars: sc.bars, overdub: sc.overdub, bus: [] }, fx: [], punch: this.webPunch(), lfo: [], patterns: [],
        looper: { bank: 'A', bars: 4, later: '', pattern: 0, patternName: '', known: false, pads: [] },
        slicer: { bank: 'A', pad: -1, pads: [], slices: 8, random: false, order: 'shuffle', div: 0, swing: 0, target: 'A', free: '', src: '', busy: false, phase: 'pick', dest: -1, destName: '', played: '', later: '', prev: false, loop: '', mangle: [0, 0, 0, 0, 0, 0, 5], active: 0 },
        ptn: this.webPtn(), level: null, levelNote: 'no level meter in the browser',
      };
      for (let i = 0; i < 16; i++) {
        const at = this.lastHit.get(bank * 16 + i), age = at ? now - at : 1e9;
        const kit = this.demo ? duPad(bank, i) : null, playing = this.demo && this.ptn.nowBank >= 0 ? duPtn(this.ptn.nowBank, this.ptn.now) : null;
        s.pads.push({ n: i + 1, name: kit ? kit.name : '', has: !this.demo || !!kit, bus: kit ? kit.bus : 0, level: 127, muted: false, heard: age < 250, inPat: !!(playing && playing.pads.has(bank * 16 + i)),
          glow: Math.round(clamp(1 - age / 1500, 0, 1) * 5) / 5, sel: false, keep: false, bars: '', dry: false });
      }
      sc.bus.forEach((bs, b) => {
        const slots = bs.mo.map((mo) => {
          const sl = { state: mo.state, filled: motionFilled(mo), bars: Math.floor(mo.total / TICKS_BAR), pos: 0, speed: mo.speed > 0 ? mo.speed : 1, rev: mo.rev, knob: 0, curve: [] };
          if (mo.state === 4) sl.pos = Math.floor(mo.fpos / Math.max(mo.total, 1) * 32) / 32;
          else if (mo.state === 2) sl.pos = Math.floor(mo.pos / Math.max(mo.total, 1) * 32) / 32;
          if (sl.filled) { const [knob, curve] = motionCurve(mo, 16); sl.knob = knob + 1; sl.curve = curve; }
          return sl;
        });
        s.motion.bus.push({ sel: bs.sel, slots });
      });
      for (let b = 0; b < 2; b++) {
        const f = this.fx[b], fx = { seen: f.onSeen, on: f.on, name: f.numSeen ? FX.fx12[f.num] || '?' : '', note: '', knobs: [-1, -1, -1] };
        for (let k = 0; k < 3; k++) { const v = this.cc(b + 1, 16 + k); if (v !== null) fx.knobs[k] = v; }
        s.fx.push(fx);
      }
      for (let i = 0; i < LFO_COUNT; i++) {
        const set = this.lfoSetting(i), run = sc.lfo[i];
        s.lfo.push({ bus: set.bus, knob: set.knob, shape: set.shape, rate: set.rate, depth: set.depth, shift: set.shift, on: run.on, value: run.value });
      }
      for (let i = 0; i < 16; i++) { const d = this.demo ? duPtn(bank, i) : null; s.patterns.push({ n: i + 1, exists: !this.demo || !!d, pads: d ? d.pads.size : 0, now: this.demo && this.ptn.nowBank === bank && this.ptn.now === i, next: false }); }
      return s;
    }
    webPunch() {
      const run = this.sc.punch;
      const w = { arm: run.arm, edit: run.edit, sel: run.sel, bus: [], snap: [], live: [], morph: run.morph, active: run.active, mix: run.mix, slots: [], presets: [] };
      for (let b = 0; b < 4; b++) {
        const f = this.fx[b], st = { num: -1, name: '', on: -1 };
        if (f.numSeen) { st.num = f.num; st.name = fxNameOf(b, f.num); }
        if (f.onSeen) st.on = f.on ? 1 : 0;
        w.bus.push(st);
      }
      const live = run.sel > 0 ? this.capturePunch() : null;
      for (let b = 0; b < 4; b++) {
        w.snap.push(run.sel > 0 ? webVals(b, this.mem.punch[run.sel - 1][b]) : { set: false, name: '', num: 0, on: 0, params: [], text: [], raw: [] });
        w.live.push(live ? webVals(b, live[b]) : { set: false, name: '', num: 0, on: 0, params: [], text: [], raw: [] });
      }
      FX.presets.forEach((p) => w.presets.push({ name: p.name, note: p.note, buses: p.buses.map((b) => !!b.fx) }));
      this.mem.punch.forEach((sn, i) => w.slots.push({ set: snapSet(sn), held: run.held === i + 1, buses: sn.map((p) => p.set) }));
      return w;
    }
    webPtn() {
      const bank = this.ptnBank(), p = this.ptn;
      return { bank: String.fromCharCode(65 + bank), known: true, now: p.now >= 0 && p.nowBank === bank ? p.now : -1, next: p.next >= 0 && p.nextBank === bank ? p.next : -1,
        slots: Array.from({ length: 16 }, (_, i) => { const d = this.demo ? duPtn(bank, i) : null; return { n: i + 1, exists: !this.demo || !!d, pads: d ? d.pads.size : 0 }; }) };
    }
  }

  // ---- the pieces ----
  function newMotion() { return { state: 0, total: 0, pos: 0, events: [], speed: 0, rev: false, fpos: 0, touch: [0, 0, 0], lastAt: [0, 0, 0] }; }
  function newLfoRun() { return { on: false, center: 64, wasOn: -1, last: -1, period: 0, rv: 0, value: 0 }; }
  function motionFilled(mo) { return mo.events.length > 0 && mo.total > 0; }
  function padOfNote(note) { for (let p = 0; p < 16; p++) if (36 + PAD_OFFSET[p] === note) return p + 1; return 0; }
  function overdub(mo, knob, val) {
    const at = mo.fpos;
    let from = at;
    const held = mo.touch[knob] > 0 && mo.lastAt[knob] <= at;
    if (held) from = mo.lastAt[knob];
    mo.events = mo.events.filter((ev) => !(ev.knob === knob && ((held && ev.tick > from && ev.tick <= at) || (!held && ev.tick === Math.floor(at)))));
    mo.events.push({ tick: Math.min(Math.floor(at), mo.total - 1), knob, val });
    mo.events.sort((a, b) => a.tick - b.tick);
    mo.touch[knob] = OVERDUB_HOLD; mo.lastAt[knob] = at;
  }
  // The knob that moves the most as n values over the loop; a step with no move holds the value before it.
  function motionCurve(mo, n) {
    const curves = [0, 1, 2].map(() => Array(n).fill(-1));
    mo.events.forEach((ev) => { curves[ev.knob][Math.min(Math.floor(ev.tick * n / Math.max(mo.total, 1)), n - 1)] = ev.val; });
    let best = 0, bestRange = 0;
    curves.forEach((c, k) => {
      let lo = 128, hi = -1;
      c.forEach((v) => { if (v >= 0) { lo = Math.min(lo, v); hi = Math.max(hi, v); } });
      if (hi >= 0 && hi - lo >= bestRange) { best = k; bestRange = hi - lo; }
    });
    let last = 0;
    return [best, curves[best].map((v) => { if (v < 0) v = last; last = v; return v; })];
  }
  function morphBus(a, b, mix) {
    const out = { set: a.set, on: a.on, num: a.num, knobs: a.knobs.slice() };
    if (a.on < 0 && b.on === 1) out.on = 0;
    const half = mix >= 64, same = a.num < 0 || b.num < 0 || a.num === b.num;
    if (b.num >= 0 && half) out.num = b.num;
    if (b.on >= 0 && half) out.on = b.on;
    out.knobs.forEach((_, k) => {
      const x = a.knobs[k], y = b.knobs[k];
      if (y < 0) return;
      if (x < 0 || !same) { if (half) out.knobs[k] = y; return; }
      out.knobs[k] = x + Math.trunc((y - x) * mix / 127 + 0.5 * sign(y - x));
    });
    return out;
  }
  function punchMerge(snap, cur, was) {
    let out = { set: snap.set, on: snap.on, num: snap.num, knobs: snap.knobs.slice() }, changed = false;
    if (!snap.set) out = emptyBus();
    const take = (snapV, curV, wasV, set) => {
      if (curV < 0 || (curV === snapV && snap.set)) return;
      if (!snap.set && curV === wasV) return;
      set(curV); changed = true;
    };
    take(snap.num, cur.num, was.num, (v) => { out.num = v; });
    take(snap.on, cur.on, was.on, (v) => { out.on = v; });
    out.knobs.forEach((_, k) => take(snap.knobs[k], cur.knobs[k], was.knobs[k], (v) => { out.knobs[k] = v; }));
    if (changed) out.set = true;
    return [out, changed];
  }
  function punchText(s) {
    const parts = [];
    s.forEach((p, b) => { if (p.set) parts.push('BUS ' + (b + 1) + (p.num >= 0 ? ' ' + fxNameOf(b, p.num) : '')); });
    return parts.length ? parts.join(', ') : 'nothing known yet: turn a knob on the unit once';
  }
  function webVals(bus, p) {
    if (!p.set) return { set: false, name: '', num: 0, on: 0, params: [], text: [], raw: [] };
    const v = { set: true, num: p.num, on: p.on, raw: p.knobs, name: p.num >= 0 ? fxNameOf(bus, p.num) : '?', params: ['', '', '', '', '', ''], text: ['', '', '', '', '', ''] };
    const knobs = knobsOf(bus, Math.max(p.num, 0));
    knobs.forEach((kn, k) => {
      if (p.num < 0) kn = { n: 'CTRL ' + (k + 1), k: 'c', lo: 0, hi: 127 };
      if (kn.n) { v.params[k] = kn.n; v.text[k] = valueText(kn, p.knobs[k]); }
    });
    return v;
  }
  function lfoWave(shape, p, rv) {
    switch (shape) {
      case 0: return Math.sin(2 * Math.PI * p);
      case 1: return p < 0.25 ? 4 * p : p < 0.75 ? 2 - 4 * p : 4 * p - 4;
      case 2: return 2 * p - 1;
      case 3: return 1 - 2 * p;
      case 4: return p < 0.5 ? 1 : -1;
      case 5: return rv;
      case 6: return -Math.pow(1 - p, 2);
    }
    return 0;
  }
  function lfoValue(set, center, ticks, run) {
    const period = Math.floor(LFO_RATES[clamp(set.rate, 0, LFO_RATES.length - 1)] * 24);
    let p = (ticks % period) / period + set.shift * 0.25;
    p -= Math.floor(p);
    const cycle = Math.floor(ticks / period) + set.shift;
    if (cycle !== run.period || run.rv === 0) { run.period = cycle; run.rv = Math.random() * 2 - 1; }
    const w = lfoWave(set.shape, p, run.rv);
    if (set.knob === LFO_SWITCH) { let on = w > 0; if (set.shape === 6) on = p < 0.3; return on ? 127 : 0; }
    let amp = set.depth / 100 * 127 / 2;
    if (set.shape === 6) amp = set.depth / 100 * 127;
    return clamp(Math.round(center + w * amp), 0, 127);
  }

  window.SP404Local = {
    // the pads and patterns of the unit of the demo (from the tools), for the Pads and Patterns pages
    unit: {
      setPads(list) { DU.pads = Array.from({ length: 160 }, (_, i) => { const p = list[i]; return p ? { name: p.name, has: p.has, secs: p.secs } : { name: '', has: false }; }); DU.fromUnit = true; },
      setPattern: duSetPattern,
      clearBuffers() { DU.buf.clear(); },
    },
    supported: () => !!navigator.requestMIDIAccess,
    start: async (opts) => { const e = new Engine(opts || {}); await e.start(); return e; },
  };
})();
