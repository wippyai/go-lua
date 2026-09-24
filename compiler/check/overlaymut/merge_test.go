package overlaymut

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyFieldWritesToOverlay_WritesLandOnNestedTable(t *testing.T) {
	sym := cfg.SymbolID(1)
	state := typ.NewRecord().Field("clears", typ.NewArray(typ.String)).Build()
	overlay := map[cfg.SymbolID]typ.Type{
		sym: typ.NewRecord().Field("state", state).Build(),
	}
	stateSegs := []constraint.Segment{{Kind: constraint.SegmentField, Name: "state"}}
	ApplyFieldWritesToOverlay(overlay, map[cfg.SymbolID]api.FieldWriteSet{
		sym: {
			api.NewFieldWriteKey(stateSegs, flow.IndexerWriteField): typ.NewMap(typ.String, typ.Any),
			api.NewFieldWriteKey(stateSegs, "mode"):                 typ.String,
			{Field: "version"}:                                      typ.Integer,
		},
	})

	root, ok := overlay[sym].(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %v", overlay[sym])
	}
	if root.GetField("version") == nil {
		t.Errorf("expected direct field version on %v", root)
	}
	nested, ok := root.GetField("state").Type.(*typ.Record)
	if !ok {
		t.Fatalf("expected state record, got %v", root.GetField("state").Type)
	}
	if nested.GetField("clears") == nil || nested.GetField("mode") == nil {
		t.Errorf("expected clears and mode on %v", nested)
	}
	if !nested.HasMapComponent() || !typ.TypeEquals(nested.MapValue, typ.Any) {
		t.Errorf("expected map component [string]: any on %v", nested)
	}
}

func TestFieldWriteKey_UnderPrefixesPath(t *testing.T) {
	key := api.NewFieldWriteKey([]constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}}, "x")
	moved := key.Under([]constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}})
	segs := moved.Segments()
	if len(segs) != 2 || segs[0].Name != "a" || segs[1].Name != "b" || moved.Field != "x" {
		t.Errorf("expected .a.b / x, got %q / %q", moved.Path, moved.Field)
	}
}
