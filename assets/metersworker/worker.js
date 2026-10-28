// The analyzers' worker.
//
// Loads Go's wasm_exec.js runtime and then the meters build, and does nothing
// else: every message it handles is handled on the Go side (see
// pkg/metersworker). The only job here is that the Go instance is running
// before a message can arrive, which importScripts and the await below
// guarantee — messages that arrive first are queued by the browser until this
// script's initial evaluation finishes.
// The runtime is the Go build's, from assets/gowasm: meters.wasm is built by
// the same toolchain, and that copy is a file in the repository, where one
// beside this script was only ever a route of the local server and 404'd on
// GitHub Pages. Both by the build the worker was started for (location.search,
// ?v= and the page's build stamp), so a cache cannot hand it an older one.
const v = self.location.search;
self.importScripts('../gowasm/wasm_exec.js' + v);

(async function () {
  const go = new Go();
  const res = await WebAssembly.instantiateStreaming(fetch('meters.wasm' + v), go.importObject)
    .catch(async () => {
      // instantiateStreaming needs the right Content-Type; fall back for a
      // host that does not send it.
      const b = await (await fetch('meters.wasm' + v)).arrayBuffer();
      return WebAssembly.instantiate(b, go.importObject);
    });
  go.run(res.instance);
})();
