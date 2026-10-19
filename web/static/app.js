'use strict';
// 404tui Perform on a phone: the state comes in over a WebSocket, the commands go out the same way.
(() => {
  const $ = (id) => document.getElementById(id);
  const qs = new URLSearchParams(location.search);
  const store = {
    get(k) { try { return localStorage.getItem(k) || ''; } catch (e) { return ''; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch (e) {} },
  };

  // ---- connection ----
  let ws = null, retry = 0, retryTimer = 0, host = '', key = '', everOpen = false, state = null, rtt = null;
  const servedByUs = location.protocol.startsWith('http') && !qs.get('host');

  function target() {
    host = qs.get('host') || (servedByUs ? location.host : store.get('host'));
    key = qs.get('key') || store.get('key');
    return host;
  }
  function wsURL() {
    const proto = location.protocol === 'https:' ? 'wss://' : 'ws://';
    return proto + host + '/ws' + (key ? '?key=' + encodeURIComponent(key) : '');
  }
  function setConn(kind, text) {
    $('conn').className = 'conn ' + kind;
    $('connText').textContent = text;
    $('conn2').className = 'conn ' + kind;
    $('connText2').textContent = text;
  }
  function open() {
    clearTimeout(retryTimer);
    if (!target()) return showConnect('');
    try { ws = new WebSocket(wsURL()); } catch (e) { return showConnect('That address does not work.'); }
    ws.onopen = () => { retry = 0; everOpen = true; $('lost').hidden = true; $('connect').hidden = true; $('app').hidden = false;
      store.set('host', host); if (key) store.set('key', key); setConn('ok', 'connected'); ping(); send({ a: 'view', name: $('app').dataset.view }); };
    ws.onmessage = (ev) => { let o; try { o = JSON.parse(ev.data); } catch (e) { return; }
      if (o.pong !== undefined) { pong(); return; } state = o; schedule(); };
    ws.onclose = () => { ws = null; setConn('bad', 'offline'); if (!everOpen && retry >= 2) return showConnect('No answer from ' + host + '. Is sp404 serve running, and the key right?');
      $('lost').hidden = !everOpen; $('lostS').textContent = 'reconnecting to ' + host + '…';
      retry++; retryTimer = setTimeout(open, Math.min(500 * retry, 3000)); };
    ws.onerror = () => {};
  }
  function showConnect(err) {
    $('app').hidden = true; $('lost').hidden = true; $('connect').hidden = false;
    $('hostIn').value = host || store.get('host'); $('keyIn').value = key || store.get('key'); $('connectErr').textContent = err || '';
  }
  $('connectForm').addEventListener('submit', (e) => {
    e.preventDefault();
    qs.delete('host'); qs.delete('key');
    store.set('host', $('hostIn').value.trim().replace(/^https?:\/\//, '').replace(/\/.*$/, ''));
    store.set('key', $('keyIn').value.trim());
    retry = 0; everOpen = false; if (ws) ws.close(); open();
  });
  $('lostB').addEventListener('click', () => { clearTimeout(retryTimer); if (ws) ws.onclose = null, ws.close(); everOpen = false; showConnect(''); });

  // Round trip: shown, so the delay is not a guess.
  let pingAt = 0;
  function ping() {
    if (!ws || ws.readyState !== 1) return;
    pingAt = performance.now();
    ws.send(JSON.stringify({ a: 'ping', t: pingAt }));
    setTimeout(ping, 3000);
  }
  function pong() {
    rtt = Math.round(performance.now() - pingAt);
    if (state && state.unit === false) setConn('nounit', 'no unit · ' + rtt + ' ms'); else setConn('ok', rtt + ' ms');
  }
  function send(o) {
    if (ws && ws.readyState === 1) ws.send(JSON.stringify(o));
    if (navigator.vibrate) navigator.vibrate(8);
  }

  // ---- building the screen once ----
  const grid = $('grid');
  const padEls = [];
  for (let i = 0; i < 16; i++) {
    const d = document.createElement('div');
    d.className = 'pad empty';
    d.innerHTML = '<div class="n"></div><div class="nm"></div><div class="lv"><i></i></div><span class="bars"></span><span class="bus"></span>';
    grid.appendChild(d);
    padEls.push({ d, n: d.children[0], nm: d.children[1], lvi: d.children[2].firstChild, bars: d.children[3], bus: d.children[4], last: '' });
    pad(d, i);
  }
  // A pad: a tap mutes it (its level goes to 0 and back), holding it isolates it (the only one on its bus; again brings
  // the others back), a swipe up or down sets the volume from where it was. With focus on, a tap picks the pad.
  // A tap counts when the finger comes up, because it is not known before whether it will swipe or hold.
  const HOLD = 400;
  // Only the FOCUS button's mode (multi) picks pads by tap; pads kept by a hold leave taps to mute.
  const focusOn = () => !!(state && state.focus && state.focus.multi);
  function tap(i) { send({ a: focusOn() ? 'pad' : 'mute', i }); }
  // While a pad is swiped its level is shown in a bubble above it: the finger covers the pad itself.
  function showLv(el, i, v) {
    const b = $('lvPop');
    const r = el.getBoundingClientRect();
    b.hidden = false;
    $('lvPopV').textContent = v;
    $('lvPopN').textContent = 'PAD ' + (i + 1) + ' LEVEL';
    const w = b.offsetWidth, h = b.offsetHeight;
    b.style.left = Math.max(w / 2 + 4, Math.min(innerWidth - w / 2 - 4, r.left + r.width / 2)) + 'px';
    b.style.top = Math.max(4, r.top - h - 8) + 'px';
  }
  function hideLv() { $('lvPop').hidden = true; }

  function pad(el, i) {
    let down = null;
    el.addEventListener('pointerdown', (e) => {
      e.preventDefault();
      try { el.setPointerCapture(e.pointerId); } catch (err) {}
      const p = state && state.pads && state.pads[i];
      down = { y: e.clientY, moved: false, held: false, v0: p ? p.level : 0, h: el.getBoundingClientRect().height || 80 };
      const d = down;
      d.timer = setTimeout(() => { if (down === d && !d.moved && !focusOn()) { d.held = true; send({ a: 'solo', i }); if (navigator.vibrate) navigator.vibrate(15); } }, HOLD);
    });
    el.addEventListener('pointermove', (e) => {
      if (!down) return;
      if (!down.moved && !down.held && Math.abs(e.clientY - down.y) > 10) { down.moved = true; clearTimeout(down.timer); }
      if (down.held) return;
      if (!down.moved) return;
      const v = Math.round(Math.max(0, Math.min(127, down.v0 + (down.y - e.clientY) / (down.h * 0.8) * 127)));
      const now = performance.now();
      showLv(el, i, v);
      if (now - (down.sent || 0) > 33 && v !== down.v) { down.sent = now; down.v = v; send({ a: 'level', i, v }); padEls[i].lvi.style.width = (v / 127 * 100) + '%'; el.dataset.lv = v; }
    });
    el.addEventListener('pointerup', () => {
      if (!down) return;
      clearTimeout(down.timer);
      hideLv();
      if (!down.held && !down.moved) tap(i);
      else if (down.held) {}
      else if (down.v !== undefined) send({ a: 'level', i, v: down.v });
      down = null;
    });
    el.addEventListener('pointercancel', () => { if (down) clearTimeout(down.timer); hideLv(); down = null; });
    el.addEventListener('contextmenu', (e) => e.preventDefault());
  }

  // No zoom: iOS ignores user-scalable=no, so double taps and pinches are stopped here.
  let lastEnd = 0;
  document.addEventListener('touchend', (e) => { const n = Date.now(); if (n - lastEnd < 350) e.preventDefault(); lastEnd = n; }, { passive: false });
  document.addEventListener('dblclick', (e) => e.preventDefault());
  ['gesturestart', 'gesturechange', 'gestureend'].forEach((g) => document.addEventListener(g, (e) => e.preventDefault()));

  // Buttons with a data-a send it as they are pressed down, not when the finger comes up.
  document.querySelectorAll('[data-a]').forEach((b) => b.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: b.dataset.a }); }));

  const barsEl = $('bars');
  [1, 2, 3, 4, 8].forEach((n) => { const b = document.createElement('button'); b.textContent = n; b.dataset.n = n;
    b.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: 'bars', n }); }); barsEl.appendChild(b); });

  const busesEl = $('buses');
  const busEls = [];
  const CTL = [['mspeed', 'speed'], ['mrev', 'reverse'], ['mclear', 'clear']];
  for (let bus = 0; bus < 2; bus++) {
    const w = document.createElement('div');
    w.className = 'bm';
    w.innerHTML = '<div class="bushead"><b style="color:var(--bus' + (bus + 1) + ')">BUS ' + (bus + 1) + '</b><span class="fx"></span><span class="knobs">' +
      '<span class="knob"><i></i></span><span class="knob"><i></i></span><span class="knob"><i></i></span></span></div><div class="slots"></div>';
    const slots = w.querySelector('.slots');
    const slotEls = [], ctlEls = [];
    for (let s = 0; s < 4; s++) {
      const b = document.createElement('button');
      b.className = 'big slot';
      b.innerHTML = '<b>' + (s + 1) + '</b><svg viewBox="0 0 64 20" preserveAspectRatio="none"><path d=""/></svg><span class="st">empty</span><span class="pos"></span>';
      b.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: 'motion', bus, slot: s }); });
      slots.appendChild(b);
      slotEls.push({ b, path: b.querySelector('path'), st: b.querySelector('.st'), pos: b.querySelector('.pos'), key: '' });
    }
    for (const [act, label] of CTL) {
      const b = document.createElement('button');
      b.className = 'big ctl';
      b.innerHTML = '<b>' + label + '</b><small></small>';
      b.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: act, bus }); });
      slots.appendChild(b);
      ctlEls.push({ b, small: b.querySelector('small') });
    }
    busesEl.appendChild(w);
    busEls.push({ w, fx: w.querySelector('.fx'), knobs: [...w.querySelectorAll('.knob i')], slotEls, ctlEls });
  }

  // PUNCH: four snapshots of all four buses. Held, a snapshot is on the buses; let go, they go back.
  const punchEl = $('punch');
  const punchEls = [];
  const punchHeld = new Set();
  function punchUp(i) { if (punchHeld.delete(i)) send({ a: 'punch', i, n: 0 }); }
  for (let i = 0; i < 4; i++) {
    const b = document.createElement('button');
    b.className = 'big pn empty';
    b.innerHTML = '<b>' + (i + 1) + '</b><span class="dots">' + [1, 2, 3, 4].map((n) => '<i style="--c:var(--bus' + n + ')"></i>').join('') + '</span><small>empty</small>';
    b.addEventListener('pointerdown', (e) => {
      e.preventDefault();
      try { b.setPointerCapture(e.pointerId); } catch (err) {}
      punchHeld.add(i); send({ a: 'punch', i, n: 1 });
      if (navigator.vibrate) navigator.vibrate(10);
    });
    ['pointerup', 'pointercancel', 'lostpointercapture'].forEach((ev) => b.addEventListener(ev, () => punchUp(i)));
    b.addEventListener('contextmenu', (e) => e.preventDefault());
    punchEl.appendChild(b);
    punchEls.push({ b, dots: [...b.querySelectorAll('i')], small: b.querySelector('small') });
  }
  const pnSide = document.createElement('div');
  pnSide.id = 'pnSide';
  const pnSave = document.createElement('button');
  pnSave.id = 'pnSave'; pnSave.className = 'big'; pnSave.innerHTML = '<span>SAVE</span><small>hold to punch</small>';
  pnSave.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: 'punchsave' }); });
  const pnPre = document.createElement('button');
  pnPre.className = 'big'; pnPre.innerHTML = '<span>PRESETS</span>';
  pnPre.addEventListener('pointerdown', (e) => { e.preventDefault(); $('presetPick').hidden = false; drawPresets(); });
  pnSide.appendChild(pnSave); pnSide.appendChild(pnPre);
  punchEl.appendChild(pnSide);
  // The presets: pick the snapshot they go into (the first empty one to begin with, then the next), tap a preset.
  let presetInto = -1;
  const intoEl = $('presetInto');
  for (let i = 0; i < 4; i++) {
    const b = document.createElement('button');
    b.textContent = i + 1;
    b.addEventListener('pointerdown', (e) => { e.preventDefault(); presetInto = i; drawPresets(); });
    intoEl.appendChild(b);
  }
  function drawPresets() {
    const p = state && state.punch;
    if (!p) return;
    if (presetInto < 0) { const e = p.slots.findIndex((s) => !s.set); presetInto = e < 0 ? 0 : e; }
    [...intoEl.children].forEach((b, i) => { cls(b, 'on', i === presetInto); cls(b, 'set', p.slots[i].set); });
    const list = $('presetList');
    if (list.children.length !== p.presets.length) {
      list.innerHTML = '';
      p.presets.forEach((pr, i) => {
        const b = document.createElement('button');
        b.innerHTML = '<b></b><span class="dots">' + [1, 2, 3, 4].map((n) => '<i style="--c:var(--bus' + n + ')"></i>').join('') + '</span><small></small>';
        b.querySelector('b').textContent = pr.name; b.querySelector('small').textContent = pr.note;
        pr.buses.forEach((on, n) => cls(b.querySelectorAll('i')[n], 'on', on));
        b.addEventListener('pointerdown', (e) => {
          e.preventDefault();
          send({ a: 'punchpreset', i, n: presetInto });
          presetInto = (presetInto + 1) % 4;
          if (navigator.vibrate) navigator.vibrate(10);
          drawPresets();
        });
        list.appendChild(b);
      });
    }
  }
  $('presetX').addEventListener('pointerdown', (e) => { e.preventDefault(); $('presetPick').hidden = true; });
  $('presetPick').addEventListener('pointerdown', (e) => { if (e.target === $('presetPick')) $('presetPick').hidden = true; });
  // A finger that is lost (the page hidden, the screen locked) must not leave the buses punched.
  window.addEventListener('blur', () => [...punchHeld].forEach(punchUp));
  document.addEventListener('visibilitychange', () => { if (document.hidden) [...punchHeld].forEach(punchUp); });
  function renderPunch(p) {
    if (!p) return;
    p.slots.forEach((s, i) => {
      const e = punchEls[i];
      cls(e.b, 'empty', !s.set); cls(e.b, 'held', s.held); cls(e.b, 'arm', p.arm);
      s.buses.forEach((on, n) => cls(e.dots[n], 'on', on));
      set(e.small, 'textContent', p.arm ? 'SAVE HERE' : s.held ? 'HELD' : s.set ? 'hold' : 'empty');
    });
    cls(pnSave, 'arm', p.arm);
    set(pnSave.querySelector('small'), 'textContent', p.arm ? 'tap a snapshot' : 'hold to punch');
  }

  // tabs (portrait), and the switch between Perform and the Looper (the tab in portrait, the button in the top bar)
  const app = $('app');
  app.dataset.tab = store.get('tab') === 'looper' ? 'pads' : (store.get('tab') || 'pads');
  app.dataset.view = ['looper', 'patterns'].includes(store.get('view')) ? store.get('view') : 'perform';
  const tabBtns = document.querySelectorAll('#tabs button');
  function syncTabs() {
    tabBtns.forEach((x) => x.classList.toggle('on', app.dataset.view !== 'perform' ? x.dataset.tab === app.dataset.view : x.dataset.tab === app.dataset.tab));
    // The button in the top bar (landscape) names the view it goes to: Perform, Looper, Patterns, and round.
    $('viewBtn').textContent = { perform: 'LOOPER', looper: 'PATTERNS', patterns: 'PERFORM' }[app.dataset.view];
    $('viewBtn').classList.toggle('on', app.dataset.view !== 'perform');
  }
  function setView(v) { app.dataset.view = v; store.set('view', v); syncTabs(); send({ a: 'view', name: v }); try { if (state) render(); } catch (e) { /* before the first state */ } }
  // A double tap on the Patterns button (the tab, or the button at the top) shows the bank of the pattern that plays.
  let ptnTapAt = 0;
  function ptnDouble() {
    const now = performance.now(), dbl = now - ptnTapAt < 450 && app.dataset.view === 'patterns';
    ptnTapAt = now;
    if (dbl) send({ a: 'ptnfollow' });
    return dbl;
  }
  tabBtns.forEach((b) => b.addEventListener('pointerdown', (e) => {
    e.preventDefault();
    if (b.dataset.tab === 'patterns' && ptnDouble()) return;
    if (b.dataset.tab === 'looper' || b.dataset.tab === 'patterns') return setView(b.dataset.tab);
    app.dataset.tab = b.dataset.tab; store.set('tab', b.dataset.tab); setView('perform');
  }));
  $('viewBtn').addEventListener('pointerdown', (e) => {
    e.preventDefault();
    // Tapped twice quickly where it said PATTERNS: stay on the Patterns page, and show the bank of the pattern that plays.
    if (app.dataset.view === 'patterns' && performance.now() - ptnTapAt < 450) { ptnTapAt = performance.now(); send({ a: 'ptnfollow' }); return; }
    const to = { perform: 'looper', looper: 'patterns', patterns: 'perform' }[app.dataset.view];
    if (to === 'patterns') ptnTapAt = performance.now();
    setView(to);
  });
  syncTabs();

  // ---- the Looper ----
  const lpGrid = $('lpgrid');
  const lpPads = [];
  let lpSel = -1, lpAsk = 0, lpAskT = 0;
  for (let i = 0; i < 16; i++) {
    const d = document.createElement('div');
    d.className = 'lpad';
    d.innerHTML = '<b></b><span></span>';
    // In the Slicer a tap chooses the pad and holding it mutes it (again brings it back).
    let hold = null;
    const stopHold = () => { if (hold) clearTimeout(hold.t); };
    d.addEventListener('pointerdown', (e) => {
      e.preventDefault();
      if ($('looper').dataset.lp !== 'slice') { lpSel = lpSel === i ? -1 : i; lpAsk = 0; render(); return; }
      try { d.setPointerCapture(e.pointerId); } catch (x) { /* not everywhere */ }
      stopHold();
      const h = hold = { x: e.clientX, y: e.clientY, done: false, t: 0 };
      const p = state && state.slicer && state.slicer.pads[i];
      if (p && p.has) h.t = setTimeout(() => { h.done = true; if (navigator.vibrate) navigator.vibrate(20); send({ a: 'slmute', i }); }, 500);
    });
    d.addEventListener('pointermove', (e) => { if (hold && !hold.done && Math.hypot(e.clientX - hold.x, e.clientY - hold.y) > 10) { stopHold(); hold.done = true; } });
    d.addEventListener('pointerup', () => {
      const h = hold; stopHold(); hold = null;
      if (h && !h.done && $('looper').dataset.lp === 'slice') send({ a: 'slpad', i });
    });
    d.addEventListener('pointercancel', () => { stopHold(); hold = null; });
    lpGrid.appendChild(d);
    lpPads.push({ d, n: d.children[0], nm: d.children[1] });
  }
  const press = (id, fn) => $(id).addEventListener('pointerdown', (e) => { e.preventDefault(); fn(); if (navigator.vibrate) navigator.vibrate(8); });
  press('lpPat', () => send({ a: 'lppattern' }));
  press('slPrev', () => send({ a: 'slbank', n: -1 }));
  press('slNext', () => send({ a: 'slbank', n: 1 }));
  press('slTPrev', () => send({ a: 'sltbank', n: -1 }));
  press('slTNext', () => send({ a: 'sltbank', n: 1 }));
  press('slSlices', () => send({ a: 'slslices' }));
  press('slCut', () => send({ a: 'slcut' }));
  press('slOrder', () => send({ a: 'slorder' }));
  press('slDiv', () => send({ a: 'sldiv' }));
  press('slSwing', () => send({ a: 'slswing' }));
  let slAsk = 0, slAskT = 0;
  press('slGo', () => {
    const sl = state && state.slicer;
    if (sl && sl.phase === 'ready' && sl.destName && slAsk !== sl.dest + 1) {
      slAsk = sl.dest + 1; clearTimeout(slAskT); slAskT = setTimeout(() => { slAsk = 0; render(); }, 3000); return render();
    }
    slAsk = 0; send({ a: 'slgo' });
  });
  press('slAgain', () => send({ a: 'slagain' }));
  press('slCancel', () => send({ a: 'slcancel' }));
  press('lpDel', () => {
    const p = state && state.looper && state.looper.pads[lpSel];
    if (!p || !p.has) return;
    if (!p.del && lpAsk !== lpSel + 1) { lpAsk = lpSel + 1; clearTimeout(lpAskT); lpAskT = setTimeout(() => { lpAsk = 0; render(); }, 3000); return render(); }
    lpAsk = 0; send({ a: 'lpdel', i: lpSel }); lpSel = -1; render();
  });
  const lpSw = document.querySelectorAll('.lpsw button');
  function setLp(v) { $('looper').dataset.lp = v; lpSw.forEach((b) => b.classList.toggle('on', b.dataset.lp === v)); store.set('lp', v); try { render(); } catch (e) { /* before the first state */ } }
  lpSw.forEach((b) => b.addEventListener('pointerdown', (e) => { e.preventDefault(); setLp(b.dataset.lp); }));
  setLp(store.get('lp') === 'slice' ? 'slice' : 'loop');
  const PHASE = { idle: 'READY', starting: 'STARTING…', armed: 'ARMED', recording: 'REC', saving: 'SAVING…' };
  // The level meters (there is one on the stage panel and one on each page of the Looper): -60 dB to 0 dB.
  const meters = [...document.querySelectorAll('[data-meter]')].map((el) => ({ el, bars: [...el.querySelectorAll('.mbar')].map((b) => ({ i: b.querySelector('i'), u: b.querySelector('u') })), clip: el.querySelector('em'), txt: el.querySelector('b') }));
  const mpos = (db) => Math.max(0, Math.min(100, (db + 60) / 60 * 100));
  const SRC = { input: 'input', recording: 'recording', take: 'take' };
  function renderMeters(lv, note) {
    for (const m of meters) {
      m.el.classList.toggle('off', !lv);
      if (!lv) { m.bars.forEach((b) => { b.i.style.width = '0%'; b.u.style.left = '0%'; }); set(m.txt, 'textContent', note || 'no level'); cls(m.clip, 'on', false); continue; }
      m.bars.forEach((b, c) => {
        const w = mpos(lv.peak[c]).toFixed(0) + '%';
        if (b.i.style.width !== w) b.i.style.width = w;
        const left = mpos(lv.hold[c]).toFixed(0) + '%';
        if (b.u.style.left !== left) b.u.style.left = left;
        b.i.dataset.l = lv.peak[c] >= -3 ? 'hot' : lv.peak[c] >= -12 ? 'warn' : 'ok';
      });
      cls(m.clip, 'on', lv.clip);
      const top = Math.max(lv.hold[0], lv.hold[1]);
      set(m.txt, 'textContent', (lv.src === 'input' ? '' : 'REC ') + (top <= -60 ? '−∞ dB' : top + ' dB'));
    }
  }
  const SWING = { 0: 'OFF', 54: 'LIGHT', 58: 'MEDIUM', 62: 'HEAVY', 66: 'TRIPLET' };
  function renderSlicer(sl) {
    set($('slBank'), 'textContent', sl.bank);
    set($('slTBank'), 'textContent', sl.target);
    set($('slSlicesT'), 'textContent', sl.slices + ' SLICES');
    set($('slCutT'), 'textContent', sl.random ? 'RANDOM CUTS' : 'EQUAL CUTS');
    set($('slOrderT'), 'textContent', sl.order.toUpperCase());
    set($('slDivT'), 'textContent', sl.div ? 'GRID 1/' + sl.div : 'OWN LENGTH');
    set($('slSwingT'), 'textContent', 'SWING: ' + (SWING[sl.swing] || sl.swing + '%'));
    $('slSwing').disabled = !sl.div;
    set($('slMangleT'), 'textContent', sl.active ? 'MANGLE · ' + sl.active : 'MANGLE');
    cls($('slMangle'), 'on', sl.active > 0);
    drawDice();
    const ready = sl.phase === 'ready';
    $('slReady').hidden = !ready;
    set($('slT'), 'textContent', ready ? sl.src : sl.pad >= 0 ? sl.src : 'SLICER');
    set($('slSub'), 'textContent', sl.later ? sl.later : ready ? 'order ' + sl.played + ' · tap a pad of bank ' + sl.target + ', then PLACE' : sl.pad >= 0 ? (sl.free ? 'next free pad: ' + sl.free : 'bank ' + sl.target + ' is full') : 'tap a pad of bank ' + sl.bank + ' to slice');
    const asking = ready && sl.destName && slAsk === sl.dest + 1;
    cls($('slGo'), 'on', !!asking);
    set($('slGoT'), 'textContent', sl.busy ? 'WORKING…' : ready ? (sl.dest < 0 ? 'TAP A PAD' : asking ? 'TAP AGAIN: REPLACE ' + sl.free : sl.destName ? 'REPLACE ' + sl.free + ' …' : 'PLACE → ' + sl.free) : 'SLICE');
    $('slGo').disabled = sl.busy || (ready ? sl.dest < 0 : sl.pad < 0 || !(sl.pads[sl.pad] && sl.pads[sl.pad].has));
    for (const id of ['slPrev', 'slNext', 'slSlices', 'slCut', 'slOrder', 'slDiv']) $(id).disabled = sl.busy || ready;
    for (const id of ['slTPrev', 'slTNext', 'slAgain', 'slCancel']) $(id).disabled = sl.busy;
  }
  function renderLooper(s) {
    const l = s.looper;
    if (!l) return;
    set($('lpPatS'), 'textContent', !l.known ? 'reading…' : l.pattern ? 'pattern ' + l.patternName + ' plays all 16 pads ✓' : 'no pattern with all 16 pads');
    cls($('lpPat'), 'warn', l.known && !l.pattern);
    const sel = l.pads[lpSel];
    $('lpDel').disabled = !sel || !(sel.has || sel.del);
    const asking = lpAsk === lpSel + 1 && sel && sel.has;
    cls($('lpDel'), 'danger', asking);
    set($('lpDelT'), 'textContent', sel && sel.del ? 'KEEP IT' : asking ? 'TAP AGAIN TO DELETE' : 'DELETE');
    cls($('lpDel'), 'on', !!(sel && sel.del));
    set($('lpDelS'), 'textContent', sel ? (sel.has ? 'pad ' + sel.n + ' ' + sel.name : 'pad ' + sel.n + ' is empty') : 'tap a pad first');
    const sl = s.slicer, slMode = $('looper').dataset.lp === 'slice' && sl;
    (slMode ? sl.pads : l.pads).forEach((p, i) => {
      const e = lpPads[i];
      set(e.n, 'textContent', p.n);
      set(e.nm, 'textContent', p.has ? p.name : '');
      cls(e.d, 'del', !!p.del); cls(e.d, 'muted', !!p.muted);
      cls(e.d, 'has', p.has); cls(e.d, 'target', false); cls(e.d, 'sel', slMode ? i === (sl.phase === 'ready' ? sl.dest : sl.pad) : i === lpSel);
    });
    if (sl) renderSlicer(sl);
    set($('lpLast'), 'textContent', l.later || 'Tap a pad, then DELETE; or work with the Slicer.');
  }

  // ---- the Patterns page ----
  const ptnBtns = [];
  for (let i = 0; i < 16; i++) {
    const b = document.createElement('button');
    b.innerHTML = '<span>' + (i + 1) + '</span><small></small>';
    b.addEventListener('pointerdown', (e) => { e.preventDefault(); if (!b.disabled) send({ a: 'ptn', i }); });
    $('ptnGrid').appendChild(b);
    ptnBtns.push({ b, s: b.querySelector('small') });
  }
  function renderPatterns(p) {
    ptnBtns.forEach((e, i) => {
      const sl = p.slots[i];
      const ok = !!(sl && sl.exists);
      e.b.disabled = !ok;
      cls(e.b, 'now', i === p.now);
      cls(e.b, 'next', i === p.next);
      set(e.s, 'textContent', !p.known ? '' : i === p.next ? 'next…' : i === p.now ? 'playing' : ok ? sl.pads + (sl.pads === 1 ? ' pad' : ' pads') : 'empty');
    });
    set($('ptnInfo'), 'textContent', !p.known ? 'reading the patterns of bank ' + p.bank + '…' : p.next >= 0 ? 'Pattern ' + p.bank + (p.next + 1) + ' is sent: it takes over when the pattern ends.' : p.now >= 0 ? 'Playing ' + p.bank + (p.now + 1) + '. Tap another pattern: the unit plays it next.' : 'Tap a pattern of bank ' + p.bank + ': the unit plays it next.');
  }

  // ---- the Slicer's dice ----
  const DICE = [
    ['DROP-OUTS', 'slices that stay silent', (v) => v + ' %'],
    ['REVERSE', 'slices played backwards', (v) => v + ' %'],
    ['PITCH', 'up or down by up to', (v) => '±' + v + ' st'],
    ['ROLLS', 'slices that stutter', (v) => v + ' %'],
    ['HUMANIZE', 'loudness and timing wander', (v) => v + ' %'],
    ['FADE', 'at the ends of every slice', (v) => v + ' ms'],
  ];
  const diceEls = [];
  DICE.forEach(([name, hint], k) => {
    const r = document.createElement('div');
    r.className = 'dicerow';
    r.innerHTML = '<span>' + name + '</span><button>−</button><b></b><button>+</button><small>' + hint + '</small>';
    const [lo, , hi] = [r.children[1], r.children[2], r.children[3]];
    lo.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: 'slmang', i: k, n: -1 }); });
    hi.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: 'slmang', i: k, n: 1 }); });
    $('mangleRows').appendChild(r);
    diceEls.push(r.children[2]);
  });
  function drawDice() {
    const sl = state && state.slicer;
    if (!sl || !sl.mangle) return;
    diceEls.forEach((b, k) => { b.textContent = sl.mangle[k] ? DICE[k][2](sl.mangle[k]) : 'off'; b.classList.toggle('on', sl.mangle[k] > 0); });
  }
  $('slMangle').addEventListener('pointerdown', (e) => { e.preventDefault(); $('manglePick').hidden = false; drawDice(); });
  $('mangleX').addEventListener('pointerdown', (e) => { e.preventDefault(); $('manglePick').hidden = true; });
  $('manglePick').addEventListener('pointerdown', (e) => { if (e.target === $('manglePick')) $('manglePick').hidden = true; });
  $('mangleReset').addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: 'slmang', i: 6, n: 0 }); });

  // ---- choosing the bank ----
  // The same list serves the unit's bank (the chip at the top), the bank the Looper's loops go to, and the banks of the Slicer.
  const bankGrid = $('bankGrid');
  let bankKind = 'unit';
  const BANKS = {
    unit: { title: 'Bank on the unit', cmd: 'bank', cur: (s) => s.bank },
    loop: { title: 'Bank for the loops', cmd: 'lpbankset', cur: (s) => s.looper && s.looper.bank },
    from: { title: 'Slice a pad of bank', cmd: 'slbankset', cur: (s) => s.slicer && s.slicer.bank },
    to: { title: 'Slices go to bank', cmd: 'sltbankset', cur: (s) => s.slicer && s.slicer.target },
    ptn: { title: 'Patterns of bank', cmd: 'ptnbankset', cur: (s) => s.ptn && s.ptn.bank },
  };
  for (let i = 0; i < 10; i++) {
    const b = document.createElement('button');
    b.innerHTML = String.fromCharCode(65 + i) + '<small></small>';
    b.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: BANKS[bankKind].cmd, i }); $('bankPick').hidden = true; });
    bankGrid.appendChild(b);
  }
  function drawBanks() {
    if ($('bankPick').hidden || !state) return;
    const cur = BANKS[bankKind].cur(state);
    [...bankGrid.children].forEach((b, i) => {
      b.className = cur === String.fromCharCode(65 + i) ? 'now' : '';
      b.lastChild.textContent = state.bankFill ? state.bankFill[i] + ' pads' : '';
    });
  }
  function openBanks(kind) { bankKind = kind; $('bankT').textContent = BANKS[kind].title; $('bankPick').hidden = false; drawBanks(); }
  $('bankBtn').addEventListener('pointerdown', (e) => {
    e.preventDefault();
    const inLp = app.dataset.view === 'looper';
    openBanks(app.dataset.view === 'patterns' ? 'ptn' : inLp ? ($('looper').dataset.lp === 'slice' ? 'from' : 'loop') : 'unit');
  });
  for (const [id, kind] of [['slBank', 'from'], ['slTBank', 'to']]) {
    const box = $(id).closest('.lpmid');
    box.classList.add('pick');
    box.addEventListener('pointerdown', (e) => { e.preventDefault(); if (!box.parentElement.querySelector('button:disabled')) openBanks(kind); });
  }
  $('bankX').addEventListener('pointerdown', (e) => { e.preventDefault(); $('bankPick').hidden = true; });
  $('bankPick').addEventListener('pointerdown', (e) => { if (e.target === $('bankPick')) $('bankPick').hidden = true; });

  // ---- drawing the state, at most once a frame ----
  let queued = false;
  function schedule() { if (!queued) { queued = true; requestAnimationFrame(() => { queued = false; render(); }); } }
  const set = (el, prop, v) => { if (el[prop] !== v) el[prop] = v; };
  const cls = (el, c, on) => { if (el.classList.contains(c) !== !!on) el.classList.toggle(c, !!on); };
  let toastT = 0, lastMsg = '';
  function toast(t) {
    if (!t || t === lastMsg) { lastMsg = t; return; }
    lastMsg = t; const el = $('toast'); el.textContent = t; el.classList.add('show');
    clearTimeout(toastT); toastT = setTimeout(() => el.classList.remove('show'), 3200);
  }
  const MOSTATE = ['', 'next bar', '', 'next bar', ''];

  function curvePath(c) {
    if (!c || !c.length) return '';
    return c.map((v, i) => (i ? 'L' : 'M') + (i * 64 / (c.length - 1)).toFixed(1) + ' ' + (19 - v / 127 * 18).toFixed(1)).join(' ');
  }

  function render() {
    const s = state;
    if (!s) return;
    if (ws && ws.readyState === 1 && rtt !== null) setConn(s.unit ? 'ok' : 'nounit', (s.unit ? '' : 'no unit · ') + rtt + ' ms');
    $('waiting').hidden = s.unit;
    renderLooper(s);
    renderPunch(s.punch);
    if (!$('presetPick').hidden) drawPresets();
    if (s.ptn) renderPatterns(s.ptn);
    renderMeters(s.level, s.levelNote);
    set($('bank'), 'textContent', s.bank || '–');
    // In the Looper the chip is the bank the loops go to (in the Slicer: the bank a pad is taken from); elsewhere the unit's bank.
    const inLp = app.dataset.view === 'looper', inPtn = app.dataset.view === 'patterns';
    const lpPage = $('looper').dataset.lp;
    set($('bankTop'), 'textContent', (inPtn && s.ptn ? s.ptn.bank : inLp && s.looper ? (lpPage === 'slice' && s.slicer ? s.slicer.bank : s.looper.bank) : s.bank) || '–');
    set($('bankLbl'), 'textContent', inPtn ? 'PTN' : inLp ? (lpPage === 'slice' ? 'FROM' : 'LOOP') : 'BANK');
    set($('bpm'), 'textContent', s.bpm ? s.bpm.toFixed(1) : '–');
    set($('bar'), 'textContent', s.running || s.beat >= 0 ? 'BAR ' + s.bar + ' · BEAT ' + (s.beat + 1) + (s.running ? '' : ' (stopped)') : 'no clock');
    const beats = $('beats').children;
    for (let i = 0; i < 4; i++) cls(beats[i], 'on', s.beat === i && s.running);
    cls($('beats'), 'stopped', !s.running);
    cls($('bRec'), 'hot', s.rec); set($('bRecT'), 'textContent', s.rec ? 'STOP REC' : 'RECORD');

    s.pads.forEach((p, i) => {
      const e = padEls[i], d = e.d;
      const k = [p.name, p.has, p.bus, p.bars].join('|');
      if (k !== e.last) {
        e.last = k;
        e.n.textContent = p.n; e.nm.textContent = p.name; e.bars.textContent = p.bars; e.bus.textContent = p.bus || '';
        cls(d, 'empty', !p.has); cls(d, 'b1', p.has && p.bus === 1); cls(d, 'b2', p.has && p.bus === 2);
      }
      cls(d, 'heard', p.heard); cls(d, 'inPat', p.inPat); cls(d, 'sel', p.sel); cls(d, 'keep', p.keep); cls(d, 'dry', p.dry); cls(d, 'muted', p.muted);
      d.style.setProperty('--glow', (p.glow * 0.6).toFixed(2));
      const w = (p.level / 127 * 100).toFixed(1) + '%';
      if (e.lvi.style.width !== w) e.lvi.style.width = w;
      d.dataset.lv = p.level;
    });

    [...barsEl.children].forEach((b) => cls(b, 'on', +b.dataset.n === s.motion.bars));
    cls($('bDub'), 'hot', s.motion.overdub);
    s.motion.bus.forEach((mb, bus) => {
      const be = busEls[bus], fx = s.fx[bus];
      const txt = (fx.seen ? (fx.on ? '<em>● ON</em> ' : '○ off ') : '') + (fx.name || '') + (fx.note ? ' · ' + fx.note : '');
      if (be.fx.dataset.t !== txt) { be.fx.dataset.t = txt; be.fx.innerHTML = txt.replace(/&/g, '&amp;').replace(/<(?!\/?em>)/g, '&lt;') || '<span style="color:var(--faint)">effect not sent yet</span>'; }
      fx.knobs.forEach((v, k) => { const w = v < 0 ? '0%' : (v / 127 * 100) + '%'; if (be.knobs[k].style.width !== w) be.knobs[k].style.width = w; be.knobs[k].parentElement.style.opacity = v < 0 ? .4 : 1; });
      mb.slots.forEach((sl, k) => {
        const e = be.slotEls[k];
        cls(e.b, 'rec', sl.state === 2); cls(e.b, 'play', sl.state === 4); cls(e.b, 'wait', sl.state === 1 || sl.state === 3);
        cls(e.b, 'filled', sl.filled); cls(e.b, 'sel', mb.sel === k);
        let t = MOSTATE[sl.state];
        if (sl.state === 2) t = 'REC ' + Math.min(sl.bars, Math.floor(sl.pos * sl.bars) + 1) + '/' + sl.bars;
        else if (!t) t = sl.filled ? sl.bars + (sl.bars === 1 ? ' bar' : ' bars') + (sl.speed !== 1 ? ' · ' + (sl.speed === 0.5 ? '½' : sl.speed) + '×' : '') + (sl.rev ? ' ◀' : '') : 'empty';
        if (sl.state === 4 && (sl.speed !== 1 || sl.rev)) t = '▶ ' + (sl.speed === 0.5 ? '½' : sl.speed) + '×' + (sl.rev ? ' ◀' : '');
        if (e.st.textContent !== t) e.st.textContent = t;
        const ck = (sl.curve || []).join(',');
        if (e.key !== ck) { e.key = ck; e.path.setAttribute('d', curvePath(sl.curve)); }
        e.pos.style.width = sl.state === 4 || sl.state === 2 ? (sl.pos * 100) + '%' : '0';
      });
      const cur = mb.slots[mb.sel];
      be.ctlEls[0].small.textContent = cur.speed === 0.5 ? '½×' : cur.speed + '×';
      be.ctlEls[1].small.textContent = cur.rev ? '◀ back' : 'fwd';
      be.ctlEls[2].small.textContent = '#' + (mb.sel + 1);
      cls(be.ctlEls[0].b, 'on', cur.speed !== 1); cls(be.ctlEls[1].b, 'on', cur.rev);
    });
    drawBanks();
    toast(s.msg);
  }

  // Keep the screen on: on stage nobody touches the phone for minutes.
  async function wake() { try { if (navigator.wakeLock && document.visibilityState === 'visible') await navigator.wakeLock.request('screen'); } catch (e) {} }
  document.addEventListener('visibilitychange', () => { wake(); if (document.visibilityState === 'visible' && !ws) { retry = 0; open(); } });
  document.addEventListener('pointerdown', wake, { once: true });
  // No pinch zoom or pull to refresh.
  document.addEventListener('gesturestart', (e) => e.preventDefault());
  document.addEventListener('touchmove', (e) => { if (e.touches.length > 1) e.preventDefault(); }, { passive: false });

  window.__render = render; window.__setState = (s) => { state = s; app.hidden = false; $('connect').hidden = true; render(); };
  open();
})();
