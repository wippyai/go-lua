package transformer

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCanonicalEvaluatedRootAuthorityIgnoresConstructionOrderAndFingerprints(t *testing.T) {
	left, _, leftCursor, _ := evaluatedRootFixture(t)
	right, _, rightCursor, _ := evaluatedRootFixture(t)

	// Arena fingerprints are collision buckets only. Authority must be derived
	// from the exact owned structure, never from the compact bucket key.
	right.arena.fingerprintMask = 0
	for low, high := 0, len(right.rows)-1; low < high; low, high = low+1, high-1 {
		right.rows[low], right.rows[high] = right.rows[high], right.rows[low]
	}

	leftAuthority := mustDeriveEvaluatedRootAuthority(t, left, leftCursor, nil)
	rightAuthority := mustDeriveEvaluatedRootAuthority(t, right, rightCursor, nil)
	if leftAuthority != rightAuthority {
		t.Fatalf("equivalent relation authority changed with construction details:\nleft  %#v\nright %#v", leftAuthority, rightAuthority)
	}
}

func TestCanonicalEvaluatedRootAuthoritySeparatesRelationAndEntryDrift(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)
	base := mustDeriveEvaluatedRootAuthority(t, relation, cursor, nil)

	changedRelation := relation
	changedRelation.rows = append([]Row(nil), relation.rows...)
	changedRelation.rows[0] = relation.rows[0]
	changedRelation.rows[0].Guard = relation.arena.False()
	relationDrift := mustDeriveEvaluatedRootAuthority(t, changedRelation, cursor, nil)
	if relationDrift.relation == base.relation {
		t.Fatal("semantic row-output drift retained relation authority")
	}
	if relationDrift.entry != base.entry || relationDrift.registry != base.registry || relationDrift.lineage != base.lineage {
		t.Fatal("relation-only drift contaminated entry, registry, or lineage fences")
	}

	changedCursor, err := NewBindingCursor(relation.shape, []product.Value{typevalue.LiteralString(relation.arena.reg, "entry-drift")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entryDrift := mustDeriveEvaluatedRootAuthority(t, relation, changedCursor, nil)
	if entryDrift.entry == base.entry {
		t.Fatal("semantic binding drift retained entry authority")
	}
	if entryDrift.relation != base.relation || entryDrift.registry != base.registry || entryDrift.lineage != base.lineage {
		t.Fatal("entry-only drift contaminated relation, registry, or lineage fences")
	}
}

func TestCanonicalEntryAuthorityUsesConcretePathIdentity(t *testing.T) {
	relation, _, _, input := evaluatedRootFixture(t)
	derive := func(path pathdom.Path) EvaluatedRootAuthority {
		t.Helper()
		cursor, err := NewBindingCursor(relation.shape, []product.Value{input}, []pathdom.Path{path})
		if err != nil {
			t.Fatal(err)
		}
		return mustDeriveEvaluatedRootAuthority(t, relation, cursor, nil)
	}

	symbol := derive(pathdom.Path{Root: "display-a", Symbol: 17, Version: 3})
	if got := derive(pathdom.Path{Root: "display-b", Symbol: 17, Version: 3}); got.entry != symbol.entry {
		t.Fatal("display-only root changed a symbol-backed path identity")
	}
	if got := derive(pathdom.Path{Root: "display-a", Symbol: 17, Version: 4}); got.entry == symbol.entry {
		t.Fatal("symbol-backed SSA version drift retained entry authority")
	}

	placeholder := derive(pathdom.Path{Root: "$0", Version: 3})
	if got := derive(pathdom.Path{Root: "$0", Version: 99}); got.entry != placeholder.entry {
		t.Fatal("non-identity placeholder version changed entry authority")
	}
	if got := derive(pathdom.Path{Root: "$1", Version: 3}); got.entry == placeholder.entry {
		t.Fatal("placeholder root drift retained entry authority")
	}
}

func TestCanonicalLineageIsAnOrderIndependentDependencySet(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)
	first := mustDeriveEvaluatedRootAuthority(t, relation, cursor, nil)

	secondCursor, err := NewBindingCursor(relation.shape, []product.Value{typevalue.LiteralString(relation.arena.reg, "second")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second := mustDeriveEvaluatedRootAuthority(t, relation, secondCursor, nil)

	left := mustDeriveEvaluatedRootAuthority(t, relation, cursor, []EvaluatedRootAuthority{first, second, first})
	right := mustDeriveEvaluatedRootAuthority(t, relation, cursor, []EvaluatedRootAuthority{second, first})
	if left.lineage != right.lineage {
		t.Fatal("dependency permutation or duplication changed lineage authority")
	}
	if left.lineage == first.lineage {
		t.Fatal("nonempty dependency set retained empty lineage authority")
	}
}

func TestCanonicalEvaluatedRootAuthorityFailsTransactionally(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if authority, err := relation.DeriveEvaluatedRootAuthority(canceled, cursor, nil); !errors.Is(err, context.Canceled) || authority != (EvaluatedRootAuthority{}) {
		t.Fatalf("canceled derivation = %#v, %v; want zero and context.Canceled", authority, err)
	}

	callbackRelation := relation
	callbackRelation.rows = append([]Row(nil), relation.rows...)
	callbackRelation.rows[0] = relation.rows[0]
	callbackRelation.rows[0].Ops = append([]Operation(nil), relation.rows[0].Ops...)
	callbackRelation.rows[0].Ops[0].Value = callbackRelation.arena.CellResultValue(CellRef{Function: 7, Slot: 2})
	authority, err := callbackRelation.DeriveEvaluatedRootAuthority(context.Background(), cursor, nil)
	var nonportable *NonportableCanonicalRelationError
	if !errors.As(err, &nonportable) || authority != (EvaluatedRootAuthority{}) {
		t.Fatalf("callback relation derivation = %#v, %v; want typed nonportable zero", authority, err)
	}

	if authority, err = relation.DeriveEvaluatedRootAuthority(context.Background(), cursor, []EvaluatedRootAuthority{{}}); err == nil || authority != (EvaluatedRootAuthority{}) {
		t.Fatalf("invalid dependency derivation = %#v, %v; want transactional rejection", authority, err)
	}
}

func TestCanonicalEvaluatedRootAuthorityRejectsCyclesAndMalformedRoots(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(Relation) Relation
	}{
		{name: "value-cycle", mutate: func(relation Relation) Relation {
			cycle := ValueTerm(len(relation.arena.values))
			relation.arena.values = append(relation.arena.values, valueNode{op: valueJoin, args: []ValueTerm{cycle, relation.arena.Root(Root{Kind: RootParam})}})
			return relationWithFirstOperation(relation, cycle)
		}},
		{name: "guard-cycle", mutate: func(relation Relation) Relation {
			cycle := Guard(len(relation.arena.guards))
			relation.arena.guards = append(relation.arena.guards, guardNode{op: guardAnd, args: []Guard{cycle, relation.arena.True()}})
			relation.rows = append([]Row(nil), relation.rows...)
			relation.rows[0].Guard = cycle
			return relation
		}},
		{name: "root-outside-shape", mutate: func(relation Relation) Relation {
			return relationWithFirstOperation(relation, relation.arena.Root(Root{Kind: RootParam, Index: relation.shape.Params}))
		}},
		{name: "invalid-value-arity", mutate: func(relation Relation) Relation {
			malformed := ValueTerm(len(relation.arena.values))
			relation.arena.values = append(relation.arena.values, valueNode{op: valueStringConcat, args: []ValueTerm{relation.arena.Root(Root{Kind: RootParam})}})
			return relationWithFirstOperation(relation, malformed)
		}},
		{name: "invalid-guard-arity", mutate: func(relation Relation) Relation {
			malformed := Guard(len(relation.arena.guards))
			relation.arena.guards = append(relation.arena.guards, guardNode{op: guardAnd, args: []Guard{relation.arena.True()}})
			relation.rows = append([]Row(nil), relation.rows...)
			relation.rows[0].Guard = malformed
			return relation
		}},
		{name: "path-root-outside-shape", mutate: func(relation Relation) Relation {
			relation.rows = append([]Row(nil), relation.rows...)
			relation.rows[0].PathRefinements = []PathRefinementTerm{{
				Path:  relation.arena.Path(Root{Kind: RootParam, Index: relation.shape.Params}),
				Value: relation.arena.Root(Root{Kind: RootParam}),
			}}
			return relation
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relation, _, cursor, _ := evaluatedRootFixture(t)
			authority, err := test.mutate(relation).DeriveEvaluatedRootAuthority(context.Background(), cursor, nil)
			var nonportable *NonportableCanonicalRelationError
			if !errors.As(err, &nonportable) || authority != (EvaluatedRootAuthority{}) {
				t.Fatalf("malformed authority = %#v, %v; want typed nonportable zero", authority, err)
			}
		})
	}

	relation, _, cursor, _ := evaluatedRootFixture(t)
	cursor.values = nil // same-package adversary bypassing NewBindingCursor
	if authority, err := relation.DeriveEvaluatedRootAuthority(context.Background(), cursor, nil); err == nil || authority != (EvaluatedRootAuthority{}) {
		t.Fatalf("malformed binding cursor = %#v, %v; want zero rejection", authority, err)
	}
}

func TestCanonicalEvaluatedRootAuthorityAllowsSharedDAGs(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)
	root := relation.arena.Root(Root{Kind: RootParam})
	shared := relation.arena.StringConcatValue(root, root)
	relation = relationWithFirstOperation(relation, shared)
	relation.rows[0].Guard = relation.arena.And(relation.arena.Truthy(root), relation.arena.Falsy(root))
	if authority := mustDeriveEvaluatedRootAuthority(t, relation, cursor, nil); !authority.Valid() {
		t.Fatalf("shared acyclic DAG lost authority: %#v", authority)
	}
}

