package interproc

import (
	"bytes"
	"os"
	"reflect"
	"runtime"
	"runtime/metrics"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

const (
	actorMemoryGateReferences = 4_000_000
	actorMemoryRSSBudget      = 512 << 20
	actorMemoryHeapBudget     = 128 << 20
	actorMemoryScanBudget     = 64 << 20
)

// Run the actor-scale gate with:
//
//	PERF_4M=1 go test ./analysis/check/fixpoint/interproc -run '^TestFourMillionActorMemoryGate$' -count=1 -v
//
// The budgets are absolute byte ceilings for the 4M run, not percentages of
// the test process's starting footprint. The scalar slab payload itself is
// 48,000,000 bytes (4M x 12-byte SummaryInstanceRef).
func TestFourMillionActorMemoryGate(t *testing.T) {
	if os.Getenv("PERF_4M") != "1" {
		t.Skip("4M actor memory gate disabled; run with PERF_4M=1")
	}
	if words := actorReferencePointerWords(); words != 0 {
		t.Fatalf("actor reference has %d GC pointer words, want zero", words)
	}
	if size := unsafe.Sizeof(SummaryInstanceRef{}); size != 12 {
		t.Fatalf("SummaryInstanceRef size = %d, want 12 scalar bytes", size)
	}

	artifactCanonical := []byte("actor-memory-gate/artifact-v1")
	projection := []byte("actor-memory-gate/equal-projection-v1")
	key := NewFastInstanceKey(artifactCanonical, projection)
	cache := NewRuntimeCache(1, 4)
	outcome, err := NewClosedOutcome([]byte("actor-memory-gate/closed-outcome-v1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Store(key, artifactCanonical, projection, outcome); err != nil {
		t.Fatal(err)
	}

	slab := NewSummaryInstanceSlab(actorMemoryGateReferences)
	started := time.Now()
	for index := 0; index < actorMemoryGateReferences; index++ {
		// The shared artifact/instance is equal for all actors. Generation
		// varies as a realistic lease/flag mix without adding a pointer edge.
		reference := SummaryInstanceRef{ArtifactHandle: 1, InstanceHandle: 1, Generation: uint32(index & 3)}
		if !slab.Retain(reference) {
			t.Fatalf("slab exhausted after %d references", index)
		}
	}
	retentionWall := time.Since(started)

	scanBefore := gcScanHeapBytes()
	gcStarted := time.Now()
	runtime.GC()
	gcWall := time.Since(gcStarted)
	scanAfter := gcScanHeapBytes()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	rss, rssErr := currentProcessRSSBytes()
	runtime.KeepAlive(slab)
	runtime.KeepAlive(cache)

	wantPayload := uint64(actorMemoryGateReferences) * uint64(unsafe.Sizeof(SummaryInstanceRef{}))
	if slab.Len() != actorMemoryGateReferences || slab.RequestedBytes() != wantPayload || slab.HighWaterBytes() != wantPayload {
		t.Fatalf("slab accounting len/requested/high-water = %d/%d/%d, want %d/%d/%d", slab.Len(), slab.RequestedBytes(), slab.HighWaterBytes(), actorMemoryGateReferences, wantPayload, wantPayload)
	}
	if metrics := cache.Metrics(); metrics.Publications != 1 {
		t.Fatalf("equal-projection cache publications = %d, want one shared instance", metrics.Publications)
	}
	if got, ok := cache.Load(key, artifactCanonical, projection); !ok || !bytes.Equal(got.canonical, outcome.canonical) {
		t.Fatal("shared cache instance was not retained")
	}
	if memory.HeapInuse > actorMemoryHeapBudget {
		t.Fatalf("heap-in-use = %d, budget = %d", memory.HeapInuse, actorMemoryHeapBudget)
	}
	if scanAfter > actorMemoryScanBudget {
		t.Fatalf("GC scan heap bytes = %d, budget = %d", scanAfter, actorMemoryScanBudget)
	}
	if rssErr == nil && rss > actorMemoryRSSBudget {
		t.Fatalf("RSS = %d, budget = %d", rss, actorMemoryRSSBudget)
	}
	t.Logf("ACTOR_MEMORY_GATE refs=%d bytes_per_actor=%d pointer_words=%d instances=1 slab_requested=%d slab_high_water=%d heap_inuse=%d rss=%d rss_err=%v gc_scan_before=%d gc_scan_after=%d gc_wall=%s retain_wall=%s", actorMemoryGateReferences, unsafe.Sizeof(SummaryInstanceRef{}), actorReferencePointerWords(), slab.RequestedBytes(), slab.HighWaterBytes(), memory.HeapInuse, rss, rssErr, scanBefore, scanAfter, gcWall.Round(time.Millisecond), retentionWall.Round(time.Millisecond))
}

func actorReferencePointerWords() int {
	// SummaryInstanceRef is intentionally flat. Treat every GC-bearing field
	// kind as a pointer word so a future convenience pointer/slice/map/string
	// cannot silently enter the actor fanout record.
	typeOfReference := reflect.TypeOf(SummaryInstanceRef{})
	words := 0
	for index := 0; index < typeOfReference.NumField(); index++ {
		switch typeOfReference.Field(index).Type.Kind() {
		case reflect.Pointer, reflect.UnsafePointer, reflect.Map, reflect.Func, reflect.Chan, reflect.Interface:
			words++
		case reflect.Slice, reflect.String:
			words += 2
		}
	}
	return words
}

func gcScanHeapBytes() uint64 {
	samples := []metrics.Sample{{Name: "/gc/scan/heap:bytes"}}
	metrics.Read(samples)
	return samples[0].Value.Uint64()
}

func currentProcessRSSBytes() (uint64, error) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	fields := bytes.Fields(data)
	if len(fields) < 2 {
		return 0, strconv.ErrSyntax
	}
	pages, err := strconv.ParseUint(string(fields[1]), 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * uint64(os.Getpagesize()), nil
}
