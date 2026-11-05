'use strict';
// 404tui tools in the page: the program's own tools (the pattern's tools and settings) as the terminal has them, with every page, setting and value.
// The state comes from /tools/api (one JSON call for each action, the whole state back); nothing is kept here but what is on the screen.
(() => {
  const $ = (id) => document.getElementById(id);
  const qs = new URLSearchParams(location.search);
  const store = { get(k) { try { return localStorage.getItem(k) || ''; } catch (e) { return ''; } } };
  const key = qs.get('key') || store.get('key');
  $('back').href = './' + (key ? '?key=' + encodeURIComponent(key) : '');
  let st = null, busy = false, editing = 0;

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
  $('bank').onchange = $('pat').onchange = () => act({ op: 'pattern', slot: slot() });
  $('play').onclick = () => act({ op: 'play', on: !(st && st.playing) });
  $('undo').onclick = () => act({ op: 'undo' });
  $('redo').onclick = () => act({ op: 'redo' });
  $('open').onclick = () => act({ op: 'open' });
  $('mback').onclick = () => act({ op: 'back' });

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
    $('idle').hidden = !!m; $('menu').hidden = !m;
    if (!m) return;
    $('mtitle').textContent = m.title; $('mback').textContent = m.depth > 1 ? '◂ BACK' : '✕ CLOSE';
    const tabs = $('tabs'); tabs.textContent = '';
    (m.tabs || []).forEach((name, i) => { const b = document.createElement('button'); b.className = 'btn' + (i === m.tab ? ' on' : ''); b.textContent = name; b.onclick = () => act({ op: 'tab', tab: i }); tabs.appendChild(b); });
    const box = $('items'); if (editing && document.activeElement && box.contains(document.activeElement)) return; // do not replace a control while it is in the hand
    box.textContent = '';
    for (const it of m.items) {
      const d = document.createElement('div');
      if (it.kind === 'head') { d.className = 'item head'; d.textContent = it.label; }
      else if (it.kind === 'choice') {
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
        ctl.append(prev, sel, next); d.append(lab, ctl);
      } else {
        d.className = 'item action';
        const b = document.createElement('button'); b.className = 'btn' + (/^keep it/i.test(it.label) ? ' go' : ''); b.textContent = it.label; b.onclick = () => act({ op: 'run', i: it.i }); d.appendChild(b);
      }
      box.appendChild(d);
    }
  }

  function renderProjects() {
    const list = $('projList'); list.textContent = '';
    const names = st.projects || [];
    $('projSum').textContent = names.length ? '(' + names.length + ')' : '(no unit)';
    names.forEach((n, i) => {
      const r = document.createElement('div'); r.className = 'proj';
      const s = document.createElement('span'); s.textContent = (i + 1) + '  ' + (n || '(empty)');
      const copy = document.createElement('button'); copy.className = 'btn'; copy.textContent = 'OPEN A COPY'; copy.title = 'Copies the project from the unit into the editor; the unit is not changed'; copy.onclick = () => act({ op: 'load', i, live: false });
      const live = document.createElement('button'); live.className = 'btn' + (st.live === i + 1 ? ' on' : ''); live.textContent = st.live === i + 1 ? 'LIVE' : 'EDIT LIVE'; live.title = 'Every change you keep goes to the unit'; live.onclick = () => act({ op: 'load', i, live: true });
      r.append(s, copy, live); list.appendChild(r);
    });
  }

  function render() {
    const p = st.pattern;
    $('bank').value = Math.floor((p.slot - 1) / 16); $('pat').value = (p.slot - 1) % 16;
    $('patinfo').textContent = 'Pattern ' + p.label + (p.exists ? ' · ' + p.bars + ' bars · ' + Math.round(p.tempo) + ' BPM' : ' · empty');
    $('play').textContent = st.playing ? '■ STOP' : '▶ PLAY HERE'; $('play').classList.toggle('on', !!st.playing);
    const u = $('unit'); u.className = 'unit' + (st.live ? ' live' : st.unit ? ' on' : '');
    $('unitText').textContent = st.live ? 'live: unit project ' + st.live : st.unit ? 'unit: ' + st.unit : 'no unit';
    let line = st.msg || '';
    if (st.job && !st.job.ended) line = st.job.what + (st.job.total ? ' ' + st.job.done + '/' + st.job.total : '') + ' ' + st.job.name;
    $('status').textContent = line; $('status').className = 'status' + (st.msgBad || (st.job && st.job.error) ? ' bad' : '');
    renderGrid(p); renderMenu(st.menu); renderProjects();
  }

  call({ op: 'state' });
  setInterval(() => { if (!busy && !document.hidden) call({ op: 'state' }); }, 1000);
})();
