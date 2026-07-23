package interproc

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func TestFastInstanceKeyIsFixed64Bytes(t *testing.T) {
	if size := unsafe.Sizeof(FastInstanceKey{}); size != 64 {
		t.Fatalf("FastInstanceKey size = %d, want 64", size)
	}
}

func TestScratchProjectionEncoderMatchesReferenceProjection(t *testing.T) {
	artifact := demandedBodyArtifactFixture(t)
	entry := tableEntry(t, "value", "guard", "diagnostic", "unread")
	want, err := artifact.ReadCertificate().Project(entry)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := want.CanonicalBytes()
	encoder, err := NewScratchProjectionEncoder(artifact)
	if err != nil {
		t.Fatal(err)
	}
	scratch := NewEvaluatorScratch(len(wantBytes), 2)
	key, got, err := encoder.Encode(scratch, entry)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Fatalf("scratch projection differs from reference\n got %x\nwant %x", got, wantBytes)
	}
	if key.ArtifactID != artifact.ContentID() || key.ProjectionID != want.ContentID() {
		t.Fatalf("key = %+v, want artifact/projection IDs", key)
	}
	if !scratch.Push() || !scratch.Pop() {
		t.Fatal("preallocated scratch frame was not usable")
	}
	if scratch.OverflowCount() != 0 {
		t.Fatalf("overflow count = %d, want zero", scratch.OverflowCount())
	}
}

