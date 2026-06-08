package paramuse

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestSetEvidenceIsStableAndMerged(t *testing.T) {
	name, _ := fieldkey.FromName("name")
	id := constraint.Segment{Kind: constraint.SegmentIndexString, Name: "id"}

	var set Set
	set.Field(2, id)
	set.MarkWhole(1)
	set.Field(2, name)
	set.Field(2, name)

	got := set.Evidence()
	if len(got) != 2 {
		t.Fatalf("Evidence() length = %d, want 2: %#v", len(got), got)
	}
	if got[0].Symbol != 1 || !got[0].Whole || len(got[0].Fields) != 0 {
		t.Fatalf("first evidence = %#v, want whole symbol 1", got[0])
	}
	if got[1].Symbol != 2 || got[1].Whole || len(got[1].Fields) != 2 {
		t.Fatalf("second evidence = %#v, want two fields for symbol 2", got[1])
	}
}

func TestFromEvidenceNormalizesInvalidAndDuplicateFields(t *testing.T) {
	set := FromEvidence([]api.ParameterUseEvidence{
		{Symbol: 0, Whole: true},
		{
			Symbol: 3,
			Fields: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "kind"},
				{Kind: constraint.SegmentField, Name: "kind"},
				{Kind: constraint.SegmentField},
			},
		},
	})

	if set.Len() != 1 {
		t.Fatalf("Len() = %d, want one valid symbol", set.Len())
	}
	use, ok := set.Get(cfg.SymbolID(3))
	if !ok || !use.Observed() || use.Whole() {
		t.Fatalf("use = %#v, ok=%v; want field-only observed use", use, ok)
	}
	if len(use.Fields()) != 1 {
		t.Fatalf("fields = %#v, want one normalized field", use.Fields())
	}
}
