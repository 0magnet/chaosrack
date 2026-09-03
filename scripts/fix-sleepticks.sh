#!/bin/sh
# Collapse TinyGo's scheduler wakeups onto a single pending timer.
#
# The shipped shim arms a fresh setTimeout on every sleepTicks call and never
# cancels the previous one, and the callback calls go_scheduler(), which calls
# sleepTicks again — so every armed timer replaces itself forever. Each JS->Go
# callback that reaches the scheduler starts one more of these self-sustaining
# chains, and they accumulate: a bare globe climbs past 1000 setTimeout/sec in
# twelve seconds, and chaosrack sits at ~50000/sec.
#
# The scheduler only ever needs ONE pending wakeup: the earliest deadline. Keep
# that, and drop a request that is no sooner than what is already armed.
f="$1"
[ -f "$f" ] || { echo "no such file: $f" >&2; exit 1; }
grep -q '__tgSleepTimer' "$f" && { echo "already patched: $f"; exit 0; }
perl -0777 -i -pe 's{\t*"runtime\.sleepTicks": \(timeout\) => \{\n.*?\n\t*\},\n}{\t\t\t\t\t"runtime.sleepTicks": (timeout) => {
						// Reactivate the scheduler after the given timeout, keeping
						// exactly one pending wakeup. Arming a second one without
						// cancelling the first leaks a self-sustaining timer chain,
						// because the callback schedules its own replacement.
						const ms = Number(timeout)/1e6;
						const due = Date.now() + ms;
						if (this.__tgSleepTimer !== undefined && this.__tgSleepDue <= due) return;
						if (this.__tgSleepTimer !== undefined) clearTimeout(this.__tgSleepTimer);
						this.__tgSleepDue = due;
						this.__tgSleepTimer = setTimeout(() => {
							this.__tgSleepTimer = undefined;
							if (this.exited) return;
							try {
								this._inst.exports.go_scheduler();
							} catch (e) {
								if (e !== wasmExit) throw e;
							}
						}, ms);
					},
}s' "$f"
grep -q '__tgSleepTimer' "$f" && echo "patched: $f" || { echo "PATCH FAILED: $f" >&2; exit 1; }