func TestScratchProjectionEncoderMetersOverflowAndFailsClosed(t *testing.T) {
	artifact := demandedBodyArtifactFixture(t)
	encoder, err := NewScratchProjectionEncoder(artifact)
	if err != nil {
		t.Fatal(err)
	}
	entry := tableEntry(t, "value", "guard", "diagnostic", "unread")
	if _, _, err := encoder.Encode(NewEvaluatorScratch(1, 0), entry); err != ErrProjectionScratchOverflow {
		t.Fatalf("overflow error = %v, want %v", err, ErrProjectionScratchOverflow)
	}
	missing, err := NewEntryBinding([]EntryValue{{Selector: "guard", Encoding: []byte("guard")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := encoder.Encode(NewEvaluatorScratch(512, 0), missing); err == nil || !IsIncompleteReadCertificateError(err) {
		t.Fatalf("missing certified value error = %v, want incomplete certificate", err)
	}
}

func TestRuntimeCacheConfirmsCanonicalWitnessesForPrimaryKeyCollision(t *testing.T) {
	cache := NewRuntimeCache(1, 4)
	key := FastInstanceKey{ArtifactID: ContentID{1}, ProjectionID: ContentID{2}}
	leftArtifact, leftProjection := []byte("artifact-left"), []byte("projection-left")
	rightArtifact, rightProjection := []byte("artifact-right"), []byte("projection-right")
	if err := cache.storeCold(key, leftArtifact, leftProjection, []byte("outcome-left")); err != nil {
		t.Fatal(err)
	}
	if err := cache.storeCold(key, rightArtifact, rightProjection, []byte("outcome-right")); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		artifact, projection, want []byte
	}{{leftArtifact, leftProjection, []byte("outcome-left")}, {rightArtifact, rightProjection, []byte("outcome-right")}} {
		got, ok := cache.Load(key, test.artifact, test.projection)
		if !ok || !bytes.Equal(got.canonical, test.want) {
			t.Fatalf("collision lookup = %q, %v; want %q, true", got.canonical, ok, test.want)
		}
	}
	if _, ok := cache.Load(key, leftArtifact, rightProjection); ok {
		t.Fatal("mixed collision witnesses were accepted")
	}
	if !cache.Invalidate(key, leftArtifact, leftProjection) {
		t.Fatal("exact collision member was not invalidated")
	}
	if _, ok := cache.Load(key, leftArtifact, leftProjection); ok {
		t.Fatal("invalidated collision member remained reusable")
	}
	if got, ok := cache.Load(key, rightArtifact, rightProjection); !ok || !bytes.Equal(got.canonical, []byte("outcome-right")) {
		t.Fatalf("peer collision member was evicted: %q, %v", got.canonical, ok)
	}
}

func TestRuntimeCacheSingleFlightAtOneTenAndOneHundred(t *testing.T) {
	artifact := demandedBodyArtifactFixture(t)
	encoder, err := NewScratchProjectionEncoder(artifact)
	if err != nil {
		t.Fatal(err)
	}
	entry := tableEntry(t, "same", "true", "diagnostic", "unread")
	seedScratch := NewEvaluatorScratch(512, 1)
	key, projection, err := encoder.Encode(seedScratch, entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, fanout := range []int{1, 10, 100} {
		t.Run(fmt.Sprintf("fanout-%d", fanout), func(t *testing.T) {
			cache := NewRuntimeCache(4, 4)
			var executions atomic.Int32
			ready := make(chan struct{})
			var callers sync.WaitGroup
			callers.Add(fanout)
			for i := 0; i < fanout; i++ {
				go func() {
					defer callers.Done()
					<-ready
					outcome, err := cache.LoadOrCompute(key, encoder.artifactCanonical, projection, func() (ClosedOutcome, error) {
						executions.Add(1)
						return NewClosedOutcome([]byte("closed-outcome"))
					})
					if err != nil || !outcome.Valid() {
						t.Errorf("resolve = %v, %v", outcome, err)
					}
				}()
			}
			close(ready)
			callers.Wait()
			if executions.Load() != 1 {
				t.Fatalf("executions = %d, want one", executions.Load())
			}
			if metrics := cache.Metrics(); metrics.Publications != 1 {
				t.Fatalf("metrics = %+v, want one publication", metrics)
			}
		})
	}
}

func TestRuntimeCacheSingleFlightSharesOwnerFailure(t *testing.T) {
	cache := NewRuntimeCache(1, 4)
	key := NewFastInstanceKey([]byte("artifact"), []byte("projection"))
	started := make(chan struct{})
	release := make(chan struct{})
	want := fmt.Errorf("owner failure")
	var executions atomic.Int32
	ownerErr := make(chan error, 1)
	go func() {
		_, err := cache.LoadOrCompute(key, []byte("artifact"), []byte("projection"), func() (ClosedOutcome, error) {
			executions.Add(1)
			close(started)
			<-release
			return ClosedOutcome{}, want
		})
		ownerErr <- err
	}()
	<-started
	joinerErr := make(chan error, 1)
	go func() {
		_, err := cache.LoadOrCompute(key, []byte("artifact"), []byte("projection"), func() (ClosedOutcome, error) {
			executions.Add(1)
			return NewClosedOutcome([]byte("must-not-run"))
		})
		joinerErr <- err
	}()
	for cache.Metrics().Joins != 1 {
		runtime.Gosched()
	}
	close(release)
	if got := <-ownerErr; got != want {
		t.Fatalf("owner error = %v, want %v", got, want)
	}
	if got := <-joinerErr; got != want {
		t.Fatalf("joiner error = %v, want owner %v", got, want)
	}
	if executions.Load() != 1 {
		t.Fatalf("executions = %d, want one", executions.Load())
	}
}

func TestRuntimeCacheParityWithProjectedTableForClosedHit(t *testing.T) {
	artifact := demandedBodyArtifactFixture(t)
	entry := tableEntry(t, "same", "true", "diagnostic", "unread")
	reference := NewProjectedTable()
	var referenceRuns atomic.Int32
	want, err := reference.Resolve(context.Background(), artifact, entry, countingRunner(&referenceRuns))
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := NewScratchProjectionEncoder(artifact)
	if err != nil {
		t.Fatal(err)
	}
	scratch := NewEvaluatorScratch(512, 1)
	key, projection, err := encoder.Encode(scratch, entry)
	if err != nil {
		t.Fatal(err)
	}
	cache := NewRuntimeCache(2, 4)
	if err := cache.Store(key, encoder.artifactCanonical, projection, want); err != nil {
		t.Fatal(err)
	}
	got, ok := cache.Load(key, encoder.artifactCanonical, projection)
	if !ok || !bytes.Equal(got.canonical, want.canonical) {
		t.Fatalf("runtime closed hit = %q, %v; want reference %q", got.canonical, ok, want.canonical)
	}
}

func TestRuntimeCacheWarmedHitHasNoAllocations(t *testing.T) {
	artifact := demandedBodyArtifactFixture(t)
	entry := tableEntry(t, "same", "true", "diagnostic", "unread")
	encoder, err := NewScratchProjectionEncoder(artifact)
	if err != nil {
		t.Fatal(err)
	}
	scratch := NewEvaluatorScratch(512, 1)
	key, projection, err := encoder.Encode(scratch, entry)
	if err != nil {
		t.Fatal(err)
	}
	cache := NewRuntimeCache(2, 4)
	outcome, err := NewClosedOutcome([]byte("closed-outcome"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Store(key, encoder.artifactCanonical, projection, outcome); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		scratch.Reset()
		gotKey, gotProjection, err := encoder.Encode(scratch, entry)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := cache.Load(gotKey, encoder.artifactCanonical, gotProjection)
		if !ok || !got.Valid() {
			t.Fatal("warmed cache hit missed")
		}
	})
	if allocations != 0 {
		t.Fatalf("warmed projection/cache hit allocations = %v, want 0", allocations)
	}
}

func BenchmarkCompiledProjectedApplication_Hit(b *testing.B) {
	artifact := demandedBodyArtifactFixture(b)
	entry := tableEntry(b, "same", "true", "diagnostic", "unread")
	encoder, err := NewScratchProjectionEncoder(artifact)
	if err != nil {
		b.Fatal(err)
	}
	scratch := NewEvaluatorScratch(512, 1)
	key, projection, err := encoder.Encode(scratch, entry)
	if err != nil {
		b.Fatal(err)
	}
	cache := NewRuntimeCache(2, 4)
	outcome, err := NewClosedOutcome([]byte("closed-outcome"))
	if err != nil {
		b.Fatal(err)
	}
	if err := cache.Store(key, encoder.artifactCanonical, projection, outcome); err != nil {
		b.Fatal(err)
	}
	runCompiledProjectedHitBenchmark(b, cache, encoder, entry, scratch, 1)
}

// BenchmarkCompiledProjectedApplication_Hit_Fanout measures equal certified
// callers after their one shared instance has been published.  The fanout is
// deliberately in the measured operation: it catches a regression which only
// appears when a call site starts serving many closed applications.
func BenchmarkCompiledProjectedApplication_Hit_Fanout(b *testing.B) {
	for _, fanout := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("fanout-%d", fanout), func(b *testing.B) {
			artifact := demandedBodyArtifactFixture(b)
			entry := tableEntry(b, "same", "true", "diagnostic", "unread")
			encoder, err := NewScratchProjectionEncoder(artifact)
			if err != nil {
				b.Fatal(err)
			}
			scratch := NewEvaluatorScratch(512, 1)
			key, projection, err := encoder.Encode(scratch, entry)
			if err != nil {
				b.Fatal(err)
			}
			cache := NewRuntimeCache(2, 4)
			outcome, err := NewClosedOutcome([]byte("closed-outcome"))
			if err != nil {
				b.Fatal(err)
			}
			if err := cache.Store(key, encoder.artifactCanonical, projection, outcome); err != nil {
				b.Fatal(err)
			}
			runCompiledProjectedHitBenchmark(b, cache, encoder, entry, scratch, fanout)
		})
	}
}

// BenchmarkCompiledProjectedApplication_Hit_CollisionConfirmation measures the
// mandatory byte-for-byte witness confirmation on a fixed-key collision. A
// digest match is never an equality result, even on the hot path.
func BenchmarkCompiledProjectedApplication_Hit_CollisionConfirmation(b *testing.B) {
	cache := NewRuntimeCache(1, 4)
	key := FastInstanceKey{ArtifactID: ContentID{1}, ProjectionID: ContentID{2}}
	leftArtifact, leftProjection := []byte("artifact-left"), []byte("projection-left")
	rightArtifact, rightProjection := []byte("artifact-right"), []byte("projection-right")
	if err := cache.storeCold(key, leftArtifact, leftProjection, []byte("outcome-left")); err != nil {
		b.Fatal(err)
	}
	if err := cache.storeCold(key, rightArtifact, rightProjection, []byte("outcome-right")); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := cache.Load(key, rightArtifact, rightProjection); !ok {
			b.Fatal("collision-confirmed warmed cache hit missed")
		}
	}
}