func TestCanonicalProjectionFragmentValueDriftChangesBytes(t *testing.T) {
	relation, _, _, _ := evaluatedRootFixture(t)
	codec := newRelationCanonicalCodec(context.Background(), relation)
	base := sparseProjectionFragment{guard: relation.arena.True()}
	left, err := codec.projectionFragmentBytes(base)
	if err != nil {
		t.Fatal(err)
	}
	base.values = []sparseProjectionValue{{index: 0, value: relation.arena.Root(Root{Kind: RootParam})}}
	right, err := codec.projectionFragmentBytes(base)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(left, right) {
		t.Fatal("sparse call-result fragment value drift retained canonical bytes")
	}
}

func TestCanonicalCallResultIdentityIncludesPointAndSlot(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)
	root := relation.arena.Root(Root{Kind: RootParam})
	derive := func(point cfg.Point, slot uint32) EvaluatedRootAuthority {
		t.Helper()
		changed := relationWithFirstOperation(relation, relation.arena.CallResultValue(point, slot, root))
		return mustDeriveEvaluatedRootAuthority(t, changed, cursor, nil)
	}
	base := derive(3, 0)
	for name, changed := range map[string]EvaluatedRootAuthority{
		"point": derive(4, 0),
		"slot":  derive(3, 1),
	} {
		if changed.relation == base.relation {
			t.Fatalf("%s drift retained call-result relation authority", name)
		}
		if changed.entry != base.entry || changed.registry != base.registry || changed.lineage != base.lineage {
			t.Fatalf("%s drift contaminated non-relation authority", name)
		}
	}
}

