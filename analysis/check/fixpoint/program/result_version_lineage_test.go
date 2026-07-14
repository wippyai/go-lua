package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestTrackedSummaryInputsPreserveKeyPresenceAndNormalizedPayload(t *testing.T) {
	reg := standard.Registry()
	presentKey := summary.SummaryKey{
		Ref: ref.FuncRef{Kind: ref.KindSymbol, ID: 17},
		Entry: summary.EntryKey{
			Values: 3, Facts: 5, References: 7,
		},
	}
	missingKey := summary.SummaryKey{
		Ref: ref.FuncRef{Kind: ref.KindCFG, ID: 19},
		Entry: summary.EntryKey{
			Values: 11, Facts: 13, References: 17,
		},
	}
	present := summary.Normalize(reg, summary.Summary{MaySuspend: true})
	deps := map[summary.SummaryKey]trackedSummaryRead{
		presentKey: {present: true, sum: present},
		missingKey: {},
	}
	inputs := trackedSummaryInputs(nil, reg, deps)
	if len(inputs) != 2 {
		t.Fatalf("inputs = %d, want 2", len(inputs))
	}
	byID := make(map[uint64]struct {
		present bool
		payload uint64
	}, len(inputs))
	for _, input := range inputs {
		byID[input.Key.RefID] = struct {
			present bool
			payload uint64
		}{input.Present, input.PayloadDigest}
	}
	if got := byID[presentKey.Ref.ID]; !got.present || got.payload != uint64(summary.NormalizedPayloadDigest(reg, present)) {
		t.Fatalf("present record = %#v", got)
	}
	if got := byID[missingKey.Ref.ID]; got.present || got.payload != 0 {
		t.Fatalf("missing record = %#v", got)
	}
	for _, input := range inputs {
		var want summary.SummaryKey
		switch input.Key.RefID {
		case presentKey.Ref.ID:
			want = presentKey
		case missingKey.Ref.ID:
			want = missingKey
		default:
			t.Fatalf("unexpected ref id %d", input.Key.RefID)
		}
		if input.Key.RefKind != uint8(want.Ref.Kind) || input.Key.Values != uint64(want.Entry.Values) ||
			input.Key.Facts != uint64(want.Entry.Facts) || input.Key.References != uint64(want.Entry.References) {
			t.Fatalf("key record = %#v, want %#v", input.Key, want)
		}
	}
}

func TestUncachedSummaryPreparedBuildsTrackedConfigOnce(t *testing.T) {
	prepared, reg := lineagePreparedBody(t)
	builds := 0
	_, err := solveSummaryPrepared(nil, nil, "", 0, prepared, nil, func(summary.Reader) body.Config {
		builds++
		return body.Config{Registry: reg}
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Fatalf("uncached config builds = %d, want 1", builds)
	}
}

func BenchmarkUncachedSummaryPreparedBuildOnce(b *testing.B) {
	prepared, reg := lineagePreparedBody(b)
	builds := 0
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := solveSummaryPrepared(nil, nil, "", 0, prepared, nil, func(summary.Reader) body.Config {
			builds++
			return body.Config{Registry: reg}
		}, nil, nil); err != nil {
			b.Fatal(err)
		}
	}
	if builds != b.N {
		b.Fatalf("uncached config builds = %d, want %d", builds, b.N)
	}
}

func lineagePreparedBody(tb testing.TB) (*body.Static, *axis.Registry) {
	tb.Helper()
	reg := standard.Registry()
	stmts, err := parse.ParseString("return 1", "result_version_lineage_test.lua")
	if err != nil {
		tb.Fatal(err)
	}
	prepared, err := body.PrepareChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		tb.Fatal(err)
	}
	return prepared, reg
}
