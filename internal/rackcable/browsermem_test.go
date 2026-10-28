//go:build !js

package rackcable

import "testing"

// A trace as the browser hands it over: the frames and their processes in
// TracingStartedInBrowser, and a process's dump in more than one event, the
// totals (with a flag that is a JSON bool) apart from the allocators.
const testTrace = `{"traceEvents":[
 {"ph":"I","pid":1,"name":"TracingStartedInBrowser","args":{"data":{"frames":[
   {"frame":"AAA","processId":10},
   {"frame":"BBB","processId":20},
   {"frame":"CCC","parent":"BBB","processId":20},
   {"frame":"DDD","processId":20}]}}},
 {"ph":"v","pid":20,"name":"periodic_interval","args":{"dumps":{"process_totals":
   {"is_peak_rss_resettable":true,"peak_resident_set_size":"20000000","private_footprint_bytes":"16000000"}}}},
 {"ph":"v","pid":20,"name":"periodic_interval","args":{"dumps":{"allocators":{
   "blink_gc":{"attrs":{"size":{"type":"scalar","units":"bytes","value":"a000000"}}},
   "blink_gc/main":{"attrs":{"size":{"type":"scalar","units":"bytes","value":"9000000"}}},
   "v8":{"attrs":{"size":{"type":"scalar","units":"bytes","value":"1800000"}}},
   "tiny":{"attrs":{"size":{"type":"scalar","units":"bytes","value":"1000"}}}}}}},
 {"ph":"v","pid":10,"name":"periodic_interval","args":{"dumps":{"process_totals":
   {"private_footprint_bytes":"1000000"}}}}]}`

func TestParseDumpReadsTheTabsProcess(t *testing.T) {
	b, err := parseDump([]byte(testTrace), "bbb") // target ids are matched without case
	if err != nil {
		t.Fatal(err)
	}
	if b.Pid != 20 || b.Pages != 2 {
		t.Errorf("pid %d with %d pages, want 20 with 2 (a child frame is not a page)", b.Pid, b.Pages)
	}
	if b.Footprint != 0x16000000 || b.PeakRSS != 0x20000000 {
		t.Errorf("footprint %x, peak %x", b.Footprint, b.PeakRSS)
	}
	if len(b.Parts) != 2 || b.Parts[0].Name != "blink_gc" || b.Parts[0].N != 0xa000000 || b.Parts[1].Name != "v8" {
		t.Errorf("parts %+v, want blink_gc then v8: top level only, a megabyte or more, largest first", b.Parts)
	}
	if _, err := parseDump([]byte(testTrace), "ZZZ"); err == nil {
		t.Error("a page the trace does not list was read")
	}
}
