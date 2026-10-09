// The page's end of the rack link (racklink.go): connect to the server this
// page came from, and answer what a terminal asks, one line for one line.
//
// Evaluated in the page; defines window.__racklink. start(answer) takes the
// page's answering function — Go's, which turns a request line into an
// answer line — and keeps a connection up for as long as the page is open:
// WebTransport when the browser and the server both have it, a WebSocket
// otherwise, and again a few seconds after either drops (a server restarted
// under the page). A page served by something that is not a chaosrack
// server, or one run without the link, finds no /rack/info and does nothing.
(function () {
  if (window.__racklink) return;
  var L = { via: "", state: "off", why: "" };
  window.__racklink = L;

  var RETRY = 3000;
  var waiting = false;
  function retry() {
    if (waiting) return;
    waiting = true;
    L.state = "off";
    setTimeout(function () { waiting = false; connect(); }, RETRY);
  }

  L.start = function (answer) {
    L.answer = answer;
    connect();
  };

  function connect() {
    fetch("/rack/info", { cache: "no-store" }).then(function (r) {
      if (!r.ok) { L.state = "none"; L.why = "no rack link on this server"; return null; }
      return r.json();
    }).then(function (info) {
      if (!info) return;
      if (info.wt && info.certHash && window.WebTransport && window.isSecureContext !== false) {
        overWebTransport(info).then(retry, function (e) {
          // Never got going: the browser or the network will not have it.
          L.why = "WebTransport: " + e;
          overWebSocket();
        });
      } else {
        overWebSocket();
      }
    }, retry);
  }

  function hash(b64) {
    var s = atob(b64), u = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) u[i] = s.charCodeAt(i);
    return u;
  }

  // overWebTransport resolves when an established session ends, and rejects
  // when one could not be established at all.
  function overWebTransport(info) {
    var t = new WebTransport(info.wt + "/rack/page", {
      serverCertificateHashes: [{ algorithm: "sha-256", value: hash(info.certHash) }]
    });
    return t.ready.then(function () {
      return t.createBidirectionalStream();
    }).then(function (s) {
      L.via = "wt"; L.state = "on";
      var w = s.writable.getWriter(), r = s.readable.getReader();
      var enc = new TextEncoder(), dec = new TextDecoder(), buf = "";
      function pump() {
        return r.read().then(function (x) {
          if (x.done) return;
          buf += dec.decode(x.value, { stream: true });
          for (var n; (n = buf.indexOf("\n")) >= 0;) {
            var line = buf.slice(0, n);
            buf = buf.slice(n + 1);
            w.write(enc.encode(L.answer(line) + "\n"));
          }
          return pump();
        });
      }
      return pump().catch(function () {}).then(function () {
        try { t.close(); } catch (e) { /* already closed */ }
      });
    });
  }

  function overWebSocket() {
    var ws = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/rack/page/ws");
    ws.onopen = function () { L.via = "ws"; L.state = "on"; };
    ws.onmessage = function (e) { ws.send(L.answer(e.data)); };
    ws.onclose = retry;
  }
})();
