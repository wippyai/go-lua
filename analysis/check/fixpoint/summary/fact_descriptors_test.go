package summary

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// summaryFactCorpus returns a representative set of summaries for lane parity:
// the empty summary, one summary per populated payload field, and a summary with
// every field populated at once. It reuses summaryWithOneLane so the corpus
// stays in lockstep with the Summary schema.
func summaryFactCorpus(t *testing.T) []Summary {
	t.Helper()
	typ := reflect.TypeOf(Summary{})
	out := []Summary{{}}
	combined := Summary{}
	combinedValue := reflect.ValueOf(&combined).Elem()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Name == "HeapKeySpace" {
			continue
		}
		single := summaryWithOneLane(t, field.Name)
		out = append(out, single)
		combinedValue.FieldByName(field.Name).Set(reflect.ValueOf(single).FieldByName(field.Name))
	}
	out = append(out, combined)
	return out
}

// TestSummaryFactDescriptorsDeriveLiveLanes proves the descriptor table
// derives lanes structurally identical to the live summaryLanes: same
// order, field name, slot flag, and the same set of non-nil ops per lane.
func TestSummaryFactDescriptorsDeriveLiveLanes(t *testing.T) {
	derived := derivedSummaryLanes()
	if len(derived) != len(summaryLanes) {
		t.Fatalf("derived lanes = %d, want live = %d", len(derived), len(summaryLanes))
	}
	for i, hand := range summaryLanes {
		d := derived[i]
		if d.fieldName != hand.fieldName {
			t.Fatalf("lane[%d] field = %q, want %q", i, d.fieldName, hand.fieldName)
		}
		if d.slot != hand.slot {
			t.Fatalf("lane[%d] %s slot = %v, want %v", i, hand.fieldName, d.slot, hand.slot)
		}
		if (d.empty == nil) != (hand.empty == nil) {
			t.Fatalf("lane[%d] %s empty nil-ness mismatch", i, hand.fieldName)
		}
		if (d.assignClone == nil) != (hand.assignClone == nil) {
			t.Fatalf("lane[%d] %s assignClone nil-ness mismatch", i, hand.fieldName)
		}
		if (d.normalizeOwned == nil) != (hand.normalizeOwned == nil) {
			t.Fatalf("lane[%d] %s normalizeOwned nil-ness mismatch", i, hand.fieldName)
		}
		if (d.equal == nil) != (hand.equal == nil) {
			t.Fatalf("lane[%d] %s equal nil-ness mismatch", i, hand.fieldName)
		}
		if (d.lessOrEq == nil) != (hand.lessOrEq == nil) {
			t.Fatalf("lane[%d] %s lessOrEq nil-ness mismatch", i, hand.fieldName)
		}
		if (d.assignJoin == nil) != (hand.assignJoin == nil) {
			t.Fatalf("lane[%d] %s assignJoin nil-ness mismatch", i, hand.fieldName)
		}
		if (d.assignWiden == nil) != (hand.assignWiden == nil) {
			t.Fatalf("lane[%d] %s assignWiden nil-ness mismatch", i, hand.fieldName)
		}
	}
}

