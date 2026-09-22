// The analyzers' worker.
//
// Loads Go's wasm_exec.js runtime and then the meters build, and does nothing
// else: every message it handles is handled on the Go side (see
// pkg/metersworker). The only job here is that the Go instance is running
// before a message can arrive, which importScripts and the await below
// guarantee — messages that arrive first are queued by the browser until this
// script's initial evaluation finishes.
self.importScripts('wasm_exec.js');

(async function () {
  const go = new Go();
  const res = await WebAssembly.instantiateStreaming(fetch('meters.wasm'), go.importObject)
    .catch(async () => {
      // instantiateStreaming needs the right Content-Type; fall back for a
      // host that does not send it.
      const b = await (await fetch('meters.wasm')).arrayBuffer();
      return WebAssembly.instantiate(b, go.importObject);
    });
  go.run(res.instance);
})();
