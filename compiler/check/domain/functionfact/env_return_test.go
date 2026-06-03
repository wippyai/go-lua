package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type envReturnProofFacts struct {
	condition constraint.Condition
}

func (f envReturnProofFacts) DeclaredAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f envReturnProofFacts) RefinedAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: nil, State: flow.StateUnknown}
}

func (f envReturnProofFacts) EffectiveTypeAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f envReturnProofFacts) IsAnnotated(cfg.SymbolID) bool {
	return false
}

func (f envReturnProofFacts) ConditionAt(cfg.Point) constraint.Condition {
	return f.condition
}

func (f envReturnProofFacts) ProvesTypeAt(cfg.Point, constraint.Path, typ.Type) bool {
	return false
}

func (f envReturnProofFacts) ConditionTypeAt(cfg.Point, constraint.Path) typ.Type {
	return nil
}

func (f envReturnProofFacts) ConditionedTypeAt(cfg.Point, constraint.Path, constraint.Condition) typ.Type {
	return nil
}

func (f envReturnProofFacts) ConditionedSeedTypeAt(cfg.Point, constraint.Path, typ.Type, constraint.Path, constraint.Condition) typ.Type {
	return nil
}

func TestEnvReturnConditionUsesConditionProofFactsWithoutConcreteSolver(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"args"}}}
	graph := cfg.Build(fn)
	params := graph.ParamSlotsReadOnly()
	if len(params) != 1 {
		t.Fatalf("param slots len = %d, want 1", len(params))
	}

	paramPath := constraint.Path{Root: "args", Symbol: params[0].Symbol}.Field("bad")
	result := &api.FuncResult{
		Graph: graph,
		Facts: envReturnProofFacts{
			condition: constraint.FromConstraints(constraint.Truthy{Path: paramPath}),
		},
	}

	got := envReturnCondition(result, graph.Entry())
	want := constraint.FromConstraints(constraint.Truthy{Path: constraint.ParamPath(0).Field("bad")})
	if !got.Equals(want) {
		t.Fatalf("envReturnCondition() = %v, want %v", got, want)
	}
}

func TestNormalizeEnvReturns_UsesStructuralPathIdentity(t *testing.T) {
	fieldPath := []constraint.Segment{{Kind: constraint.SegmentField, Name: "x-y"}}
	indexPath := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "x-y"}}
	got := NormalizeEnvReturns([]contract.EnvReturnSpec{
		{ReturnIndex: 0, ResultIndex: 0, Path: indexPath, Args: []typ.Type{typ.String}},
		{ReturnIndex: 0, ResultIndex: 0, Path: fieldPath, Args: []typ.Type{typ.Number}},
	})

	if len(got) != 2 {
		t.Fatalf("NormalizeEnvReturns len = %d, want distinct field and string-index specs: %#v", len(got), got)
	}
	if len(got[0].Path) != 1 || got[0].Path[0].Kind != constraint.SegmentField {
		t.Fatalf("first normalized env return path = %#v, want dot field first", got[0].Path)
	}
	if len(got[1].Path) != 1 || got[1].Path[0].Kind != constraint.SegmentIndexString {
		t.Fatalf("second normalized env return path = %#v, want string-index path", got[1].Path)
	}
}

func TestJoinEnvReturns_MergesSameTypedIdentity(t *testing.T) {
	path := []constraint.Segment{{Kind: constraint.SegmentField, Name: "invoke"}}
	left := contract.EnvReturnSpec{
		ReturnIndex: 0,
		ResultIndex: 0,
		Path:        path,
		Args:        []typ.Type{typ.Integer},
	}
	right := contract.EnvReturnSpec{
		ReturnIndex: 0,
		ResultIndex: 0,
		Path:        path,
		Args:        []typ.Type{typ.Integer},
	}

	got := JoinEnvReturns([]contract.EnvReturnSpec{left}, []contract.EnvReturnSpec{right})
	if len(got) != 1 {
		t.Fatalf("JoinEnvReturns len = %d, want duplicate identity merged: %#v", len(got), got)
	}
	if len(got[0].Args) != 1 || !typ.TypeEquals(got[0].Args[0], typ.Integer) {
		t.Fatalf("merged args = %#v, want integer", got[0].Args)
	}
}
