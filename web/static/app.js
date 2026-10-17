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
      store.set('host', host); if (key) store.set('key', key); setConn('ok', 'connected'); ping(); };
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
  let mixing = false;
  // A tap mutes the pad (its level goes to 0 and back); two taps in a row make it the only one on its bus.
  // With focus on, the pads are picked: the program decides what a tap means then.
  let lastTap = { i: -1, t: 0 };
  function tap(i) {
    if (state && state.focus && state.focus.on) return send({ a: 'pad', i });
    const now = performance.now();
    if (lastTap.i === i && now - lastTap.t < 380) { lastTap = { i: -1, t: 0 }; return send({ a: 'solo', i }); }
    lastTap = { i, t: now };
    send({ a: 'mute', i });
  }
  function pad(el, i) {
    let down = null;
    el.addEventListener('pointerdown', (e) => {
      try { el.setPointerCapture(e.pointerId); } catch (err) {}
      down = { y: e.clientY, x: e.clientX, moved: false, at: performance.now() };
      if (!mixing) tap(i);
    });
    el.addEventListener('pointermove', (e) => {
      if (!down || !mixing) return;
      if (Math.abs(e.clientY - down.y) > 8) down.moved = true;
      if (!down.moved) return;
      const r = el.getBoundingClientRect();
      const v = Math.round(Math.max(0, Math.min(1, 1 - (e.clientY - r.top) / r.height)) * 127);
      const now = performance.now();
      if (now - (down.sent || 0) > 33 && v !== down.v) { down.sent = now; down.v = v; send({ a: 'level', i, v }); padEls[i].lvi.style.width = (v / 127 * 100) + '%'; el.dataset.lv = v; }
    });
    const up = () => {
      if (down && mixing && !down.moved) send({ a: 'mute', i });
      else if (down && mixing && down.moved && down.v !== undefined) send({ a: 'level', i, v: down.v });
      down = null;
    };
    el.addEventListener('pointerup', up);
    el.addEventListener('pointercancel', () => { down = null; });
    el.addEventListener('contextmenu', (e) => e.preventDefault());
  }

  // Buttons with a data-a send it as they are pressed down, not when the finger comes up.
  document.querySelectorAll('[data-a]').forEach((b) => b.addEventListener('pointerdown', (e) => { e.preventDefault(); send({ a: b.dataset.a }); }));
  $('bMix').addEventListener('pointerdown', (e) => { e.preventDefault(); mixing = !mixing; document.body.classList.toggle('mixing', mixing); $('bMix').classList.toggle('on', mixing);
    $('bMixS').textContent = mixing ? 'drag = volume · tap = mute' : 'drag a pad = volume'; if (navigator.vibrate) navigator.vibrate(8); render(); });

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

  // tabs (portrait)
  const app = $('app');
  app.dataset.tab = store.get('tab') || 'pads';
  document.querySelectorAll('#tabs button').forEach((b) => {
    const sync = () => b.classList.toggle('on', b.dataset.tab === app.dataset.tab);
    b.addEventListener('pointerdown', (e) => { e.preventDefault(); app.dataset.tab = b.dataset.tab; store.set('tab', b.dataset.tab); document.querySelectorAll('#tabs button').forEach((x) => x.classList.toggle('on', x.dataset.tab === b.dataset.tab)); });
    sync();
  });

  // ---- choosing the pattern that plays now or next ----
  let picking = 0;
  const pickGrid = $('pickGrid');
  for (let i = 0; i < 16; i++) {
    const b = document.createElement('button');
    b.addEventListener('pointerdown', (e) => { e.preventDefault(); if (!picking) return; send({ a: 'cue', i, n: picking }); closePick(); });
    pickGrid.appendChild(b);
  }
  function openPick(n) { picking = n; $('picker').hidden = false; drawPick(); }
  function closePick() { picking = 0; $('picker').hidden = true; }
  function drawPick() {
    if (!picking || !state) return;
    $('pickT').textContent = (picking === 1 ? 'Plays now' : 'Plays next') + ' · bank ' + state.bank;
    [...pickGrid.children].forEach((b, i) => {
      const p = (state.patterns || [])[i];
      const ok = !!(p && p.exists);
      b.disabled = !ok;
      b.className = p && p.now ? 'now' : p && p.next ? 'next' : '';
      b.innerHTML = (i + 1) + (ok ? '<small>' + p.pads + ' pad' + (p.pads === 1 ? '' : 's') + (p.now ? ' · now' : p.next ? ' · next' : '') + '</small>' : '<small>empty</small>');
    });
    $('pickS').textContent = (state.patterns || []).some((p) => p.exists) ? 'Tap a pattern.' : 'No patterns in this bank (the unit’s bank decides).';
  }
  $('cNow').addEventListener('pointerdown', (e) => { e.preventDefault(); openPick(1); });
  $('cNext').addEventListener('pointerdown', (e) => { e.preventDefault(); openPick(2); });
  $('pickX').addEventListener('pointerdown', (e) => { e.preventDefault(); closePick(); });
  $('picker').addEventListener('pointerdown', (e) => { if (e.target === $('picker')) closePick(); });

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
    set($('bank'), 'textContent', s.bank || '–');
    set($('bpm'), 'textContent', s.bpm ? s.bpm.toFixed(1) : '–');
    set($('bar'), 'textContent', s.running || s.beat >= 0 ? 'BAR ' + s.bar + ' · BEAT ' + (s.beat + 1) + (s.running ? '' : ' (stopped)') : 'no clock');
    const beats = $('beats').children;
    for (let i = 0; i < 4; i++) cls(beats[i], 'on', s.beat === i && s.running);
    cls($('beats'), 'stopped', !s.running);
    set($('now'), 'textContent', s.now); set($('next'), 'textContent', s.next);
    cls($('bRec'), 'hot', s.rec); set($('bRecT'), 'textContent', s.rec ? 'STOP REC' : 'RECORD');
    cls($('bFocus'), 'on', s.focus.on);
    set($('bFocusT'), 'textContent', s.focus.on ? 'FOCUS ON' : 'FOCUS');
    set($('bFocusS'), 'textContent', s.focus.on ? (s.focus.multi ? 'tap pads to keep · ' + s.focus.dry + ' dry' : 'tap again: all back') : 'tap, then pads');

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
    drawPick();
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
