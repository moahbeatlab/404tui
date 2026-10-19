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
  class Demo {
    constructor(onEvent, onChange) { this.onEvent = onEvent; this.onChange = onChange; this.ok = true; this.name = 'demo'; this.error = ''; this.ctx = null; }
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
      let t = performance.now(), n = 0;
      this.onEvent({ kind: 'start' });
      this.timer = setInterval(() => {
        const per = 60000 / 125 / 24, now = performance.now();
        while (t + per <= now) { t += per; this.onEvent({ kind: 'clock' }); n++; }
      }, 10);
      return true;
    }
    send(bytes) {
      if ((bytes[0] & 0xF0) === 0x90 && bytes[2] > 0) this.blip(bytes[1]);
      return true;
    }
    blip(note) {
      try {
        this.ctx = this.ctx || new (window.AudioContext || window.webkitAudioContext)();
        const o = this.ctx.createOscillator(), g = this.ctx.createGain();
        o.frequency.value = 110 * Math.pow(2, (note - 36) / 12); o.type = 'triangle';
        g.gain.setValueAtTime(0.15, this.ctx.currentTime); g.gain.exponentialRampToValueAtTime(0.001, this.ctx.currentTime + 0.25);
        o.connect(g); g.connect(this.ctx.destination); o.start(); o.stop(this.ctx.currentTime + 0.3);
      } catch (e) { /* no audio: the demo is silent */ }
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
      this.ptn = { now: -1, next: -1, bank: -1, dueTick: 0 };
      this.dirty = false;
      this.midi = this.demo ? new Demo((e) => this.input(e), () => this.changed()) : new Midi((e) => this.input(e), () => this.changed());
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
        s.pads.push({ n: i + 1, name: '', has: true, bus: 0, level: 127, muted: false, heard: age < 250, inPat: false,
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
      for (let i = 0; i < 16; i++) s.patterns.push({ n: i + 1, exists: true, pads: 0, now: false, next: false });
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
        slots: Array.from({ length: 16 }, (_, i) => ({ n: i + 1, exists: true, pads: 0 })) };
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
    supported: () => !!navigator.requestMIDIAccess,
    start: async (opts) => { const e = new Engine(opts || {}); await e.start(); return e; },
  };
})();