func TestCanonicalRelationValidationTraversesSparseFragmentValues(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)
	cycle := ValueTerm(len(relation.arena.values))
	relation.arena.values = append(relation.arena.values, valueNode{
		op: valueJoin, args: []ValueTerm{cycle, relation.arena.Root(Root{Kind: RootParam})},
	})
	relation.projectionTrace = cloneSparseProjectionTrace(relation.projectionTrace)
	mutated := false
	for slot := range relation.projectionTrace.slots {
		if len(relation.projectionTrace.slots[slot].fragments) == 0 {
			continue
		}
		relation.projectionTrace.slots[slot].fragments[0].values = []sparseProjectionValue{{index: 0, value: cycle}}
		mutated = true
		break
	}
	if !mutated {
		t.Fatal("fixture has no sparse projection fragment")
	}
	authority, err := relation.DeriveEvaluatedRootAuthority(context.Background(), cursor, nil)
	var nonportable *NonportableCanonicalRelationError
	if !errors.As(err, &nonportable) || authority != (EvaluatedRootAuthority{}) {
		t.Fatalf("cyclic sparse fragment value = %#v, %v; want typed nonportable zero", authority, err)
	}
}

func TestCanonicalAuthorityAccessorsDoNotExposeForgeableStorage(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)
	authority := mustDeriveEvaluatedRootAuthority(t, relation, cursor, nil)
	typ := reflect.TypeOf(authority)
	for index := 0; index < typ.NumField(); index++ {
		if typ.Field(index).PkgPath == "" {
			t.Fatalf("authority field %q is externally writable", typ.Field(index).Name)
		}
	}
	copyOfDigest := authority.RelationIdentity()
	copyOfDigest.Value[0] ^= 0xff
	if authority.RelationIdentity() == copyOfDigest || !authority.Valid() {
		t.Fatal("mutating an accessor copy changed private authority storage")
	}
}