// BenchmarkCompiledProjectedApplication_Hit_ConcurrentClosedHits gives every
// worker its own scratch arena.  RuntimeCache closed reads themselves need no
// locks, channels, pools, or caller-owned allocations.
func BenchmarkCompiledProjectedApplication_Hit_ConcurrentClosedHits(b *testing.B) {
	artifact := demandedBodyArtifactFixture(b)
	entry := tableEntry(b, "same", "true", "diagnostic", "unread")
	encoder, err := NewScratchProjectionEncoder(artifact)
	if err != nil {
		b.Fatal(err)
	}
	seed := NewEvaluatorScratch(512, 1)
	key, projection, err := encoder.Encode(seed, entry)
	if err != nil {
		b.Fatal(err)
	}
	cache := NewRuntimeCache(4, 4)
	outcome, err := NewClosedOutcome([]byte("closed-outcome"))
	if err != nil {
		b.Fatal(err)
	}
	if err := cache.Store(key, encoder.artifactCanonical, projection, outcome); err != nil {
		b.Fatal(err)
	}
	scratches := make([]*EvaluatorScratch, runtime.GOMAXPROCS(0)*4)
	for i := range scratches {
		scratches[i] = NewEvaluatorScratch(512, 1)
	}
	var next atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		scratch := scratches[next.Add(1)%uint64(len(scratches))]
		for pb.Next() {
			scratch.Reset()
			gotKey, gotProjection, err := encoder.Encode(scratch, entry)
			if err != nil {
				b.Fatal(err)
			}
			if _, ok := cache.Load(gotKey, encoder.artifactCanonical, gotProjection); !ok {
				b.Fatal("concurrent warmed cache hit missed")
			}
		}
	})
}

func runCompiledProjectedHitBenchmark(b *testing.B, cache *RuntimeCache, encoder *ScratchProjectionEncoder, entry EntryBinding, scratch *EvaluatorScratch, fanout int) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for caller := 0; caller < fanout; caller++ {
			scratch.Reset()
			gotKey, gotProjection, err := encoder.Encode(scratch, entry)
			if err != nil {
				b.Fatal(err)
			}
			if _, ok := cache.Load(gotKey, encoder.artifactCanonical, gotProjection); !ok {
				b.Fatal("warmed cache hit missed")
			}
		}
	}
}
