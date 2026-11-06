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
  let st = null, busy = false, editing = 0, mode = 'pattern', pads = null, cbank = 0, win = null, armed = -1, armTimer = 0;

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

  // the three kinds of tools: the pattern's, the slicer's, a pad's
  const setMode = (m) => {
    mode = m;
    if (m !== 'slicer' && st && st.screen === 'slicer') return act({ op: 'slicer-close' });
    if (m !== 'pattern' && st && st.menu && !st.slicer) return act({ op: 'escape' });
    render();
  };
  $('mPattern').onclick = () => setMode('pattern'); $('mSlicer').onclick = () => setMode('slicer'); $('mPad').onclick = () => setMode('pad');
  $('otherClose').onclick = () => act({ op: 'escape' });
  for (let i = 0; i < 10; i++) { const o = document.createElement('option'); o.value = i; o.textContent = bankNames[i]; $('cbank').appendChild(o); }
  $('cbank').onchange = () => { cbank = +$('cbank').value; renderChooser(); };
  const loadPads = () => fetch('/tools/api' + (key ? '?key=' + encodeURIComponent(key) : ''), { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ op: 'pads' }) }).then((r) => r.json()).then((p) => { pads = p; renderChooser(); }).catch(() => {});

  // the pad chooser: a bank of 16, a pad with a sample for the slicer, any pad for the tools
  function renderChooser() {
    const box = $('chooser'); const show = (mode === 'slicer' && !(st && st.slicer)) || (mode === 'pad' && !(st && st.menu));
    box.hidden = !show; if (!show) return;
    if (!pads) { loadPads(); return; }
    $('cbank').value = cbank;
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
    const cx = X(s.at); g.strokeStyle = text; g.lineWidth = 2; g.beginPath(); g.moveTo(cx, 0); g.lineTo(cx, H); g.stroke();
  }
  const frameAt = (ev) => { const s = st.slicer, r = wave.getBoundingClientRect(); return Math.round(s.from + Math.min(Math.max((ev.clientX - r.left) / r.width, 0), 1) * (s.to - s.from)); };
  wave.onclick = (ev) => { if (st && st.slicer) act({ op: 'slicer-at', i: frameAt(ev) }); };
  const cur = () => st.slicer.at;
  $('slCut').onclick = () => act({ op: 'slicer-cut', i: cur() });
  $('slJoin').onclick = () => act({ op: 'slicer-join', i: cur() });
  $('slPlay').onclick = () => act({ op: 'slicer-play', i: cur() });
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

  function renderGrid(p) {
    const g = $('grid'); g.textContent = '';
    if (!p.exists || !p.tracks.length) { const d = document.createElement('div'); d.className = 'empty'; d.textContent = p.exists ? 'The pattern has no notes.' : 'No pattern in ' + p.label + ' yet. A tool writes one.'; g.appendChild(d); return; }
    for (const t of p.tracks) {
      const row = document.createElement('div'); row.className = 'track' + (t.pad === st.pad ? ' sel' : '');
      const l = document.createElement('b'); l.textContent = t.label; l.title = 'Put this track in front: the tools act on it'; l.onclick = () => act({ op: 'track', pad: t.pad });
      const steps = document.createElement('div'); steps.className = 'steps'; steps.style.gridTemplateColumns = 'repeat(' + p.steps + ', minmax(6px, 1fr))';
      const on = new Map(); for (const n of t.notes) on.set(n.step, n);
      for (let s = 0; s < p.steps; s++) {
        const c = document.createElement('div'); c.className = 'step' + (s % 16 === 0 && s ? ' bar' : s % 4 === 0 && s ? ' b4' : '');
        const n = on.get(s); if (n) { c.classList.add('on'); c.style.opacity = String(0.35 + 0.65 * Math.min(n.vel, 127) / 127); c.title = 'step ' + (s + 1) + ', velocity ' + n.vel + ', length ' + n.len + (n.pitch ? ', pitch ' + n.pitch : ''); }
        steps.appendChild(c);
      }
      row.append(l, steps); g.appendChild(row);
    }
  }

  function renderMenu(m) {
    const wasOpen = !$('menu').hidden;
    $('idle').hidden = !!m; $('menu').hidden = !m;
    if (!m) return;
    if (!wasOpen) $('right').scrollIntoView({ block: 'start' }); // on a phone the menu is under the pattern: bring it up
    $('mtitle').textContent = m.title; $('mback').textContent = m.depth > 1 ? '◂ BACK' : '✕ CLOSE';
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
    if (st.screen === 'slicer') mode = 'slicer';
    const sl = st.screen === 'slicer';
    $('patview').hidden = sl; $('sliceview').hidden = !sl;
    for (const [id, m] of [['mPattern', 'pattern'], ['mSlicer', 'slicer'], ['mPad', 'pad']]) $(id).classList.toggle('on', mode === m);
    $('other').hidden = !st.other; if (st.other) $('otherText').textContent = 'The program has opened ' + st.other + '. It is made for the terminal; close it here.';
    $('idPattern').hidden = mode !== 'pattern'; $('idSlicer').hidden = !(mode === 'slicer' && sl);
    if (sl) {
      const s = st.slicer; $('slTitle').textContent = 'Slicer · ' + s.label + ' ' + s.name; $('slUndo').disabled = !s.canUndo; $('slRedo').disabled = !s.canRedo;
      $('slInfo').textContent = s.slices.length + ' slices · cursor ' + (s.at / s.rate).toFixed(3) + ' s · plays ' + s.mode;
      drawWave();
    }
    $('idle').hidden = !!st.menu ? true : false;
    renderGrid(p); renderMenu(st.menu); renderUnit(); renderStrip(p); renderChooser(); renderModal();
  }

  call({ op: 'state' });
  setInterval(() => { if (!busy && !document.hidden) call({ op: 'state' }); }, 1000);
})();
