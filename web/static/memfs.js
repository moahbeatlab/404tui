// A file system in memory for Go's WebAssembly runtime (what wasm_exec.js leaves out): the tools of the demo (demo.wasm) keep a copy of
// the unit's project in it. It has to be loaded before wasm_exec.js: Go reads fs.constants when the program starts.
(function () {
  const S_IFDIR = 0o40000, S_IFREG = 0o100000;
  const C = { O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 64, O_EXCL: 128, O_TRUNC: 512, O_APPEND: 1024, O_DIRECTORY: 65536 };
  const dec = new TextDecoder('utf-8');
  const E = (code) => { const e = new Error(code); e.code = code; return e; };
  let ino = 1;
  const mk = (dir) => ({ dir, kids: dir ? new Map() : null, data: dir ? null : new Uint8Array(0), mode: dir ? 0o755 : 0o644, mtime: Date.now(), ino: ino++ });
  const root = mk(true);
  const fds = new Map(); let nextFd = 3;
  let out = '';

  function resolve(...segs) {
    let parts = [];
    for (const s of segs) {
      if (!s) continue;
      if (s.startsWith('/')) parts = [];
      for (const p of s.split('/')) { if (p === '' || p === '.') continue; if (p === '..') parts.pop(); else parts.push(p); }
    }
    return '/' + parts.join('/');
  }
  const split = (p) => resolve(p).split('/').filter(Boolean);
  function lookup(p) {
    let n = root;
    for (const part of split(p)) { if (!n.dir || !n.kids.has(part)) return null; n = n.kids.get(part); }
    return n;
  }
  function parentOf(p) {
    const parts = split(p); const name = parts.pop();
    let n = root;
    for (const part of parts) { if (!n.dir || !n.kids.has(part)) return { err: 'ENOENT' }; n = n.kids.get(part); }
    if (!n.dir) return { err: 'ENOTDIR' };
    return { dir: n, name };
  }
  function stat(n) {
    const size = n.dir ? 0 : n.data.length;
    return { dev: 1, ino: n.ino, mode: (n.dir ? S_IFDIR : S_IFREG) | (n.mode & 0o777), nlink: 1, uid: 0, gid: 0, rdev: 0, size, blksize: 4096, blocks: Math.ceil(size / 512),
      atimeMs: n.mtime, mtimeMs: n.mtime, ctimeMs: n.mtime, isDirectory() { return n.dir; }, isFile() { return !n.dir; } };
  }
  const fileOf = (fd) => fds.get(fd);
  const grow = (n, size) => { if (n.data.length < size) { const d = new Uint8Array(Math.max(size, n.data.length * 2)); d.set(n.data); n.data = d; n.len = size; } };
  // n.data may be longer than the file: n.len is the size
  const len = (n) => (n.len === undefined ? n.data.length : n.len);
  const sized = (n) => { n.data = n.data.subarray(0, len(n)); delete n.len; return n.data; };

  const fs = {
    constants: C,
    writeSync(fd, buf) {
      if (fd === 1 || fd === 2) { out += dec.decode(buf); const nl = out.lastIndexOf('\n'); if (nl !== -1) { console.log(out.substring(0, nl)); out = out.substring(nl + 1); } return buf.length; }
      const f = fileOf(fd); if (!f) throw E('EBADF');
      return this._write(f, buf, 0, buf.length, null);
    },
    _write(f, buf, off, length, pos) {
      const n = f.node; if (n.dir) throw E('EISDIR');
      if (f.append) f.pos = len(n);
      const at = pos === null || pos === undefined ? f.pos : pos;
      if (at + length > n.data.length) { const d = new Uint8Array(Math.max(at + length, n.data.length * 2)); d.set(n.data.subarray(0, len(n))); n.data = d; }
      n.data.set(buf.subarray(off, off + length), at);
      n.len = Math.max(len(n), at + length); n.mtime = Date.now();
      if (pos === null || pos === undefined) f.pos = at + length;
      return length;
    },
    write(fd, buf, off, length, pos, cb) { try { cb(null, fd === 1 || fd === 2 ? this.writeSync(fd, buf.subarray(off, off + length)) : this._write(fileOf(fd) || (() => { throw E('EBADF'); })(), buf, off, length, pos)); } catch (e) { cb(e); } },
    read(fd, buf, off, length, pos, cb) {
      try {
        const f = fileOf(fd); if (!f) throw E('EBADF'); const n = f.node; if (n.dir) throw E('EISDIR');
        const at = pos === null || pos === undefined ? f.pos : pos, avail = Math.max(0, len(n) - at), k = Math.min(length, avail);
        buf.set(n.data.subarray(at, at + k), off);
        if (pos === null || pos === undefined) f.pos = at + k;
        cb(null, k);
      } catch (e) { cb(e); }
    },
    open(path, flags, mode, cb) {
      try {
        let n = lookup(path);
        if (n && (flags & C.O_CREAT) && (flags & C.O_EXCL)) throw E('EEXIST');
        if (!n) {
          if (!(flags & C.O_CREAT)) throw E('ENOENT');
          const p = parentOf(path); if (p.err) throw E(p.err);
          n = mk(false); n.mode = mode & 0o777 || 0o644; p.dir.kids.set(p.name, n);
        } else if ((flags & C.O_TRUNC) && !n.dir) { n.data = new Uint8Array(0); delete n.len; n.mtime = Date.now(); }
        const fd = nextFd++; fds.set(fd, { node: n, pos: 0, append: !!(flags & C.O_APPEND) });
        cb(null, fd);
      } catch (e) { cb(e); }
    },
    close(fd, cb) { cb(fds.delete(fd) ? null : E('EBADF')); },
    fsync(fd, cb) { cb(null); },
    fstat(fd, cb) { const f = fileOf(fd); cb(f ? null : E('EBADF'), f && stat(f.node)); },
    stat(path, cb) { const n = lookup(path); cb(n ? null : E('ENOENT'), n && stat(n)); },
    lstat(path, cb) { this.stat(path, cb); },
    readdir(path, cb) { const n = lookup(path); if (!n) cb(E('ENOENT')); else if (!n.dir) cb(E('ENOTDIR')); else cb(null, [...n.kids.keys()].sort()); },
    mkdir(path, mode, cb) {
      const p = parentOf(path); if (p.err) return cb(E(p.err));
      if (!p.name) return cb(E('EEXIST'));
      if (p.dir.kids.has(p.name)) return cb(E('EEXIST'));
      p.dir.kids.set(p.name, mk(true)); cb(null);
    },
    unlink(path, cb) { const p = parentOf(path); const n = !p.err && p.dir.kids.get(p.name); if (!n) cb(E('ENOENT')); else if (n.dir) cb(E('EISDIR')); else { p.dir.kids.delete(p.name); cb(null); } },
    rmdir(path, cb) { const p = parentOf(path); const n = !p.err && p.dir.kids.get(p.name); if (!n) cb(E('ENOENT')); else if (!n.dir) cb(E('ENOTDIR')); else if (n.kids.size) cb(E('ENOTEMPTY')); else { p.dir.kids.delete(p.name); cb(null); } },
    rename(from, to, cb) {
      const a = parentOf(from), n = !a.err && a.dir.kids.get(a.name); if (!n) return cb(E('ENOENT'));
      const b = parentOf(to); if (b.err) return cb(E(b.err));
      a.dir.kids.delete(a.name); b.dir.kids.set(b.name, n); cb(null);
    },
    truncate(path, length, cb) { const n = lookup(path); if (!n) return cb(E('ENOENT')); const d = new Uint8Array(length); d.set(sized(n).subarray(0, length)); n.data = d; cb(null); },
    ftruncate(fd, length, cb) { const f = fileOf(fd); if (!f) return cb(E('EBADF')); const n = f.node; const d = new Uint8Array(length); d.set(sized(n).subarray(0, length)); n.data = d; cb(null); },
    chmod(path, mode, cb) { const n = lookup(path); if (n) n.mode = mode; cb(n ? null : E('ENOENT')); },
    fchmod(fd, mode, cb) { cb(null); },
    chown(path, u, g, cb) { cb(null); }, fchown(fd, u, g, cb) { cb(null); }, lchown(path, u, g, cb) { cb(null); },
    utimes(path, a, m, cb) { const n = lookup(path); if (n) n.mtime = m * 1000; cb(n ? null : E('ENOENT')); },
    readlink(path, cb) { cb(E('EINVAL')); }, symlink(a, b, cb) { cb(E('ENOSYS')); }, link(a, b, cb) { cb(E('ENOSYS')); },
  };
  globalThis.fs = fs;
  globalThis.path = { resolve: (...s) => resolve('/', ...s) };
  if (!globalThis.process) {
    globalThis.process = {
      getuid() { return 1000; }, getgid() { return 1000; }, geteuid() { return 1000; }, getegid() { return 1000; }, getgroups() { return [1000]; },
      pid: 1, ppid: 0, umask() { return 0o22; }, cwd() { return '/'; }, chdir() {},
    };
  }
})();
