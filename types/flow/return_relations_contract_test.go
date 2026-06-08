package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnRelationsFromSpec(t *testing.T) {
	rel := ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}

	tests := []struct {
		name string
		spec *contract.Spec
		want ReturnRelations
	}{
		{
			name: "nil spec is top",
			spec: nil,
			want: ReturnRelationsDomain.Top(),
		},
		{
			name: "empty spec is top",
			spec: contract.NewSpec(),
			want: ReturnRelationsDomain.Top(),
		},
		{
			name: "error return label becomes finite relation",
			spec: contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: rel.ValueIndex, ErrorIndex: rel.ErrorIndex}),
			want: ReturnRelationsOfErrorReturns([]ReturnCorrelation{rel}),
		},
		{
			name: "length proof does not enter return relations",
			spec: contract.NewSpec().WithEffects(
				effect.ErrorReturn{ValueIndex: rel.ValueIndex, ErrorIndex: rel.ErrorIndex},
				effect.ReturnLength{ReturnIndex: 0, Length: constraint.PL(1)},
			),
			want: ReturnRelationsOfErrorReturns([]ReturnCorrelation{rel}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReturnRelationsFromSpec(tt.spec)
			if !ReturnRelationsDomain.Equal(got, tt.want) {
				t.Fatalf("ReturnRelationsFromSpec() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBoundaryFactsFromSpecProjectsLengthRelations(t *testing.T) {
	common := BoundaryLengthRelationFact{
		Target: BoundaryPath{Kind: BoundaryPathReturn, Index: 0},
		Source: BoundaryPath{Kind: BoundaryPathParam, Index: 1},
	}
	other := BoundaryLengthRelationFact{
		Target: BoundaryPath{Kind: BoundaryPathReturn, Index: 2},
		Source: BoundaryPath{Kind: BoundaryPathParam, Index: 0},
	}
	spec := contract.NewSpec().WithEffects(effect.ReturnLength{ReturnIndex: common.Target.Index, Length: constraint.PL(common.Source.Index)})
	spec.ExprEnsures = append(spec.ExprEnsures, constraint.GeExpr(constraint.RL(other.Target.Index), constraint.PL(other.Source.Index)))

	got := BoundaryFactsFromSpec(spec)
	if !got.HasLengthRelation(common) || !got.HasLengthRelation(other) {
		t.Fatalf("BoundaryFactsFromSpec() = %#v, want length relations %#v and %#v", got, common, other)
	}
	if !got.HasProof() {
		t.Fatalf("boundary length relations should be finite proof")
	}
}

func TestPointRelationsLengthParamJoinAndKill(t *testing.T) {
	root := cfg.SymbolID(10)
	otherRoot := cfg.SymbolID(11)
	target, ok := LocalAddressOfPath(constraint.Path{Symbol: root, Version: 1})
	if !ok {
		t.Fatalf("local address for root %d was not produced", root)
	}
	otherTarget, ok := LocalAddressOfPath(constraint.Path{Symbol: otherRoot, Version: 1})
	if !ok {
		t.Fatalf("local address for root %d was not produced", otherRoot)
	}

	a := PointRelations{}.
		WithTargetLengthParamLocal(target, 0).
		WithTargetLengthParamLocal(otherTarget, 1)
	b := PointRelations{}.WithTargetLengthParamLocal(target, 0)

	got := PointRelationsDomain.Join(a, b)
	if !got.HasTargetLengthParamLocal(target, 0) {
		t.Fatalf("joined point relations lost common length-param proof: %#v", got)
	}
	if got.HasTargetLengthParamLocal(otherTarget, 1) {
		t.Fatalf("joined point relations kept non-must length-param proof: %#v", got)
	}

	errSym := cfg.SymbolID(12)
	valueSym := cfg.SymbolID(13)
	withSibling := got.WithSiblingNil(errSym, []cfg.SymbolID{valueSym})
	killedLength := withSibling.KillLengthTargets(root)
	if killedLength.HasTargetLengthParamLocal(target, 0) {
		t.Fatalf("KillLengthTargets kept stale length-param proof: %#v", killedLength)
	}
	if _, ok := killedLength.SiblingNil(errSym); !ok {
		t.Fatalf("KillLengthTargets removed unrelated sibling-nil proof: %#v", killedLength)
	}

	killedSymbol := withSibling.KillSymbols(root)
	if killedSymbol.HasTargetLengthParamLocal(target, 0) {
		t.Fatalf("KillSymbols kept stale target-length proof: %#v", killedSymbol)
	}
}

func TestPointRelationsContainerLowerBoundJoinAndKill(t *testing.T) {
	root := cfg.SymbolID(20)
	otherRoot := cfg.SymbolID(21)
	key := SymbolPathKey(root, nil)
	otherKey := SymbolPathKey(otherRoot, nil)

	a := PointRelations{}.
		WithContainerLowerBound(root, key, 3).
		WithContainerLowerBound(otherRoot, otherKey, 7)
	b := PointRelations{}.WithContainerLowerBound(root, key, 1)

	got := PointRelationsDomain.Join(a, b)
	if !got.HasContainerLowerBound(root, key, 1) {
		t.Fatalf("joined point relations lost common cardinality lower bound: %#v", got)
	}
	if got.HasContainerLowerBound(root, key, 2) {
		t.Fatalf("joined point relations used max instead of must/min lower bound: %#v", got)
	}
	if got.HasContainerLowerBound(otherRoot, otherKey, 1) {
		t.Fatalf("joined point relations kept non-must cardinality proof: %#v", got)
	}

	errSym := cfg.SymbolID(22)
	valueSym := cfg.SymbolID(23)
	withSibling := got.WithSiblingNil(errSym, []cfg.SymbolID{valueSym})
	killedLength := withSibling.KillLengthTargets(root)
	if killedLength.HasContainerLowerBound(root, key, 1) {
		t.Fatalf("KillLengthTargets kept stale cardinality proof: %#v", killedLength)
	}
	if _, ok := killedLength.SiblingNil(errSym); !ok {
		t.Fatalf("KillLengthTargets removed unrelated sibling-nil proof: %#v", killedLength)
	}

	killedSymbol := withSibling.KillSymbols(root)
	if killedSymbol.HasContainerLowerBound(root, key, 1) {
		t.Fatalf("KillSymbols kept stale cardinality proof: %#v", killedSymbol)
	}
}

func TestReturnRelationsFromFunctionType(t *testing.T) {
	rel01 := ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}
	rel10 := ReturnCorrelation{ValueIndex: 1, ErrorIndex: 0}
	fnWithRel := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: rel01.ValueIndex, ErrorIndex: rel01.ErrorIndex})).
		Build()
	fnWithSameRel := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: rel01.ValueIndex, ErrorIndex: rel01.ErrorIndex})).
		Build()
	fnWithOtherRel := typ.Func().
		Returns(typ.NewOptional(typ.Number), typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: rel10.ValueIndex, ErrorIndex: rel10.ErrorIndex})).
		Build()
	fnWithoutRel := typ.Func().Returns(typ.String).Build()

	tests := []struct {
		name   string
		fnType typ.Type
		want   ReturnRelations
	}{
		{
			name:   "nil type is top",
			fnType: nil,
			want:   ReturnRelationsDomain.Top(),
		},
		{
			name:   "non function type is top",
			fnType: typ.String,
			want:   ReturnRelationsDomain.Top(),
		},
		{
			name:   "function contract projects relation",
			fnType: fnWithRel,
			want:   ReturnRelationsOfErrorReturns([]ReturnCorrelation{rel01}),
		},
		{
			name:   "optional function projects through non nil function",
			fnType: typ.NewOptional(fnWithRel),
			want:   ReturnRelationsOfErrorReturns([]ReturnCorrelation{rel01}),
		},
		{
			name:   "union of functions keeps relations proven by every member",
			fnType: typ.NewUnion(fnWithRel, fnWithSameRel, typ.Nil),
			want:   ReturnRelationsOfErrorReturns([]ReturnCorrelation{rel01}),
		},
		{
			name:   "union of disagreeing function relations is top",
			fnType: typ.NewUnion(fnWithRel, fnWithOtherRel, typ.Nil),
			want:   ReturnRelationsDomain.Top(),
		},
		{
			name:   "union member without relation makes proof top",
			fnType: typ.NewUnion(fnWithRel, fnWithoutRel, typ.Nil),
			want:   ReturnRelationsDomain.Top(),
		},
		{
			name:   "union with non function member is top",
			fnType: typ.NewUnion(fnWithRel, typ.String, typ.Nil),
			want:   ReturnRelationsDomain.Top(),
		},
		{
			name:   "nil only is top",
			fnType: typ.Nil,
			want:   ReturnRelationsDomain.Top(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReturnRelationsFromFunctionType(tt.fnType)
			if !ReturnRelationsDomain.Equal(got, tt.want) {
				t.Fatalf("ReturnRelationsFromFunctionType() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
