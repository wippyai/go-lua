package body

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestInputLineageRetainsLegacyUint64Parity(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	configs := []SolveConfig{
		{SummaryInputDigests: func() []uint64 { return []uint64{9, 3, 9} }},
		{SummaryInputs: func() []SummaryInput { return []SummaryInput{lineageSummaryInput(1, 2, true, 3)} }},
	}
	for i, config := range configs {
		legacy, err := InputDigestContext(prepared, config)
		if err != nil {
			t.Fatalf("config %d legacy: %v", i, err)
		}
		lineage, err := InputLineageContext(prepared, config)
		if err != nil {
			t.Fatalf("config %d lineage: %v", i, err)
		}
		if lineage.ResultVersion() != legacy {
			t.Fatalf("config %d lineage legacy = %d, InputDigest = %d", i, lineage.ResultVersion(), legacy)
		}
		if lineage.Complete() || lineage.Digest() != (ResultVersionDigest{}) {
			t.Fatalf("config %d exposed incomplete lineage as authoritative", i)
		}
	}
}

func TestResultVersionLineageAuthorityRequiresCompleteCollisionSafeInputs(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	empty := lineageForConfig(t, prepared, SolveConfig{SummaryInputsComplete: true})
	if empty.Complete() || empty.Digest() != (ResultVersionDigest{}) {
		t.Fatal("uint64-collapsed static/state inputs exposed authoritative digest")
	}
	missing := lineageForConfig(t, prepared, SolveConfig{
		SummaryInputsComplete: true,
		SummaryInputs: func() []SummaryInput {
			return []SummaryInput{lineageSummaryInput(1, 1, false, 0)}
		},
	})
	if missing.Complete() || missing.Digest() != (ResultVersionDigest{}) {
		t.Fatal("missing-only lineage exposed non-collision-safe authority")
	}
	present := lineageForConfig(t, prepared, SolveConfig{
		SummaryInputsComplete: true,
		SummaryInputs: func() []SummaryInput {
			return []SummaryInput{lineageSummaryInput(1, 1, true, 7)}
		},
	})
	if present.Complete() || present.Digest() != (ResultVersionDigest{}) {
		t.Fatal("uint64-only present payload exposed authoritative digest")
	}
	anonymous := lineageForConfig(t, prepared, SolveConfig{
		SummaryInputsComplete: true,
		SummaryInputDigests:   func() []uint64 { return []uint64{7} },
	})
	if anonymous.Complete() || anonymous.Digest() != (ResultVersionDigest{}) {
		t.Fatal("anonymous compatibility input exposed authoritative digest")
	}
	if anonymous.ResultVersion() == 0 || present.ResultVersion() == 0 {
		t.Fatal("incomplete lineage lost legacy ResultVersion")
	}
}

func TestResultVersionLineageRejectsTypedAndAnonymousProviders(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = InputLineageContext(prepared, SolveConfig{
		SummaryInputs:       func() []SummaryInput { return nil },
		SummaryInputDigests: func() []uint64 { return nil },
	})
	if !errors.Is(err, ErrConflictingSummaryInputProviders) {
		t.Fatalf("InputLineageContext error = %v, want provider conflict", err)
	}
}

func TestResultVersionLineageCanonicalizesIdenticalDuplicateKeyOnce(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	input := lineageSummaryInput(1, 2, false, 0)
	lineage := lineageForConfig(t, prepared, SolveConfig{
		SummaryInputsComplete: true,
		SummaryInputs:         func() []SummaryInput { return []SummaryInput{input, input} },
	})
	if got := lineage.SummaryInputs(); len(got) != 1 || got[0] != input {
		t.Fatalf("canonical duplicate inputs = %#v", got)
	}
	if lineage.Complete() || lineage.Digest() != (ResultVersionDigest{}) {
		t.Fatal("identical missing duplicate exposed non-collision-safe authority")
	}
}

func TestResultVersionLineageCoversExactSolveConfiguration(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	base := lineageForConfig(t, prepared, SolveConfig{})
	variants := map[string]SolveConfig{
		"lanes": {StateLanes: []state.LaneID{state.LaneValues}},
		"widen": {WidenAt: func(cfg.Point) bool { return false }},
		"initial": {Initial: func(cfg.Point) (state.State, bool) {
			return state.State{}, false
		}},
	}
	for name, config := range variants {
		assertDistinctLineage(t, name, base, lineageForConfig(t, prepared, config))
	}
}

