'use strict';
// 404tui tools in the page: the program's own tools (the pattern's tools and settings) as the terminal has them, with every page, setting and value.
// The state comes from /tools/api (one JSON call for each action, the whole state back); nothing is kept here but what is on the screen.
(() => {
  const $ = (id) => document.getElementById(id);
  const qs = new URLSearchParams(location.search);
  const store = { get(k) { try { return localStorage.getItem(k) || ''; } catch (e) { return ''; } } };
  const key = qs.get('key') || store.get('key');
  if (qs.get('embed')) { document.body.classList.add('embed'); $('bankOpen').hidden = false; }
  $('back').href = './' + (key ? '?key=' + encodeURIComponent(key) : '');
  let lastProject = 0, st = null, busy = false, editing = 0, mode = 'pattern', pads = null, cbank = 0, win = null, armed = -1, armTimer = 0;

  async function call(body) {
    const url = '/tools/api' + (key ? '?key=' + encodeURIComponent(key) : '');
    try {
      const r = await fetch(url, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(body || { op: 'state' }) });
      if (!r.ok) throw new Error(r.status === 403 ? 'wrong key' : 'error ' + r.status);
      st = await r.json();
      render();
    } catch (e) {
      $('status').textContent = String(e.message || e); $('status').className = 'status bad';
    }
  }
  const act = (body) => { busy = true; return call(body).finally(() => { busy = false; }); };

  const bankNames = 'ABCDEFGHIJ';
  for (let i = 0; i < 10; i++) { const o = document.createElement('option'); o.value = i; o.textContent = bankNames[i]; $('bank').appendChild(o); }
  for (let i = 0; i < 16; i++) { const o = document.createElement('option'); o.value = i; o.textContent = i + 1; $('pat').appendChild(o); }
  const slot = () => (+$('bank').value) * 16 + (+$('pat').value) + 1;
  $('bankOpen').onclick = () => window.parent.postMessage({ t: 'openBank' }, location.origin); // the page's own bank dialog
  $('bank').onchange = () => { act({ op: 'pattern', slot: slot() }); if (window.parent !== window) window.parent.postMessage({ t: 'bank', i: +$('bank').value }, location.origin); };
  $('pat').onchange = () => act({ op: 'pattern', slot: slot() });
  window.addEventListener('message', (e) => { // the page around this one has another bank chosen
    if (e.origin !== location.origin || !e.data || e.data.t !== 'bank' || !(e.data.i >= 0 && e.data.i < 10)) return;
    if (+$('bank').value !== e.data.i) { $('bank').value = e.data.i; act({ op: 'pattern', slot: e.data.i * 16 + (+$('pat').value) + 1 }); }
  });
  $('play').onclick = () => act({ op: 'play', on: !(st && st.playing) });
  $('undo').onclick = () => act({ op: 'undo' });
  $('redo').onclick = () => act({ op: 'redo' });
  $('open').onclick = () => act({ op: 'open' });
  $('mback').onclick = () => act({ op: 'back' });
  $('mdone').onclick = () => act({ op: 'close-menus' });
  $('sheetBack').onclick = () => act({ op: 'close-menus' });

  // the three kinds of tools: the pattern's, the slicer's, a pad's
  const setMode = (m) => {
    mode = m;
    if (m !== 'slicer' && st && st.screen === 'slicer') return act({ op: 'slicer-close' });
    if (m !== 'pattern' && st && st.menu && !st.slicer) return act({ op: 'escape' });
    render();
  };
  $('mPattern').onclick = () => setMode('pattern'); $('mSlicer').onclick = () => setMode('slicer'); $('mPad').onclick = () => setMode('pad');
  $('otherClose').onclick = () => act({ op: 'escape' });
  // the pad chooser uses the bank of the whole page (its dialog is the page's own, when this is a tab of it; else the pattern's bank select)
  $('cbankOpen').onclick = () => { if (window.parent !== window) window.parent.postMessage({ t: 'openBank' }, location.origin); else { $('bank').focus(); } };
  const loadPads = () => fetch('/tools/api' + (key ? '?key=' + encodeURIComponent(key) : ''), { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ op: 'pads' }) }).then((r) => r.json()).then((p) => { pads = p; renderChooser(); }).catch(() => {});

  // the pad chooser: a bank of 16, a pad with a sample for the slicer, any pad for the tools
  function renderChooser() {
    const box = $('chooser'); const show = (mode === 'slicer' && !(st && st.slicer)) || (mode === 'pad' && !(st && st.menu));
    box.hidden = !show; if (!show) return;
    if (!pads) { loadPads(); return; }
    cbank = +$('bank').value; $('cbankL').textContent = bankNames[cbank];
    $('chooseWhy').textContent = mode === 'slicer' ? 'Choose a pad with a sample to slice.' : 'Choose a pad: its tools open.';
    const cells = $('cells'); cells.textContent = '';
    for (let i = 0; i < 16; i++) {
      const p = pads[cbank * 16 + i]; if (!p) continue;
      const b = document.createElement('button'); b.className = 'btn' + (p.has ? '' : ' empty');
      const l = document.createElement('span'); l.textContent = bankNames[cbank] + (i + 1);
      const n = document.createElement('small'); n.textContent = p.has ? p.name : '—';
      b.append(l, n);
      if (mode === 'slicer' && !p.has) b.disabled = true;
      b.onclick = () => act({ op: mode === 'slicer' ? 'slicer' : 'pad-tools', pad: p.pad });
      cells.appendChild(b);
    }
  }

  // the slicer: the waveform, the slices, the cursor
  const wave = $('wave');
  function drawWave() {
    const s = st && st.slicer; if (!s) return;
    const dpr = window.devicePixelRatio || 1, W = Math.max(wave.clientWidth, 100), H = 170;
    if (wave.width !== Math.round(W * dpr) || wave.height !== Math.round(H * dpr)) { wave.width = Math.round(W * dpr); wave.height = Math.round(H * dpr); }
    const g = wave.getContext('2d'); g.setTransform(dpr, 0, 0, dpr, 0, 0); g.clearRect(0, 0, W, H);
    const css = getComputedStyle(document.documentElement), accent = css.getPropertyValue('--accent').trim(), dim = css.getPropertyValue('--dim').trim(), text = css.getPropertyValue('--text').trim();
    const span = Math.max(s.to - s.from, 1), X = (f) => (f - s.from) / span * W;
    s.slices.forEach((sl, i) => {
      const x0 = Math.max(X(sl.start), 0), x1 = Math.min(X(sl.end), W);
      if (x1 < 0 || x0 > W) return;
      g.fillStyle = i === s.sel ? 'rgba(255,176,0,.16)' : (i % 2 ? 'rgba(255,255,255,.03)' : 'rgba(255,255,255,.07)'); g.fillRect(x0, 0, x1 - x0, H);
      g.fillStyle = dim; g.font = '11px system-ui'; if (x1 - x0 > 16) g.fillText(String(i + 1), x0 + 4, 12);
    });
    const mins = s.mins || [], maxs = s.maxs || [], rms = s.rms || [], n = maxs.length || 1, mid = H / 2, amp = H / 2 - 8, cw = W / n;
    // the shape: from the lowest to the highest sample of each column, and inside it the loudness (RMS)
    g.fillStyle = 'rgba(255,176,0,.55)';
    for (let c = 0; c < n; c++) { const y0 = mid - maxs[c] / 32768 * amp, y1 = mid - mins[c] / 32768 * amp; g.fillRect(c * cw, y0, Math.max(cw, 1), Math.max(y1 - y0, 1)); }
    g.fillStyle = accent;
    for (let c = 0; c < n; c++) { const h = rms[c] / 32768 * amp; g.fillRect(c * cw, mid - h, Math.max(cw, 1), Math.max(h * 2, 1)); }
    g.strokeStyle = 'rgba(255,255,255,.18)'; g.lineWidth = 1; g.beginPath(); g.moveTo(0, mid + .5); g.lineTo(W, mid + .5); g.stroke();
    // a time scale along the bottom
    g.fillStyle = dim; g.font = '10px system-ui';
    const secs = span / s.rate, stepS = [0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60].find((v) => secs / v <= 8) || 60;
    for (let t = Math.ceil(s.from / s.rate / stepS) * stepS; t < s.to / s.rate; t += stepS) { const x = X(t * s.rate); g.fillRect(x, H - 8, 1, 8); g.fillText((stepS < 1 ? t.toFixed(2) : t.toFixed(0)) + ' s', x + 3, H - 10); }
    g.strokeStyle = dim; g.lineWidth = 1;
    s.slices.forEach((sl) => { const x = X(sl.start); if (sl.start > 0 && x >= 0 && x <= W) { g.beginPath(); g.moveTo(x + .5, 0); g.lineTo(x + .5, H); g.stroke(); } });
    const cx = X(localAt !== null && Date.now() - localTs < 1500 ? localAt : s.at); g.strokeStyle = text; g.lineWidth = 2; g.beginPath(); g.moveTo(cx, 0); g.lineTo(cx, H); g.stroke();
  }
  const frameAt = (ev) => { const s = st.slicer, r = wave.getBoundingClientRect(); return Math.round(s.from + Math.min(Math.max((ev.clientX - r.left) / r.width, 0), 1) * (s.to - s.from)); };
  // the cursor follows the finger: it is drawn at once from here and sent to the program about every 70 ms
  let localAt = null, localTs = 0, dragging = false, sentAt = 0, sentVal = -1;
  const atNow = () => (localAt !== null && Date.now() - localTs < 1500 ? localAt : st.slicer.at);
  const moveTo = (ev, last) => {
    if (!st || !st.slicer) return;
    localAt = frameAt(ev); localTs = Date.now(); drawWave();
    const now = Date.now();
    if (last || now - sentAt > 70) { if (localAt !== sentVal || last) { sentAt = now; sentVal = localAt; act({ op: 'slicer-at', i: localAt }); } }
  };
  wave.addEventListener('pointerdown', (ev) => { dragging = true; try { wave.setPointerCapture(ev.pointerId); } catch (e) { /* no capture */ } moveTo(ev, false); });
  wave.addEventListener('pointermove', (ev) => { if (dragging) moveTo(ev, false); });
  const stopDrag = (ev) => { if (dragging) { dragging = false; moveTo(ev, true); } };
  wave.addEventListener('pointerup', stopDrag); wave.addEventListener('pointercancel', () => { dragging = false; });
  const cur = () => atNow();
  $('slCut').onclick = () => act({ op: 'slicer-cut', i: cur() });
  $('slJoin').onclick = () => act({ op: 'slicer-join', i: cur() });
  $('slPlay').onclick = () => act({ op: 'slicer-play', i: cur() });
  $('slOrder').onclick = () => act({ op: 'slicer-order', on: !st.slicer.playing });
  $('slShuffle').onclick = () => act({ op: 'slicer-shuffle' });
  $('slUndo').onclick = () => act({ op: 'slicer-undo', on: false });
  $('slRedo').onclick = () => act({ op: 'slicer-undo', on: true });
  $('slClose').onclick = () => act({ op: 'slicer-close' });
  $('slSet').onclick = () => act({ op: 'slicer-menu', kind: 'settings' });
  $('slTools').onclick = () => act({ op: 'slicer-menu', kind: 'tools' });
  const zoom = (f) => { const s = st.slicer; if (!s) return; const mid = s.at, w = Math.max(Math.round((s.to - s.from) * f), 400), from = Math.min(Math.max(mid - w / 2, 0), Math.max(s.frames - w, 0)); act({ op: 'slicer-view', from: Math.round(from), to: Math.min(Math.round(from + w), s.frames), cols: Math.min(Math.round(wave.clientWidth), 1600) }); };
  $('zIn').onclick = () => zoom(0.5); $('zOut').onclick = () => zoom(2); $('zAll').onclick = () => act({ op: 'slicer-view', from: 0, to: 0, cols: Math.min(Math.round(wave.clientWidth), 1600) });
  window.addEventListener('resize', () => { if (st && st.slicer) drawWave(); });

  // a pad or pattern to choose, a text to type: the dialogs a menu may open
  function renderModal() {
    const m = $('modal'), p = st.picker, t = st.prompt;
    m.hidden = !(p || t);
    if (!p && !t) return;
    if (document.activeElement && m.contains(document.activeElement) && t) return; // typing: leave it
    $('dTitle').textContent = p ? p.title : t.title;
    const body = $('dBody'); body.textContent = '';
    $('dOk').hidden = !t;
    if (p) {
      const row = document.createElement('div'); row.className = 'row';
      const lab = document.createElement('label'); lab.textContent = 'BANK ';
      const sel = document.createElement('select'); for (let i = 0; i < 10; i++) { const o = document.createElement('option'); o.value = i; o.textContent = bankNames[i]; sel.appendChild(o); }
      sel.value = p.bank; sel.onchange = () => act({ op: 'picker-bank', i: +sel.value }); lab.appendChild(sel); row.appendChild(lab); body.appendChild(row);
      const grid = document.createElement('div'); grid.id = 'cells';
      p.cells.forEach((c, i) => {
        const b = document.createElement('button'); b.className = 'btn' + (c.used ? ' on' : '');
        const l = document.createElement('span'); l.textContent = bankNames[p.bank] + (i + 1); const n = document.createElement('small'); n.textContent = c.name || c.info || (c.used ? '' : '—');
        b.append(l, n); b.onclick = () => act({ op: 'picker-pick', i }); grid.appendChild(b);
      });
      body.appendChild(grid);
    } else {
      if (t.hint) { const h = document.createElement('p'); h.className = 'dim'; h.textContent = t.hint; body.appendChild(h); }
      const inp = document.createElement('input'); inp.type = 'text'; inp.value = t.text; inp.autocapitalize = 'off'; inp.spellcheck = false; body.appendChild(inp);
      inp.onkeydown = (e) => { if (e.key === 'Enter') $('dOk').click(); };
      setTimeout(() => inp.focus(), 0);
      $('dOk').onclick = () => act({ op: 'prompt', text: inp.value, done: true, on: true });
    }
  }
  $('dCancel').onclick = () => act({ op: 'escape' });

  // the tracks of the pattern: the one in front is the one the tools act on (the pattern itself is on the unit)
  function renderGrid(p) {
    const g = $('grid'); g.textContent = '';
    const head = document.createElement('div'); head.className = 'thead';
    head.textContent = p.tracks.length ? 'TRACK: tap the one the tools work on' : '';
    if (!p.exists || !p.tracks.length) {
      const d = document.createElement('div'); d.className = 'empty'; d.textContent = p.exists ? 'The pattern has no notes.' : 'No pattern in ' + p.label + ' yet. A tool writes one.'; g.appendChild(d); return;
    }
    g.appendChild(head);
    const box = document.createElement('div'); box.className = 'tracks';
    for (const t of p.tracks) {
      const b = document.createElement('button'); b.className = 'trk' + (t.pad === st.pad ? ' sel' : '');
      const l = document.createElement('b'); l.textContent = t.label;
      const n = document.createElement('span'); n.textContent = t.name || 'pad';
      const c = document.createElement('small'); c.textContent = t.notes.length + (t.notes.length === 1 ? ' note' : ' notes') + (t.pad === st.pad ? ' · in front' : '');
      b.append(l, n, c); b.onclick = () => act({ op: 'track', pad: t.pad }); box.appendChild(b);
    }
    g.appendChild(box);
  }

  function renderMenu(m) {
    const wasOpen = !$('menu').hidden;
    $('menu').hidden = !m; $('sheetBack').hidden = !m;
    if (!m) return;
    if (!wasOpen) $('menu').scrollTop = 0;
    // the menu is a sheet over the page: DONE leaves it (a trial that is open is thrown away: then it says so), BACK goes up one level
    const trial = m.items.some((it) => /^keep it/i.test(it.label));
    $('mtitle').textContent = m.title;
    $('mback').hidden = m.depth <= 1 && !trial; $('mback').textContent = trial ? '◂ DISCARD' : '◂ BACK';
    $('mdone').hidden = trial; // a tool's dialog has one way out besides Keep it: DISCARD
    const tabs = $('tabs'); tabs.textContent = '';
    (m.tabs || []).forEach((name, i) => { const b = document.createElement('button'); b.className = 'btn' + (i === m.tab ? ' on' : ''); b.textContent = name; b.onclick = () => act({ op: 'tab', tab: i }); tabs.appendChild(b); });
    const box = $('items'); if (editing && document.activeElement && box.contains(document.activeElement)) return; // do not replace a control while it is in the hand
    box.textContent = '';
    let tiles = null; // consecutive actions are drawn as tiles side by side: a tool is a tile, a tap opens its options
    for (const it of m.items) {
      if (it.kind === 'action') {
        if (!tiles) { tiles = document.createElement('div'); tiles.className = 'tiles'; box.appendChild(tiles); }
        const b = document.createElement('button');
        b.className = 'tile' + (/^keep it/i.test(it.label) ? ' go wide' : '') + (it.opens ? ' opens' : '');
        b.textContent = it.label.replace(/\.\.\.$/, '').replace(/\s*\([^)]*\)\s*$/, ''); b.title = it.label;
        b.onclick = () => act({ op: 'run', i: it.i }); tiles.appendChild(b);
        continue;
      }
      tiles = null;
      const d = document.createElement('div');
      if (it.kind === 'head') { d.className = 'item head'; d.textContent = it.label; }
      else {
        d.className = 'item choice';
        const lab = document.createElement('div'); lab.className = 'lab'; lab.textContent = it.label;
        const ctl = document.createElement('div'); ctl.className = 'ctl';
        const prev = document.createElement('button'); prev.className = 'btn step-btn'; prev.textContent = '◂';
        const sel = document.createElement('select');
        it.vals.forEach((v, i) => { const o = document.createElement('option'); o.value = i; o.textContent = v; sel.appendChild(o); });
        sel.value = it.val;
        const next = document.createElement('button'); next.className = 'btn step-btn'; next.textContent = '▸';
        const set = (v) => act({ op: 'set', i: it.i, val: ((v % it.vals.length) + it.vals.length) % it.vals.length });
        sel.onchange = () => set(+sel.value); prev.onclick = () => set(it.val - 1); next.onclick = () => set(it.val + 1);
        sel.onfocus = () => { editing++; }; sel.onblur = () => { editing = Math.max(0, editing - 1); };
        ctl.append(prev, sel, next);
        if (it.runs) { const go = document.createElement('button'); go.className = 'btn go'; go.textContent = 'DO IT'; go.title = 'Do it with the value chosen'; go.onclick = () => act({ op: 'run', i: it.i }); ctl.appendChild(go); }
        d.append(lab, ctl);
      }
      box.appendChild(d);
    }
  }

  // the unit's current project: the page follows it (the program loads it again when the unit's project changes)
  $('ubMode').onclick = () => act({ op: 'live', on: !(st && st.mode === 'live') });
  $('ubSend').onclick = () => {
    if (armed !== 1) { armed = 1; clearTimeout(armTimer); armTimer = setTimeout(() => { armed = -1; renderUnit(); }, 5000); renderUnit(); return; }
    armed = -1; clearTimeout(armTimer); act({ op: 'send', i: st.project - 1 });
  };
  $('ubReload').onclick = () => act({ op: 'reload' });
  function renderUnit() {
    const live = st.mode === 'live', has = !!st.project;
    $('ubTitle').textContent = has ? 'Unit project ' + st.project + (st.name ? ' · ' + st.name : '') : st.unit ? 'loading the unit\'s project…' : 'no unit';
    $('ubMode').hidden = !has; $('ubMode').textContent = live ? 'LIVE' : 'COPY'; $('ubMode').classList.toggle('on', live);
    $('ubReload').hidden = !has || !live;
    $('ubSend').hidden = !has || live; $('ubSend').textContent = armed === 1 ? 'SURE? REPLACES ' + st.project : 'SEND TO UNIT'; $('ubSend').classList.toggle('go', armed === 1);
    const w = $('ubWarn'); w.hidden = !has;
    w.textContent = live ? 'Live: every change you keep goes to the unit. Back the project up first. The unit shows changed patterns only after it reloads the project, which stops the sound.' : 'Copy: the unit is not changed. LIVE writes each kept change to it; SEND writes the whole project over it.';
  }
  function renderStrip(p) {
    const box = $('ptnStrip'); box.textContent = '';
    (st.patterns || []).forEach((c, i) => {
      const bank = Math.floor((p.slot - 1) / 16), b = document.createElement('button');
      b.className = 'btn' + (c.used ? ' used' : '') + (p.slot - 1 === bank * 16 + i ? ' on' : ''); b.title = c.used ? c.info : 'empty';
      const n = document.createElement('span'); n.textContent = i + 1; const sm = document.createElement('small'); sm.textContent = c.used ? c.info : ''; b.append(n, sm);
      b.onclick = () => act({ op: 'pattern', slot: bank * 16 + i + 1 }); box.appendChild(b);
    });
  }

  function render() {
    const p = st.pattern;
    $('bank').value = Math.floor((p.slot - 1) / 16); $('bankOpenL').textContent = bankNames[Math.floor((p.slot - 1) / 16)]; $('pat').value = (p.slot - 1) % 16;
    $('patinfo').textContent = 'Pattern ' + p.label + (p.exists ? ' · ' + p.bars + ' bars · ' + Math.round(p.tempo) + ' BPM' : ' · empty');
    $('play').textContent = st.playing ? '■ STOP' : '▶ PLAY HERE'; $('play').classList.toggle('on', !!st.playing);
    const u = $('unit'); u.className = 'unit' + (st.live ? ' live' : st.unit ? ' on' : '');
    $('unitText').textContent = st.live ? 'live: unit project ' + st.live : st.unit ? 'unit: ' + st.unit : 'no unit';
    let line = st.msg || '';
    if (st.job && !st.job.ended) line = st.job.what + (st.job.total ? ' ' + st.job.done + '/' + st.job.total : '') + ' ' + st.job.name;
    if (st.liveErr) line = 'writing to the unit failed: ' + st.liveErr; else if (st.sending) line = 'sending to the unit… (' + st.sending + ')';
    if (!line && st.job && st.job.ended) line = st.job.error ? 'failed: ' + st.job.error : (st.job.what || 'sent to the unit') + ': done';
    $('status').textContent = line; $('status').className = 'status' + (st.msgBad || st.liveErr || (st.job && st.job.error) ? ' bad' : '');
    if (st.project !== lastProject) { lastProject = st.project; pads = null; } // another project: its pads
    if (st.screen === 'slicer') mode = 'slicer';
    const sl = st.screen === 'slicer';
    // the pattern, the unit's project and its patterns belong to the pattern tools only: the slicer and the pad tools show just what they work on
    const patMode = mode === 'pattern' && !sl;
    $('patview').hidden = !patMode; $('sliceview').hidden = !sl;
    for (const [id, m] of [['mPattern', 'pattern'], ['mSlicer', 'slicer'], ['mPad', 'pad']]) $(id).classList.toggle('on', mode === m);
    $('other').hidden = !st.other; if (st.other) $('otherText').textContent = 'The program has opened ' + st.other + '. It is made for the terminal; close it here.';
   
    if (sl) {
      const s = st.slicer; $('slTitle').textContent = 'Slicer · ' + s.label + ' ' + s.name; $('slOrder').textContent = s.playing ? '■ STOP' : '▶ PLAY ORDER'; $('slOrder').classList.toggle('on', s.playing); $('slUndo').disabled = !s.canUndo; $('slRedo').disabled = !s.canRedo;
      $('slInfo').textContent = s.slices.length + ' slices · cursor ' + (atNow() / s.rate).toFixed(3) + ' s · plays ' + s.mode;
      drawWave();
    }
    $('idle').hidden = !!st.menu ? true : false;
    renderGrid(p); renderMenu(st.menu); renderUnit(); renderStrip(p); renderChooser(); renderModal();
    for (const id of ['unitbar', 'ptnStrip']) $(id).hidden = !patMode;
    if (!patMode) $('ubWarn').hidden = true;
    // the right-hand panel holds the pattern tools button or the pad chooser: when it would be empty it is not there (a menu is a sheet over the page)
    $('right').hidden = $('chooser').hidden;
    $('left').hidden = !(patMode || sl || st.msg || st.liveErr || st.sending);
  }

  call({ op: 'state' });
  setInterval(() => { if (!busy && !document.hidden) call({ op: 'state' }); }, 1000);
})();