// TestSummaryFactDescriptorsBehaviorParity proves every derived lane op produces
// the same result as the live lane op across the summary corpus. The
// corpus includes zero-value facts that some lattice helpers reject by panicking;
// the comparison captures both value and panic so it proves the derived and
// live paths agree on error behavior too.
func TestSummaryFactDescriptorsBehaviorParity(t *testing.T) {
	reg := mustRegistry(t)
	derived := derivedSummaryLanes()
	corpus := summaryFactCorpus(t)

	for i, hand := range summaryLanes {
		d := derived[i]

		for _, s := range corpus {
			s := s
			compareBool(t, hand.fieldName+" empty",
				func() bool { return d.empty(s) },
				func() bool { return hand.empty(s) })
			compareSummary(t, hand.fieldName+" assignClone",
				func() Summary { return applyClone(d.assignClone, s) },
				func() Summary { return applyClone(hand.assignClone, s) })
			compareSummary(t, hand.fieldName+" normalizeOwned",
				func() Summary { return applyNormalize(reg, d.normalizeOwned, s) },
				func() Summary { return applyNormalize(reg, hand.normalizeOwned, s) })
		}

		if hand.equal == nil {
			continue
		}
		for _, a := range corpus {
			for _, b := range corpus {
				a, b := a, b
				compareBool(t, hand.fieldName+" equal",
					func() bool { return d.equal(reg, a, b, false) },
					func() bool { return hand.equal(reg, a, b, false) })
				compareBool(t, hand.fieldName+" normalized equal",
					func() bool { return d.equal(reg, a, b, true) },
					func() bool { return hand.equal(reg, a, b, true) })
				compareBool(t, hand.fieldName+" lessOrEq",
					func() bool { return d.lessOrEq(reg, a, b) },
					func() bool { return hand.lessOrEq(reg, a, b) })
				compareSummary(t, hand.fieldName+" assignJoin",
					func() Summary { return applyMerge(reg, d.assignJoin, a, b) },
					func() Summary { return applyMerge(reg, hand.assignJoin, a, b) })
				compareSummary(t, hand.fieldName+" assignWiden",
					func() Summary { return applyMerge(reg, d.assignWiden, a, b) },
					func() Summary { return applyMerge(reg, hand.assignWiden, a, b) })
			}
		}
	}
}

func applyClone(fn func(src Summary, dst *Summary), s Summary) Summary {
	var out Summary
	fn(s, &out)
	return out
}

func applyNormalize(reg *axis.Registry, fn func(reg *axis.Registry, s *Summary), s Summary) Summary {
	out := s.Clone()
	fn(reg, &out)
	return out
}

func applyMerge(reg *axis.Registry, fn func(reg *axis.Registry, a, b Summary, out *Summary), a, b Summary) Summary {
	var out Summary
	fn(reg, a, b, &out)
	return out
}

func captureBool(fn func() bool) (out bool, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	return fn(), false
}

func captureSummary(fn func() Summary) (out Summary, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	return fn(), false
}

func compareBool(t *testing.T, name string, got, want func() bool) {
	t.Helper()
	gotVal, gotPanic := captureBool(got)
	wantVal, wantPanic := captureBool(want)
	if gotPanic != wantPanic {
		t.Fatalf("%s panic mismatch: derived=%v hand=%v", name, gotPanic, wantPanic)
	}
	if !gotPanic && gotVal != wantVal {
		t.Fatalf("%s value mismatch: derived=%v hand=%v", name, gotVal, wantVal)
	}
}

func compareSummary(t *testing.T, name string, got, want func() Summary) {
	t.Helper()
	gotVal, gotPanic := captureSummary(got)
	wantVal, wantPanic := captureSummary(want)
	if gotPanic != wantPanic {
		t.Fatalf("%s panic mismatch: derived=%v hand=%v", name, gotPanic, wantPanic)
	}
	if !gotPanic && !reflect.DeepEqual(gotVal, wantVal) {
		t.Fatalf("%s value mismatch", name)
	}
}

// TestSummaryFactDescriptorsWireRefs pins the manifest wire lane cross-reference:
// MaySuspend and ReturnPresenceRelations lower 1:1 into the OperationalEffects
// wire codec; NormalReturnFacts is a nested family and every other lane
// serializes through the signature return/param/postcondition encoders, not the
// wire codec.
func TestSummaryFactDescriptorsWireRefs(t *testing.T) {
	want := map[string][]string{
		"MaySuspend":              {"MaySuspend"},
		"ReturnPresenceRelations": {"ReturnPresenceRelations"},
	}
	for _, d := range SummaryFactDescriptors() {
		expected := want[string(d.Kind)]
		if !reflect.DeepEqual(d.WireRef, expected) {
			t.Fatalf("kind %q wire ref = %#v, want %#v", d.Kind, d.WireRef, expected)
		}
	}
}
