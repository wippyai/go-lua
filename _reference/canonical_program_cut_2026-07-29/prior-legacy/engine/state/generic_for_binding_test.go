package state

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
)

func TestGenericForTransferLanesMatchRegisteredDefaultSemantics(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	tests := []struct {
		name         string
		indexedValue bool
		source       []LaneID
		current      []LaneID
		writes       []LaneID
	}{
		{
			name:    "ordinary value",
			source:  []LaneID{LaneHeapTableIdentity},
			current: []LaneID{LaneKeyMemberships},
			writes:  []LaneID{LaneKeyMemberships},
		},
		{
			name:         "indexed value",
			indexedValue: true,
			source:       []LaneID{LanePathEvidence, LaneDynamicIndex, LaneHeapTableIdentity},
			current:      []LaneID{LanePathEvidence, LaneKeyMemberships},
			writes:       []LaneID{LanePathEvidence, LaneKeyMemberships},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, current, writes, err := domain.GenericForTransferLanes(test.indexedValue)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneIDsEqual(t, "source reads", source.IDs(), test.source)
			assertLaneIDsEqual(t, "current reads", current.IDs(), test.current)
			assertLaneIDsEqual(t, "writes", writes.IDs(), test.writes)
		})
	}
}

func TestLaneCatalogRejectsMissingGenericForBindingLaw(t *testing.T) {
	missing := valuesLaneSpec
	missing.id = LaneID("test.missing-generic-for-binding")
	missing.semanticLaws = withoutSemanticLaw(missing.semanticLaws, laneSemanticGenericForBinding)
	defer func() {
		got := recover()
		if got == nil || !strings.Contains(got.(string), `has no semantic capability "generic-for-binding"`) {
			t.Fatalf("panic = %v", got)
		}
	}()
	_ = newLaneCatalog([]laneSpec{missing})
}

func TestLaneCatalogRejectsIncompleteGenericForBindingLaw(t *testing.T) {
	incomplete := valuesLaneSpec
	incomplete.id = LaneID("test.incomplete-generic-for-binding")
	incomplete.semanticLaws = append([]laneSemanticLaw(nil), incomplete.semanticLaws...)
	for index := range incomplete.semanticLaws {
		if incomplete.semanticLaws[index].id == laneSemanticGenericForBinding {
			incomplete.semanticLaws[index].genericForBinding = nil
		}
	}
	defer func() {
		got := recover()
		if got == nil || !strings.Contains(got.(string), `has an incomplete semantic capability "generic-for-binding"`) {
			t.Fatalf("panic = %v", got)
		}
	}()
	_ = newLaneCatalog([]laneSpec{incomplete})
}

func withoutSemanticLaw(laws []laneSemanticLaw, omitted laneSemanticCapabilityID) []laneSemanticLaw {
	out := make([]laneSemanticLaw, 0, len(laws)-1)
	for _, law := range laws {
		if law.id != omitted {
			out = append(out, law)
		}
	}
	return out
}

func assertLaneIDsEqual(t *testing.T, name string, got, want []LaneID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
}