func TestCanonicalEntryAuthorityFencesProductSchemaAndPayload(t *testing.T) {
	baseRegistry := standard.Registry()
	extraKey := axis.NewKey[int]("test.transformer.canonical.schema-drift")
	extraSpec := axis.Spec[int]{
		Key: extraKey, Bottom: func() int { return 0 }, Top: func() int { return 2 },
		Equal: func(a, b int) bool { return a == b }, LessOrEq: func(a, b int) bool { return a <= b },
		Join: func(a, b int) int { return max(a, b) }, Meet: func(a, b int) int { return min(a, b) },
		Hash:      func(value int) uint64 { return uint64(value) },
		Retention: axis.ImmutableRetention[int](), Boundary: axis.PortableIdentity,
		Canonical: axis.ReadyCanonical("test.transformer.canonical.schema-drift", 1, func(w *canonical.Writer, value int) error {
			return w.Int(int64(value))
		}),
	}
	extendedRegistry, err := standard.RegistryWithAxes(extraSpec.Erase())
	if err != nil {
		t.Fatal(err)
	}
	baseRegistryID, err := canonicalRegistryAuthority(baseRegistry)
	if err != nil {
		t.Fatal(err)
	}
	extendedRegistryID, err := canonicalRegistryAuthority(extendedRegistry)
	if err != nil {
		t.Fatal(err)
	}
	if baseRegistryID == extendedRegistryID {
		t.Fatal("axis schema drift retained registry authority")
	}
	shape := Shape{Params: 1}
	baseCursor, _ := NewBindingCursor(shape, []product.Value{product.Top()}, nil)
	extendedCursor, _ := NewBindingCursor(shape, []product.Value{product.Set(extendedRegistry, product.Top(), extraKey, 1)}, nil)
	baseEntry, err := canonicalEntryIdentity(context.Background(), baseRegistry, baseRegistryID, baseCursor)
	if err != nil {
		t.Fatal(err)
	}
	extendedEntry, err := canonicalEntryIdentity(context.Background(), extendedRegistry, extendedRegistryID, extendedCursor)
	if err != nil {
		t.Fatal(err)
	}
	if baseEntry == extendedEntry {
		t.Fatal("product schema/payload drift retained entry authority")
	}
}

