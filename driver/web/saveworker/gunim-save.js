// gunim's save worker. It writes a file a gunim program in a browser saves
// to the disk as the bytes come, as a download: the page hands it the bytes
// one chunk at a time, each when the download asks for more, and nothing
// keeps the whole file. See github.com/marrasen/gunim/driver/web/saveworker.
"use strict";

// saves are the files under way, by their id: the page's port, the file's
// name, the download's wait for its next chunk, and what came before the
// download asked for it.
const saves = new Map();

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", e => e.waitUntil(self.clients.claim()));

self.addEventListener("message", e => {
  const m = e.data;
  if (!m || typeof m.gunimSave != "string" || !/^[0-9a-f]{32}$/.test(m.gunimSave)) {
    return;
  }
  let s = saves.get(m.gunimSave);
  if (typeof m.name == "string" && e.ports[0]) {
    if (s) {
      return;
    }
    saves.set(m.gunimSave, {port: e.ports[0], name: m.name, pull: null, early: null});
    e.ports[0].postMessage({ready: true});
    return;
  }
  if (!s || m.ping) {
    return;
  }
  if (m.abort && !s.stream) {
    // The download never started: nothing waits for the file.
    saves.delete(m.gunimSave);
    return;
  }
  if (s.pull) {
    const answer = s.pull;
    s.pull = null;
    answer(m);
  } else {
    s.early = m;
  }
});

// asciiName is name as the old filename parameter takes it.
const asciiName = name => name.replace(/[^\x20-\x7e]|["\\]/g, "_");

// encodedName is name as the filename* parameter takes it.
const encodedName = name => encodeURIComponent(name).replace(/['()*]/g, c => "%" + c.charCodeAt(0).toString(16).toUpperCase());

self.addEventListener("fetch", e => {
  const url = e.request.url;
  if (!url.startsWith(self.registration.scope)) {
    return;
  }
  const id = /^([0-9a-f]{32})\//.exec(url.slice(self.registration.scope.length));
  const s = id && saves.get(id[1]);
  if (!s || s.stream) {
    return;
  }
  s.stream = true;
  const port = s.port;
  // Each pull asks the page for one chunk and waits for it: the page writes
  // no faster than the download takes the bytes.
  const next = () => new Promise(answer => {
    if (s.early) {
      const m = s.early;
      s.early = null;
      answer(m);
      return;
    }
    s.pull = answer;
    port.postMessage({pull: true});
  });
  const body = new ReadableStream({
    start() {
      port.postMessage({started: true});
    },
    pull(c) {
      return next().then(m => {
        if (m.chunk) {
          c.enqueue(new Uint8Array(m.chunk));
        } else if (m.end) {
          saves.delete(id[1]);
          c.close();
        } else {
          saves.delete(id[1]);
          c.error(new Error(m.abort || "the program gave up the file"));
        }
      });
    },
    cancel() {
      saves.delete(id[1]);
      port.postMessage({cancelled: true});
    },
  }, {highWaterMark: 1});
  e.respondWith(new Response(body, {headers: {
    "Content-Type": "application/octet-stream",
    "Content-Disposition": "attachment; filename=\"" + asciiName(s.name) + "\"; filename*=UTF-8''" + encodedName(s.name),
    "X-Content-Type-Options": "nosniff",
    "Cache-Control": "no-store",
  }}));
});