func TestResultVersionLineageCanonicalizesSummaryInputPermutation(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	a := lineageSummaryInput(2, 9, true, 101)
	b := lineageSummaryInput(1, 4, false, 0)
	left := lineageForInputs(t, prepared, []SummaryInput{a, b})
	right := lineageForInputs(t, prepared, []SummaryInput{b, a})
	if left.ResultVersion() != right.ResultVersion() || left.Digest() != right.Digest() {
		t.Fatalf("summary input permutation changed lineage\nleft: %d %s\nright: %d %s",
			left.ResultVersion(), left.Digest(), right.ResultVersion(), right.Digest())
	}
	got := left.SummaryInputs()
	if len(got) != 2 || got[0].Key.RefID != 1 || got[1].Key.RefID != 2 {
		t.Fatalf("canonical summary inputs = %#v", got)
	}
}

func TestResultVersionLineageSeparatesSummaryKeySwap(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	left := lineageForInputs(t, prepared, []SummaryInput{lineageSummaryInput(1, 7, true, 101)})
	right := lineageForInputs(t, prepared, []SummaryInput{lineageSummaryInput(2, 7, true, 101)})
	if left.ResultVersion() != right.ResultVersion() {
		t.Fatalf("typed key changed legacy ResultVersion: %d != %d", left.ResultVersion(), right.ResultVersion())
	}
	if left.SummaryInputs()[0].Key == right.SummaryInputs()[0].Key {
		t.Fatal("typed lineage lost summary key separation")
	}
	if left.Complete() || right.Complete() || left.Digest() != (ResultVersionDigest{}) || right.Digest() != (ResultVersionDigest{}) {
		t.Fatal("key-separated uint64 lineage exposed full-width authority")
	}
}

func TestResultVersionLineageSeparatesPresentAndMissingRead(t *testing.T) {
	prepared, err := PrepareChunk(parseChunk(t, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	present := lineageForInputs(t, prepared, []SummaryInput{lineageSummaryInput(1, 7, true, 0)})
	missing := lineageForInputs(t, prepared, []SummaryInput{lineageSummaryInput(1, 7, false, 999)})
	assertDistinctLineage(t, "present/missing", present, missing)
	if got := missing.SummaryInputs()[0].PayloadDigest; got != 0 {
		t.Fatalf("missing dependency retained payload digest %d", got)
	}
}

func BenchmarkResultVersionLineage(b *testing.B) {
	prepared, err := PrepareChunk(parseChunk(b, "return 1"), Config{Registry: standard.Registry()})
	if err != nil {
		b.Fatal(err)
	}
	inputs := []SummaryInput{
		lineageSummaryInput(3, 7, true, 101),
		lineageSummaryInput(1, 5, false, 0),
		lineageSummaryInput(2, 6, true, 202),
	}
	config := SolveConfig{SummaryInputs: func() []SummaryInput { return inputs }}
	if _, err := InputLineageContext(prepared, config); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := InputLineageContext(prepared, config); err != nil {
			b.Fatal(err)
		}
	}
}

func lineageSummaryInput(refID, entry uint64, present bool, payload uint64) SummaryInput {
	return SummaryInput{
		Key: SummaryInputKey{
			RefKind:    2,
			RefID:      refID,
			Values:     entry,
			Facts:      entry + 1,
			References: entry + 2,
		},
		Present:       present,
		PayloadDigest: payload,
	}
}

func lineageForInputs(t *testing.T, prepared *Static, inputs []SummaryInput) ResultVersionLineage {
	t.Helper()
	lineage, err := InputLineageContext(prepared, SolveConfig{
		SummaryInputsComplete: true,
		SummaryInputs:         func() []SummaryInput { return append([]SummaryInput(nil), inputs...) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return lineage
}

func lineageForConfig(t *testing.T, prepared *Static, config SolveConfig) ResultVersionLineage {
	t.Helper()
	lineage, err := InputLineageContext(prepared, config)
	if err != nil {
		t.Fatal(err)
	}
	return lineage
}

func assertDistinctLineage(t *testing.T, name string, left, right ResultVersionLineage) {
	t.Helper()
	if left.ResultVersion() == right.ResultVersion() {
		t.Fatalf("%s did not change legacy version %d", name, left.ResultVersion())
	}
	if left.Complete() && right.Complete() && left.Digest() == right.Digest() {
		t.Fatalf("%s did not change full digest %s", name, left.Digest())
	}
}
