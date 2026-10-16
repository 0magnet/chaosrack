// The page's end of pairing (tab_js.go): storage, the address bar, and the
// hello session, in JS because each can throw, and a throw reaching Go is a
// panic that TinyGo cannot recover from.
(function () {
  if (window.__crPair) return;
  function hashes(list) {
    return list.map(function (b64) {
      var s = atob(b64.replace(/=+$/, "")), u = new Uint8Array(s.length);
      for (var i = 0; i < s.length; i++) u[i] = s.charCodeAt(i);
      return { algorithm: "sha-256", value: u };
    });
  }
  window.__crPair = {
    get: function (k) { try { return localStorage.getItem(k) || ""; } catch (e) { return ""; } },
    set: function (k, v) { try { localStorage.setItem(k, v); } catch (e) { /* storage refused: paired for this load only */ } },
    // strip takes the pair link out of the address, so it is not left in
    // the bar, a bookmark, or a screenshot.
    strip: function () {
      try { history.replaceState(history.state, "", location.pathname + location.search); } catch (e) { /* kept */ }
    },
    hashes: hashes,
    // hello opens the paired server's hello session and resolves with the
    // text of the one stream the server opens on it.
    hello: function (url, certs) {
      var t;
      try {
        t = new WebTransport(url, { serverCertificateHashes: hashes(certs) });
      } catch (e) {
        return Promise.reject(e);
      }
      function done() { try { t.close(); } catch (e) { /* closed */ } }
      return t.ready.then(function () {
        return t.incomingUnidirectionalStreams.getReader().read();
      }).then(function (x) {
        if (x.done) throw new Error("the server sent no hello");
        var rd = x.value.getReader(), dec = new TextDecoder(), s = "";
        function more() {
          return rd.read().then(function (y) {
            if (y.done) return s;
            s += dec.decode(y.value, { stream: true });
            return more();
          });
        }
        return more();
      }).then(function (s) { done(); return s; }, function (e) { done(); throw e; });
    }
  };
})();
