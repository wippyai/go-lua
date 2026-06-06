package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestApplyPathAliasProofPublishesAlias(t *testing.T) {
	value := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(1), "value"))
	source := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(2), "source"))
	state := PointState{}

	if !ApplyPathAliasProof(&state, PathAliasProof{Value: value, Source: source}) {
		t.Fatal("ApplyPathAliasProof reported no change")
	}
	aliases := state.PathAliases.AliasesOfAddress(value)
	if len(aliases) != 1 || aliases[0].Source != source.Key() {
		t.Fatalf("aliases = %v, want source %s", aliases, source.Key())
	}
}

func TestApplyValueOriginProofPublishesOrigin(t *testing.T) {
	value := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(3), "value"))
	source := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(4), "source"))
	state := PointState{}

	if !ApplyValueOriginProof(&state, ValueOriginProof{
		Value:    value,
		Source:   source,
		Kind:     ValueOriginAssignmentAlias,
		VarIndex: 1,
	}) {
		t.Fatal("ApplyValueOriginProof reported no change")
	}
	origins := state.ValueOrigins.OriginsOfAddress(value)
	if len(origins) != 1 || origins[0].Source != source.Key() || origins[0].Kind != ValueOriginAssignmentAlias || origins[0].VarIndex != 1 {
		t.Fatalf("origins = %v, want assignment alias from %s", origins, source.Key())
	}
}
