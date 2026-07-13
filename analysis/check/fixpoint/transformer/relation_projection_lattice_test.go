package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func relationProjectionTestAlias(t testing.TB, returnIndex, param int) summary.ReturnParamPathAlias {
	t.Helper()
	source, ok := pathaddr.PlaceholderKeyFromPath(pathdom.NewPlaceholder(param))
	if !ok {
		t.Fatalf("placeholder path for parameter %d is not addressable", param)
	}
	return summary.ReturnParamPathAlias{ReturnIndex: returnIndex, Source: source}
}

func TestRelationProjectionSurvivesLatticeComposition(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
	arena := builder.Arena()
	value := arena.Constant(typevalue.LiteralString(reg, "value"))
	base, err := builder.Build(certificate, []Row{{
		Guard: arena.True(),
		Ops:   []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: value}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	alias0 := relationProjectionTestAlias(t, 0, 0)
	alias1 := relationProjectionTestAlias(t, 0, 1)
	left := base
	left.projection = normalizeRelationProjection(reg, []summary.ReturnParamPathAlias{alias0, alias0})
	right := base
	right.projection = normalizeRelationProjection(reg, []summary.ReturnParamPathAlias{alias1})

	bottom := Relation{shape: base.shape, arena: base.arena}
	for name, adopted := range map[string]Relation{
		"bottom-left":  JoinRelation(bottom, left),
		"bottom-right": JoinRelation(left, bottom),
	} {
		if adopted.ContextualReason() != "" || !EqualRelation(adopted, left) {
			t.Fatalf("%s adoption lost projection: reason=%q projection=%#v", name, adopted.ContextualReason(), adopted.projection)
		}
	}

	joined := JoinRelation(left, right)
	wantProjection := normalizeRelationProjection(reg, []summary.ReturnParamPathAlias{alias0, alias1})
	if !equalRelationProjection(reg, joined.projection, wantProjection) {
		t.Fatalf("joined projection = %#v, want %#v", joined.projection, wantProjection)
	}
	if len(joined.projection.returnParamPathAliases) != 2 {
		t.Fatalf("joined projection retained duplicate aliases: %#v", joined.projection.returnParamPathAliases)
	}
	converged := JoinRelation(joined, left)
	if !EqualRelation(converged, joined) {
		t.Fatalf("projection prevented idempotent convergence: got %#v want %#v", converged.projection, joined.projection)
	}
	withoutProjection := joined
	withoutProjection.projection = relationProjection{}
	if EqualRelation(withoutProjection, joined) {
		t.Fatal("EqualRelation ignored projection metadata")
	}
}

func TestRelationProjectionSpecializationAddsAliasWithoutReturnFlow(t *testing.T) {
	reg := standard.Registry()
	caps := DefaultOutputCapabilityRegistry()
	for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
		if err := caps.SetSummary("ReturnParamPathAliases", lane, CapabilitySupported); err != nil {
			t.Fatal(err)
		}
	}
	builder, certificate := emptyBuilder(t, reg, Shape{Params: 1}, caps)
	arena := builder.Arena()
	param := arena.Root(Root{Kind: RootParam, Index: 0})
	refined := arena.runtimeValidationValue(param, typevalue.FromType(reg, typ.String))
	rows := []Row{{
		Guard: arena.True(),
		Ops:   []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: refined}},
	}}
	relation, err := builder.Build(certificate, rows)
	if err != nil {
		t.Fatal(err)
	}
	aliases, exact := projectedReturnParamAliases(arena, rows)
	if !exact || len(aliases) != 1 {
		t.Fatalf("structural alias projection = %#v/%v, want one exact alias", aliases, exact)
	}
	// Include the same alias twice to prove specialization normalizes the
	// relation-level projection together with the feasible row-local alias.
	relation.projection = normalizeRelationProjection(reg, append(aliases, aliases...))

	argument := typevalue.LiteralString(reg, "argument")
	cursor, err := NewBindingCursor(relation.Shape(), []product.Value{argument}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := relation.Specialize(cursor, nil, nil)
	if !ok {
		t.Fatal("structural alias specialization fell back")
	}
	if len(got.ReturnParamPathAliases) != 1 || got.ReturnParamPathAliases[0] != aliases[0] {
		t.Fatalf("specialized aliases = %#v, want deduplicated %#v", got.ReturnParamPathAliases, aliases)
	}
	if len(got.ReturnFlows) != 0 {
		t.Fatalf("refined structural alias manufactured ReturnFlows: %#v", got.ReturnFlows)
	}
	if len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], argument) {
		t.Fatalf("specialized return = %#v, want original caller argument", got.Returns)
	}
}