func TestCanonicalRelationAuthorityCoversEveryAdmittedRelationField(t *testing.T) {
	type mutation struct {
		name   string
		mutate func(testing.TB, *Relation, *BindingCursor, product.Value)
	}
	mutations := []mutation{
		{name: "shape", mutate: func(t testing.TB, relation *Relation, cursor *BindingCursor, input product.Value) {
			relation.shape.Params++
			var err error
			*cursor, err = NewBindingCursor(relation.shape, []product.Value{input, input}, nil)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "descriptor-registry", mutate: func(t testing.TB, relation *Relation, _ *BindingCursor, _ product.Value) {
			var err error
			relation.descriptors, err = newCompilerDescriptorRegistry([]product.Value{product.Top()})
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "output-authority", mutate: func(testing.TB, *Relation, *BindingCursor, product.Value) {}},
		{name: "rows", mutate: func(_ testing.TB, relation *Relation, _ *BindingCursor, _ product.Value) {
			relation.rows = append([]Row(nil), relation.rows...)
			relation.rows[0].Guard = relation.arena.False()
		}},
		{name: "return-correlation-policy", mutate: func(_ testing.TB, relation *Relation, _ *BindingCursor, _ product.Value) {
			relation.inferReturnCorrelations = !relation.inferReturnCorrelations
		}},
		{name: "widened", mutate: func(_ testing.TB, relation *Relation, _ *BindingCursor, _ product.Value) {
			relation.widened = !relation.widened
		}},
		{name: "projection-trace", mutate: func(_ testing.TB, relation *Relation, _ *BindingCursor, _ product.Value) {
			relation.projectionTrace = cloneSparseProjectionTrace(relation.projectionTrace)
			relation.projectionTrace.schema[0] ^= 1
		}},
		{name: "parameter-contracts", mutate: func(_ testing.TB, relation *Relation, _ *BindingCursor, _ product.Value) {
			relation.paramContracts = []product.Value{product.Top()}
		}},
		{name: "relation-projection", mutate: func(t testing.TB, relation *Relation, _ *BindingCursor, _ product.Value) {
			relation.projection = normalizeRelationProjection(relation.arena.reg, []summary.ReturnParamPathAlias{relationProjectionTestAlias(t, 0, 0)})
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			relation, _, cursor, input := evaluatedRootFixture(t)
			base := mustDeriveEvaluatedRootAuthority(t, relation, cursor, nil)
			if test.name == "output-authority" {
				relation.authority = &relationOutputAuthority{}
			} else {
				test.mutate(t, &relation, &cursor, input)
			}
			changed := mustDeriveEvaluatedRootAuthority(t, relation, cursor, nil)
			if changed.relation == base.relation {
				t.Fatalf("admitted Relation.%s drift retained relation authority", test.name)
			}
		})
	}
	// arena/effects pointers are storage owners, not semantic fields: arena
	// structure is covered by term bytes, while any populated effect term is
	// outside the admitted evaluated-root subset and fails closed.
}

func TestCanonicalEvaluatedRootAuthorityCancelsMidTraversal(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)
	ctx := &cancelAfterChecksContext{remaining: 4}
	authority, err := relation.DeriveEvaluatedRootAuthority(ctx, cursor, nil)
	if !errors.Is(err, context.Canceled) || authority != (EvaluatedRootAuthority{}) || ctx.remaining > 0 {
		t.Fatalf("mid-flight cancellation = %#v, %v (remaining %d); want canceled zero", authority, err, ctx.remaining)
	}
}

func relationWithFirstOperation(relation Relation, value ValueTerm) Relation {
	relation.rows = append([]Row(nil), relation.rows...)
	relation.rows[0].Ops = append([]Operation(nil), relation.rows[0].Ops...)
	relation.rows[0].Ops[0].Value = value
	return relation
}

func mustDeriveEvaluatedRootAuthority(t testing.TB, relation Relation, cursor BindingCursor, dependencies []EvaluatedRootAuthority) EvaluatedRootAuthority {
	t.Helper()
	authority, err := relation.DeriveEvaluatedRootAuthority(context.Background(), cursor, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if !authority.Valid() {
		t.Fatalf("derived invalid authority %#v", authority)
	}
	return authority
}
