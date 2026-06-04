package value

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestUnsafePrecisionDrop_DetectsNestedUnionMemberDrop(t *testing.T) {
	withPending := typ.NewUnion(
		typ.LiteralString("pass"),
		typ.LiteralString("pending"),
		typ.LiteralString("fail"),
		typ.LiteralString("skip"),
	)
	withoutPending := typ.NewUnion(
		typ.LiteralString("pass"),
		typ.LiteralString("fail"),
		typ.LiteralString("skip"),
	)
	prev := typ.NewRecord().Field("status", withPending).Build()
	next := typ.NewRecord().Field("status", withoutPending).Build()
	if !UnsafePrecisionDrop(prev, next) {
		t.Fatalf("expected nested union member drop to be unsafe: prev=%v next=%v", prev, next)
	}
}

func TestUnsafePrecisionDrop_AllowsSoftRecordFieldRefinement(t *testing.T) {
	prev := typ.NewOptional(typ.NewRecord().
		Field("max_tokens", typ.Any).
		Field("output_tokens", typ.Any).
		Build())
	next := typ.NewOptional(typ.NewRecord().
		Field("max_tokens", typ.Integer).
		Field("output_tokens", typ.Integer).
		Build())

	if UnsafePrecisionDrop(prev, next) {
		t.Fatalf("soft field refinement should not be treated as precision loss: prev=%v next=%v", prev, next)
	}
}

func TestMergeForConvergence_UnknownIsUnresolvedEvidence(t *testing.T) {
	next := typ.NewArray(typ.Unknown)
	got := MergeForConvergence(typ.Unknown, next)
	if !typ.TypeEquals(got, next) {
		t.Fatalf("MergeForConvergence(unknown, unknown[]) = %v, want %v", got, next)
	}

	got = MergeForConvergence(next, typ.Unknown)
	if !typ.TypeEquals(got, next) {
		t.Fatalf("MergeForConvergence(unknown[], unknown) = %v, want %v", got, next)
	}
}

func TestMergeForConvergence_RefinesInstantiatedGenericArgumentEvidence(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewRecord().Build())
	seed := typ.Instantiate(channel, typ.Unknown)
	refined := typ.Instantiate(channel, typ.String)

	for _, tc := range []struct {
		name string
		a    typ.Type
		b    typ.Type
	}{
		{name: "forward", a: seed, b: refined},
		{name: "reverse", a: refined, b: seed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MergeForConvergence(tc.a, tc.b)
			if !typ.TypeEquals(got, refined) {
				t.Fatalf("MergeForConvergence(%v, %v) = %v, want %v", tc.a, tc.b, got, refined)
			}
		})
	}
}

func TestJoinRecordShape_UsesSlotJoinForMetatableEvidence(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	unresolved := typ.NewRecord().
		Metatable(typ.Unknown).
		SetOpen(true).
		Build()
	resolved := typ.NewRecord().
		Metatable(metatable).
		SetOpen(true).
		Build()

	got, ok := JoinRecordShape(unresolved, resolved, MergeForConvergence)
	if !ok {
		t.Fatalf("JoinRecordShape(unresolved metatable, resolved metatable) ok=false")
	}
	if mt, ok := querycore.Method(got, "ready"); !ok {
		t.Fatalf("joined metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got)
	}
}

func TestMergeForConvergence_RefinesUnknownMetatableEvidence(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	unresolved := typ.NewRecord().
		Metatable(typ.Unknown).
		SetOpen(true).
		Build()
	resolved := typ.NewRecord().
		Metatable(metatable).
		SetOpen(true).
		Build()

	for _, tc := range []struct {
		name string
		a    typ.Type
		b    typ.Type
	}{
		{name: "forward", a: unresolved, b: resolved},
		{name: "reverse", a: resolved, b: unresolved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MergeForConvergence(tc.a, tc.b)
			if mt, ok := querycore.Method(got, "ready"); !ok {
				t.Fatalf("merged metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got)
			}
		})
	}
}

func TestMergeForConvergence_ReplacesEmptyRecordSeedWithRecordExtension(t *testing.T) {
	seed := typ.NewRecord().Build()
	observed := typ.NewRecord().
		Field("ready", typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()).
		Build()

	for _, tc := range []struct {
		name string
		a    typ.Type
		b    typ.Type
	}{
		{name: "forward", a: seed, b: observed},
		{name: "reverse", a: observed, b: seed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MergeForConvergence(tc.a, tc.b)
			if field, ok := querycore.Field(got, "ready"); !ok || typ.IsAbsentOrUnknown(field) {
				t.Fatalf("merged record extension ready = %v ok=%v, want field on %v", field, ok, got)
			}
		})
	}
}

func TestJoinRecordShape_DoesNotInventMetatableWhenAbsentOnOneBranch(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	plain := typ.NewRecord().Build()
	withMetatable := typ.NewRecord().Metatable(metatable).Build()

	got, ok := JoinRecordShape(plain, withMetatable, MergeForConvergence)
	if !ok {
		t.Fatalf("JoinRecordShape(plain, metatabled) ok=false")
	}
	if mt, ok := querycore.Method(got, "ready"); ok {
		t.Fatalf("joined shape invented metatable method ready = %v on %v", mt, got)
	}
}

func TestMergeForConvergence_ReplacesUnsolvedFunctionSeed(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()

	got := MergeForConvergence(seed, solved)
	if !typ.TypeEquals(got, solved) {
		t.Fatalf("MergeForConvergence(seed, solved) = %v, want %v", got, solved)
	}

	got = MergeForConvergence(solved, seed)
	if !typ.TypeEquals(got, solved) {
		t.Fatalf("MergeForConvergence(solved, seed) = %v, want %v", got, solved)
	}
}

func TestJoinPrecise_ReplacesUnsolvedFunctionSeed(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()

	got := JoinPrecise(seed, solved)
	if !typ.TypeEquals(got, solved) {
		t.Fatalf("JoinPrecise(seed, solved) = %v, want %v", got, solved)
	}

	got = JoinPrecise(solved, seed)
	if !typ.TypeEquals(got, solved) {
		t.Fatalf("JoinPrecise(solved, seed) = %v, want %v", got, solved)
	}
}

func TestMergeForConvergence_ReplacesUnsolvedFunctionSeedInsideRecordField(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	existing := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", seed).
		Build()
	candidate := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", solved).
		Build()

	got := MergeForConvergence(existing, candidate)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("MergeForConvergence(record seed, record solved) = %T %[1]v, want record", got)
	}
	field := rec.GetField("get_x")
	if field == nil || !typ.TypeEquals(field.Type, solved) {
		t.Fatalf("merged get_x = %v, want %v", field, solved)
	}

	got = MergeForConvergence(candidate, existing)
	rec, ok = got.(*typ.Record)
	if !ok {
		t.Fatalf("MergeForConvergence(record solved, record seed) = %T %[1]v, want record", got)
	}
	field = rec.GetField("get_x")
	if field == nil || !typ.TypeEquals(field.Type, solved) {
		t.Fatalf("reverse merged get_x = %v, want %v", field, solved)
	}
}

func TestUnsafePrecisionDrop_AllowsSolvedFunctionSeedReplacement(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	if UnsafePrecisionDrop(seed, solved) {
		t.Fatalf("solved function projection must not be treated as dropping unsolved seed evidence")
	}

	prev := typ.NewRecord().Field("get_x", seed).Build()
	merged := typ.NewRecord().Field("get_x", solved).Build()
	if UnsafePrecisionDrop(prev, merged) {
		t.Fatalf("record field solved function projection must not be treated as dropping seed evidence")
	}
}

func TestArrayEvidenceShape_UsesIdentityCycleCheckOnly(t *testing.T) {
	if got := arrayEvidenceShape(panicHashEvidenceType{}); got != nil {
		t.Fatalf("arrayEvidenceShape(non-wrapper) = %v, want nil", got)
	}
}

type panicHashEvidenceType struct{}

func (panicHashEvidenceType) Kind() kind.Kind { return kind.String }
func (panicHashEvidenceType) String() string  { return "panic-hash" }
func (panicHashEvidenceType) Hash() uint64 {
	panic("array evidence shape must not hash structural products")
}
func (panicHashEvidenceType) Equals(typ.Type) bool { return false }

// metatableCycleTower builds a finite unfolding of a Lua class record whose
// __index metatable field re-embeds the class itself (Bus.__index = Bus). Each
// __index level carries the full class layout, so successive depths are
// self-similar towers of one cyclic family - exactly the shapes the
// inter-procedural fixpoint produces when it merges successive observations of
// the structurally-cyclic class. The innermost __index bottoms at an unknown
// seed, the placeholder the recursion grows out of.
func metatableCycleTower(depth int) typ.Type {
	var inner typ.Type = typ.Unknown
	for i := 0; i <= depth; i++ {
		inner = classLayout(inner)
	}
	return inner
}

func classLayout(index typ.Type) typ.Type {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Nil).Build()
	return typ.NewRecord().
		Field("__index", index).
		Field("new", typ.Func().Returns(typ.NewRecord().Field("pending_ops", typ.Number).Build()).Build()).
		Field("pending_ops", typ.Number).
		Field("run", method).
		Field("stopping", typ.Boolean).
		Build()
}

// selfViewTower builds the `self` view of the cyclic class: a record carrying
// only the instance fields whose __index re-embeds the full class layout one or
// more levels deep. This mirrors the asymmetric observations the fixpoint merges
// (the self metatable shape vs the class shape), where each merge unfolds the
// __index tower one level deeper.
func selfViewTower(depth int) typ.Type {
	return typ.NewRecord().
		Field("__index", metatableCycleTower(depth)).
		Field("pending_ops", typ.Number).
		Field("stopping", typ.Boolean).
		Build()
}

// TestMergeForConvergence_FoldsMetatableSelfCycle proves the self-embedding
// merge folds a structurally-cyclic class record (Bus.__index = Bus) into a
// finite recursive upper bound instead of unfolding the __index tower one level
// deeper on every merge. Without the fold the inter-procedural fixpoint never
// converges.
func TestMergeForConvergence_FoldsMetatableSelfCycle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tower func(int) typ.Type
	}{
		{name: "class-view", tower: metatableCycleTower},
		{name: "self-view", tower: selfViewTower},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shallow := tc.tower(2)
			deeper := tc.tower(3)

			got := MergeForConvergence(shallow, deeper)
			if !typ.ContainsRecursive(got) {
				t.Fatalf("MergeForConvergence(tower2, tower3) = %v, want a recursive upper bound (mu) bounding the __index cycle", got)
			}
			gotDepth := indexNestingDepth(got)

			// Merging the folded family with a still-deeper observation must stay
			// finite: the result reuses the recursive representative instead of
			// unfolding the __index tower one level deeper on every merge, which is
			// what makes the inter-procedural fixpoint converge.
			again := MergeForConvergence(got, tc.tower(4))
			if !typ.ContainsRecursive(again) {
				t.Fatalf("re-merge with deeper observation = %v, want stable recursive family", again)
			}
			if indexNestingDepth(again) > gotDepth {
				t.Fatalf("re-merge grew __index nesting depth from %d to %d, want bounded", gotDepth, indexNestingDepth(again))
			}
		})
	}
}

// indexNestingDepth counts how many nested __index record levels t carries
// before reaching a non-record or a recursive back-edge. It bounds the tower
// growth a self-embedding fold must eliminate.
func indexNestingDepth(t typ.Type) int {
	depth := 0
	for {
		rec, ok := UnwrapStructuralShape(t).(*typ.Record)
		if !ok || rec == nil {
			return depth
		}
		field := rec.GetField("__index")
		if field == nil {
			return depth
		}
		depth++
		t = field.Type
	}
}

func TestMergeForConvergence_AnyIsDynamicTop(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	got := MergeForConvergence(rec, typ.Any)
	if !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("MergeForConvergence(record, any) = %v, want any", got)
	}

	got = MergeForConvergence(typ.Any, rec)
	if !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("MergeForConvergence(any, record) = %v, want any", got)
	}
}

func TestNormalizeFactType_CollapsesCompatibleRecordUnion(t *testing.T) {
	members := make([]typ.Type, 0, 128)
	for i := 0; i < cap(members); i++ {
		members = append(members, typ.NewRecord().
			Field("name", typ.LiteralString("suite")).
			Field("index", typ.LiteralInt(int64(i))).
			Build())
	}

	got := NormalizeFactType(typ.NewUnion(members...))
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("NormalizeFactType(record family) = %T %[1]v, want record", got)
	}
	index := rec.GetField("index")
	if index == nil || !typ.TypeEquals(index.Type, typ.Integer) {
		t.Fatalf("normalized index field = %v, want integer", index)
	}
}

func TestJoinPrecise_UsesCanonicalRecursiveProductJoin(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("proc", typ.Any).
			Build()
	})

	got := JoinPrecise(left, right)
	if _, ok := got.(*typ.Union); ok {
		t.Fatalf("JoinPrecise returned raw recursive union: %v", got)
	}
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("JoinPrecise = %T %[1]v, want recursive product", got)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	proc := body.GetField("proc")
	if proc == nil || !proc.Optional {
		t.Fatalf("merged recursive body should retain optional proc field, got %v", body)
	}
}

func TestMergeForConvergence_RecursiveProductJoinKeepsBranchFieldsOptional(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("proc", typ.Any).
			Build()
	})

	got := MergeForConvergence(left, right)
	if _, ok := got.(*typ.Union); ok {
		t.Fatalf("MergeForConvergence returned raw recursive union: %v", got)
	}
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("MergeForConvergence = %T %[1]v, want recursive product", got)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	proc := body.GetField("proc")
	if proc == nil || !proc.Optional {
		t.Fatalf("merged recursive body should retain optional proc field, got %v", body)
	}
}

func TestMergeForConvergence_RecursiveUnionAdmitsCoveredObservation(t *testing.T) {
	suite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})
	covered := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(suite)).
		Field("full_path", typ.String).
		Build()
	existing := typ.NewUnion(suite, typ.Boolean)

	got := MergeForConvergence(existing, covered)
	if !typ.SameNode(got, existing) {
		t.Fatalf("covered recursive observation should be admitted by existing union: got %T %[1]v, want existing %v", got, existing)
	}
}

func TestMergeForConvergence_EqualRecursiveProductsUseConvergenceIdentity(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})

	got := MergeForConvergence(left, right)
	if !typ.SameNode(got, left) {
		t.Fatalf("MergeForConvergence(equal recursive products) = %T %[1]v, want existing node", got)
	}
}

func TestSameConvergedFact_RecursiveProductFamily(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	refined := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	if !SameConvergedFact(left, right) {
		t.Fatalf("SameConvergedFact(equivalent recursive families) = false")
	}
	if SameConvergedFact(left, refined) {
		t.Fatalf("SameConvergedFact(strict recursive refinement) = true")
	}
}

func TestSameConvergedFact_RecursiveUnionFamily(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	refined := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	leftUnion := typ.NewUnion(left, typ.Boolean)
	rightUnion := typ.NewUnion(right, typ.Boolean)
	refinedUnion := typ.NewUnion(refined, typ.Boolean)
	if !SameConvergedFact(leftUnion, rightUnion) {
		t.Fatalf("SameConvergedFact(equivalent recursive union families) = false")
	}
	if SameConvergedFact(leftUnion, refinedUnion) {
		t.Fatalf("SameConvergedFact(strict recursive union refinement) = true")
	}
}

func TestConvergenceWidening_ReusesRecursiveProductFamily(t *testing.T) {
	widening := NewConvergenceWidening()
	acc := recursiveBuilderFact()
	for i := 0; i < 24; i++ {
		next := recursiveBuilderFact()
		acc = widening.Merge(acc, next)
		if !SameConvergedFact(acc, next) {
			t.Fatalf("merge %d left recursive builder family: got %T %[1]v", i, acc)
		}
	}
}

func TestContainsEquivalent_RecursiveRootFamilyShortCircuits(t *testing.T) {
	left := recursiveBuilderFact()
	right := recursiveBuilderFact()
	if !ContainsEquivalent(left, right) {
		t.Fatalf("equivalent recursive product roots should be admitted without descendant growth")
	}
}

func recursiveBuilderFact() typ.Type {
	return typ.NewRecursive("Builder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("value", typ.Number).
			Field("add", typ.Func().
				Param("self", self).
				Param("n", typ.Number).
				Returns(self).
				Build()).
			Field("result", typ.Func().
				Param("self", self).
				Returns(typ.Number).
				Build()).
			Build()
	})
}

func TestPrecisionEvidenceUpperBound_RecursiveProductsDoNotBypassStructuralJoin(t *testing.T) {
	baseline := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	candidate := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	got, ok := PrecisionEvidenceUpperBound(candidate, baseline)
	if ok {
		t.Fatalf("PrecisionEvidenceUpperBound(recursive branch evidence) = %T %[1]v, want structural join to own optional branch fields", got)
	}
}

func TestPrecisionEvidenceUpperBound_RecursiveSoftContainerAvoidsGenericEquality(t *testing.T) {
	baseline := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("children", typ.NewArray(self)).
			Field("metadata", typ.Any).
			Build()
	})
	candidate := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("children", typ.NewArray(self)).
			Field("metadata", typ.String).
			Build()
	})

	got, ok := PrecisionEvidenceUpperBound(candidate, baseline)
	if !ok || !typ.SameNode(got, candidate) {
		t.Fatalf("PrecisionEvidenceUpperBound(recursive soft slot) = %T %[1]v ok=%v, want candidate", got, ok)
	}
}

func TestMergeForConvergence_TopArrayAbsorbsSelfEmbeddingGrowth(t *testing.T) {
	base := typ.NewArray(typ.Unknown)
	nested := typ.NewArray(base)

	got := MergeForConvergence(base, nested)
	if !typ.TypeEquals(got, base) {
		t.Fatalf("MergeForConvergence(unknown[], unknown[][]) = %v, want %v", got, base)
	}
}

func TestSelfEmbeddingUpperBound_TopArrayCoversNestedUnknownArray(t *testing.T) {
	base := typ.NewArray(typ.Unknown)
	nested := typ.NewArray(base)

	got, ok := SelfEmbeddingUpperBound(base, nested)
	if !ok || !typ.TypeEquals(got, base) {
		t.Fatalf("SelfEmbeddingUpperBound(unknown[], unknown[][]) = %v ok=%v, want %v", got, ok, base)
	}
}

func TestWidenForConvergence_StableOptionalMapRecordIsIdempotent(t *testing.T) {
	record := typ.NewRecord().
		MapComponent(typ.String, typ.Any).
		SetOpen(true).
		Build()
	stable := typ.NewOptional(record)

	got := WidenForConvergence(stable)
	if !typ.TypeEquals(got, stable) {
		t.Fatalf("stable optional record widened to %T %[1]v, want %v", got, stable)
	}
}

func TestMergeForConvergence_ConcreteArraySelfEmbeddingBoundsSoundly(t *testing.T) {
	elem := typ.NewRecord().Field("id", typ.String).Build()
	base := typ.NewArray(elem)
	nested := typ.NewArray(base)

	got := MergeForConvergence(base, nested)

	// A bare sequence has no slot for a genuine recursive reference: folding the
	// depth tower into a recursive family yields mu X.X[], an infinitely-nested
	// sequence with no element leaf that covers neither operand. The merge must be a
	// sound upper bound instead, so the result covers both base and nested, and is
	// commutative.
	if !Covers(got, base) || !Covers(got, nested) {
		t.Fatalf("MergeForConvergence(concrete[], concrete[][]) = %v must cover both base and nested", got)
	}
	if _, ok := got.(*typ.Recursive); ok {
		t.Fatalf("sequence depth growth must not fold into a recursive family: %v", got)
	}
	if reverse := MergeForConvergence(nested, base); !SameConvergedFact(got, reverse) {
		t.Fatalf("merge must be commutative: %v vs %v", got, reverse)
	}
}

func TestMergeForConvergence_FoldsSelfEmbeddingRecordGrowth(t *testing.T) {
	prev := typ.NewRecord().Field("x", typ.Number).Build()
	next := typ.NewRecord().
		Field("x", typ.Number).
		Field("z", typ.NewRecord().Field("y", prev).Build()).
		Build()

	got := MergeForConvergence(prev, next)
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("MergeForConvergence(self-embedding record) = %T %[1]v, want recursive type", got)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	if body.GetField("x") == nil {
		t.Fatalf("recursive body lost root field x: %v", body)
	}
	z := body.GetField("z")
	if z == nil {
		t.Fatalf("recursive body lost recursive field z: %v", body)
	}
	zBody, ok := z.Type.(*typ.Record)
	if !ok {
		t.Fatalf("z field = %T %[1]v, want record", z.Type)
	}
	y := zBody.GetField("y")
	if y == nil || !typ.IsRecursiveRef(y.Type, rec) {
		t.Fatalf("z.y = %v, want recursive self reference %v", y, rec)
	}
}

func TestMergeForConvergence_SelfEmbeddingRecursiveShapeStabilizes(t *testing.T) {
	first := typ.NewRecord().Field("x", typ.Number).Build()
	second := typ.NewRecord().
		Field("x", typ.Number).
		Field("z", typ.NewRecord().Field("y", first).Build()).
		Build()
	stable := MergeForConvergence(first, second)

	next := typ.NewRecord().
		Field("x", typ.Number).
		Field("z", typ.NewRecord().Field("y", stable).Build()).
		Build()
	got := MergeForConvergence(stable, next)
	if !typ.TypeEquals(stable, got) {
		t.Fatalf("recursive convergence did not stabilize:\nprev=%v\nnext=%v", stable, got)
	}
}

func TestMergeForConvergence_RecursiveSuiteRecordFamilyStabilizes(t *testing.T) {
	suiteObservation := func(parent typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("parent", typ.NewOptional(parent)).
			Field("children", typ.NewArray(parent)).
			Field("full_path", typ.String).
			Build()
	}

	var stable typ.Type = typ.NewRecord().
		Field("name", typ.String).
		Field("parent", typ.Nil).
		Field("children", typ.NewArray(typ.NewRecord().Build())).
		Field("full_path", typ.String).
		Build()

	for i := 0; i < 4; i++ {
		stable = MergeForConvergence(stable, suiteObservation(stable))
	}
	got := MergeForConvergence(stable, suiteObservation(stable))
	if !typ.TypeEquals(stable, got) {
		t.Fatalf("suite record family should reach a stable recursive product:\nprev=%v\nnext=%v", stable, got)
	}
	if !typ.ContainsRecursive(got) {
		t.Fatalf("suite record family should be represented as a recursive product, got %v", got)
	}
}

func TestMergeForConvergence_RecursiveProductJoinUsesActiveEquation(t *testing.T) {
	suite := func(label string) typ.Type {
		return typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
			return typ.NewRecord().
				Field("name", typ.String).
				Field("children", typ.NewArray(self)).
				Field("payload", typ.NewRecord().
					Field("owner", self).
					Field(label, typ.String).
					Build()).
				Build()
		})
	}

	stable := suite("base")
	for i := 0; i < 32; i++ {
		stable = MergeForConvergence(stable, suite(fmt.Sprintf("case_%02d", i)))
	}

	rec, ok := stable.(*typ.Recursive)
	if !ok {
		t.Fatalf("recursive product join = %T %[1]v, want recursive product", stable)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	payload := body.GetField("payload")
	if payload == nil {
		t.Fatalf("recursive body lost payload field: %v", body)
	}
}

func TestSelfEmbeddingUpperBound_RecursiveCoverageIsCoinductive(t *testing.T) {
	stable := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("value", typ.String).
			Field("next", typ.NewOptional(self)).
			Build()
	})
	observation := typ.NewRecord().
		Field("value", typ.String).
		Field("next", typ.NewOptional(stable)).
		Build()

	got, ok := SelfEmbeddingUpperBound(stable, observation)
	if !ok || !typ.TypeEquals(got, stable) {
		t.Fatalf("recursive upper bound should cover same-family observation without unfolding:\ngot=%v ok=%v\nwant=%v", got, ok, stable)
	}
}

func TestSelfEmbeddingUpperBound_RecursiveCoverageRejectsBroadObservation(t *testing.T) {
	stable := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("value", typ.String).
			Field("next", typ.NewOptional(self)).
			Build()
	})
	observation := typ.NewRecord().
		Field("value", typ.String).
		Field("next", typ.Any).
		Build()

	if got, ok := SelfEmbeddingUpperBound(stable, observation); ok {
		t.Fatalf("recursive upper bound must not narrow a broad observation: %v", got)
	}
}

func TestSelfEmbeddingUpperBound_UsesExistingNestedRecursiveRecord(t *testing.T) {
	base := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(typ.NewRecord().Build())).
		Build()
	rec := typ.NewRecursivePlaceholder("Suite")
	upper := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(rec)).
		Build()
	rec.SetBody(upper)

	got, ok := SelfEmbeddingUpperBound(base, upper)
	if !ok || !typ.TypeEquals(got, upper) {
		t.Fatalf("expected existing recursive record upper bound %v, got %v ok=%v", upper, got, ok)
	}
}

func TestMergeForConvergence_SelfEmbeddingMapShapeStabilizes(t *testing.T) {
	entry := typ.NewRecord().OptField("proc", typ.Any).Build()
	first := typ.NewMap(typ.String, entry)
	second := typ.NewMap(typ.String,
		typ.NewRecord().Field("child", first).Build(),
	)
	stable := MergeForConvergence(first, second)

	next := typ.NewMap(typ.String,
		typ.NewRecord().Field("child", stable).Build(),
	)
	got := MergeForConvergence(stable, next)
	if !typ.TypeEquals(stable, got) {
		t.Fatalf("recursive map convergence did not stabilize:\nprev=%v\nnext=%v", stable, got)
	}
}

func TestDirectSelfEmbeddingUpperBound_FoldsMapValueUnion(t *testing.T) {
	anchor := typ.NewMap(typ.String, typ.Any)
	observation := typ.NewMap(typ.String, typ.NewUnion(typ.String, anchor))

	got, ok := DirectSelfEmbeddingUpperBound(anchor, observation)
	if !ok {
		t.Fatalf("expected direct map self-embedding upper bound")
	}
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("upper bound = %T %[1]v, want recursive type", got)
	}
	body, ok := rec.Body.(*typ.Map)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want map", rec.Body)
	}
	if !typ.TypeEquals(body.Key, typ.String) {
		t.Fatalf("recursive map key = %v, want string", body.Key)
	}
}

func TestDirectSelfEmbeddingUpperBound_DoesNotAdmitUnionAnchor(t *testing.T) {
	left := typ.NewRecord().Field("kind", typ.LiteralString("left")).Build()
	right := typ.NewRecord().Field("kind", typ.LiteralString("right")).Build()
	anchor := typ.NewUnion(left, right)
	observation := typ.NewRecord().
		Field("child", anchor).
		Build()

	if got, ok := DirectSelfEmbeddingUpperBound(anchor, observation); ok {
		t.Fatalf("union anchor cannot be folded as a finite self-embedding product, got %v", got)
	}
}

func TestDirectSelfEmbeddingUpperBound_ScansWrapperFamilyWithoutDepthCap(t *testing.T) {
	anchor := typ.NewMap(typ.String, typ.Any)
	var child typ.Type = anchor
	for i := 0; i < typ.DefaultRecursionDepth+8; i++ {
		child = typ.NewUnion(typ.NewOptional(child), typ.String)
	}
	observation := typ.NewMap(typ.String, child)

	got, ok := DirectSelfEmbeddingUpperBound(anchor, observation)
	if !ok {
		t.Fatalf("expected wrapped direct self-embedding upper bound")
	}
	if _, ok := got.(*typ.Recursive); !ok {
		t.Fatalf("upper bound = %T %[1]v, want recursive type", got)
	}
}

func TestDirectSelfEmbeddingUpperBound_DoesNotScanFunctionProducts(t *testing.T) {
	anchor := typ.Func().Returns(typ.String).Build()
	observation := typ.Func().Returns(anchor).Build()

	if got, ok := DirectSelfEmbeddingUpperBound(anchor, observation); ok {
		t.Fatalf("function products should be handled by function widening, got %v", got)
	}
}

func TestMergeForConvergence_DynamicMapExpectationStabilizes(t *testing.T) {
	base := typ.NewMap(typ.Any, typ.Any)
	expected := typ.NewOptional(typ.NewMap(typ.String, typ.Any))

	merged := MergeForConvergence(base, expected)
	mergedAgain := MergeForConvergence(merged, expected)
	if !typ.TypeEquals(merged, mergedAgain) {
		t.Fatalf("map convergence did not stabilize:\nfirst=%v\nsecond=%v", merged, mergedAgain)
	}
	want := typ.NewOptional(typ.NewMap(typ.Any, typ.Any))
	if !typ.TypeEquals(merged, want) {
		t.Fatalf("MergeForConvergence(map any, optional map string) = %v, want %v", merged, want)
	}
	reverse := MergeForConvergence(expected, base)
	if !typ.TypeEquals(reverse, want) {
		t.Fatalf("MergeForConvergence(optional map string, map any) = %v, want %v", reverse, want)
	}
}

func TestMergeForConvergence_RecursiveArrayEvidenceBeatsDynamicArrayOscillation(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewOptional(typ.NewMap(typ.String, typ.Any))).
		Build()
	precise := typ.NewRecursive("Inferred", func(self typ.Type) typ.Type {
		return typ.NewArray(entry)
	})
	dynamic := typ.NewArray(typ.Any)

	first := MergeForConvergence(dynamic, precise)
	second := MergeForConvergence(first, dynamic)
	third := MergeForConvergence(second, precise)
	if !typ.TypeEquals(first, second) || !typ.TypeEquals(second, third) {
		t.Fatalf("recursive array convergence oscillates:\nfirst=%v\nsecond=%v\nthird=%v", first, second, third)
	}
	if !typ.TypeEquals(first, precise) {
		t.Fatalf("recursive evidence should remain the stable product:\ngot=%v\nwant=%v", first, precise)
	}
}

func TestMergeForConvergence_RecursiveArrayElidesNilableEvidenceSymmetrically(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewOptional(typ.NewMap(typ.String, typ.Any))).
		Build()
	nilable := typ.NewOptional(typ.NewArray(entry))
	precise := typ.NewRecursive("Inferred", func(self typ.Type) typ.Type {
		return typ.NewArray(entry)
	})
	dynamic := typ.NewArray(typ.Any)

	left := MergeForConvergence(nilable, precise)
	right := MergeForConvergence(precise, nilable)
	if !typ.TypeEquals(left, precise) || !typ.TypeEquals(right, precise) {
		t.Fatalf("nilable/recursive merge is order-dependent:\nleft=%v\nright=%v\nwant=%v", left, right, precise)
	}

	state := MergeForConvergence(nil, nilable)
	for _, next := range []typ.Type{precise, nilable, dynamic, precise} {
		state = MergeForConvergence(state, next)
	}
	if !typ.TypeEquals(state, precise) {
		t.Fatalf("assignment SCC sequence did not stabilize at recursive product:\ngot=%v\nwant=%v", state, precise)
	}
}

func TestMergeForConvergence_DynamicArraySlotDoesNotCreateSelfEmbeddingCycle(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewOptional(typ.NewMap(typ.String, typ.Any))).
		Build()
	precise := typ.NewOptional(typ.NewArray(entry))
	dynamic := typ.NewArray(typ.Any)

	left := MergeForConvergence(precise, dynamic)
	right := MergeForConvergence(dynamic, precise)
	if !typ.TypeEquals(left, precise) || !typ.TypeEquals(right, precise) {
		t.Fatalf("dynamic array slot should refine to precise array evidence:\nleft=%v\nright=%v\nwant=%v", left, right, precise)
	}

	state := MergeForConvergence(nil, precise)
	for _, next := range []typ.Type{dynamic, precise, dynamic} {
		state = MergeForConvergence(state, next)
	}
	if !typ.TypeEquals(state, precise) {
		t.Fatalf("dynamic/precise array sequence did not stabilize:\ngot=%v\nwant=%v", state, precise)
	}
}

func TestDynamicMapUpperBound_TopMapCoversOptionalNarrowMap(t *testing.T) {
	base := typ.NewMap(typ.Any, typ.Any)
	expected := typ.NewOptional(typ.NewMap(typ.String, typ.Any))
	want := typ.NewOptional(base)

	got, ok := DynamicMapUpperBound(base, expected)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("DynamicMapUpperBound(map any, optional map string) = %v ok=%v, want %v", got, ok, want)
	}
}

func TestMergeForConvergence_CollapsesStructuralUnionProducts(t *testing.T) {
	left := typ.NewUnion(
		typ.NewAlias("LeftName", typ.NewRecord().Field("name", typ.String).Build()),
		typ.NewAlias("LeftCount", typ.NewRecord().Field("count", typ.Number).Build()),
	)
	right := typ.NewUnion(
		typ.NewAlias("RightName", typ.NewRecord().Field("name", typ.String).Field("ready", typ.Boolean).Build()),
		typ.NewAlias("RightCount", typ.NewRecord().Field("count", typ.Number).Field("ready", typ.Boolean).Build()),
	)

	got := MergeForConvergence(left, right)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("MergeForConvergence(structural unions) = %T %[1]v, want collapsed record", got)
	}
	for _, name := range []string{"name", "count", "ready"} {
		field := rec.GetField(name)
		if field == nil {
			t.Fatalf("collapsed record missing field %q: %v", name, rec)
		}
		if !field.Optional {
			t.Fatalf("collapsed record field %q should be optional after union convergence: %v", name, rec)
		}
	}
}

func TestNormalizeFactType_LeavesRecursiveProductUnionToConvergence(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Field("meta", typ.String).
			Build()
	})

	normalized := NormalizeFactType(typ.NewUnion(left, right))
	u, ok := normalized.(*typ.Union)
	if !ok || len(u.Members) != 2 {
		t.Fatalf("NormalizeFactType(recursive union) = %T %[1]v, want uncoalesced recursive union", normalized)
	}

	merged := MergeForConvergence(left, right)
	if _, ok := merged.(*typ.Recursive); !ok {
		t.Fatalf("MergeForConvergence(recursive products) = %T %[1]v, want convergence-owned recursive product", merged)
	}
}

func TestUnsafePrecisionDrop_RecursiveProductsDoNotUseDeepSubtype(t *testing.T) {
	prev := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("next", typ.NewOptional(self)).
			Build()
	})
	merged := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("next", typ.NewOptional(self)).
			Field("children", typ.NewArray(self)).
			Build()
	})

	if UnsafePrecisionDrop(prev, merged) {
		t.Fatalf("recursive product precision check should not be reported through deep subtype")
	}
}

func TestFoldSelfEmbedding_RecognizesMetatableIndexCycle(t *testing.T) {
	anchor := metatableCycleTower(2)
	obs := metatableCycleTower(3)
	if !typ.TypeEquals(anchorIndexField(obs), anchor) {
		t.Fatalf("obs.__index should equal anchor structurally:\n  anchor=%v\n  obs.__index=%v", anchor, anchorIndexField(obs))
	}
	folded, ok := FoldSelfEmbedding(anchor, obs)
	if !ok {
		t.Fatalf("FoldSelfEmbedding(tower2, tower3) did not fire; anchor must be recognized below root")
	}
	if !typ.ContainsRecursive(folded) {
		t.Fatalf("folded = %v, want recursive mu", folded)
	}
}

func anchorIndexField(t typ.Type) typ.Type {
	rec, ok := UnwrapStructuralShape(t).(*typ.Record)
	if !ok {
		return nil
	}
	f := rec.GetField("__index")
	if f == nil {
		return nil
	}
	return f.Type
}

// asymRefinedTower mirrors the real fixpoint observation: the innermost __index
// level carries the unrefined self (run: fun(self: unknown)) while each outer
// level refines run's self parameter. Successive observations differ only in
// __index depth, but inner levels are less-refined than outer ones, so the
// tower is self-similar without being structurally identical at each level.
func asymRefinedTower(depth int) typ.Type {
	var inner typ.Type = typ.Unknown
	for i := 0; i <= depth; i++ {
		selfArg := typ.Type(typ.Unknown)
		if i < depth {
			selfArg = typ.NewRecord().Field("pending_ops", typ.Number).SetOpen(true).Build()
		}
		inner = typ.NewRecord().
			Field("__index", inner).
			Field("new", typ.Func().Returns(typ.NewRecord().Field("pending_ops", typ.Number).Build()).Build()).
			Field("pending_ops", typ.Number).
			Field("run", typ.Func().Param("self", selfArg).Returns(typ.Nil).Build()).
			Field("stopping", typ.Boolean).
			Build()
	}
	return inner
}

func TestFoldSelfEmbedding_RecognizesAsymmetricRefinedCycle(t *testing.T) {
	anchor := asymRefinedTower(2)
	obs := asymRefinedTower(3)
	folded, ok := FoldSelfEmbedding(anchor, obs)
	if !ok {
		t.Fatalf("FoldSelfEmbedding must recognize the self-similar refined __index cycle below root:\n  anchor=%v\n  obs=%v", anchor, obs)
	}
	if !typ.ContainsRecursive(folded) {
		t.Fatalf("folded = %v, want recursive mu", folded)
	}
}

func TestCanonicalRecursiveFamily_DedupesSameFamilyFolds(t *testing.T) {
	a, okA := FoldSelfEmbedding(asymRefinedTower(2), asymRefinedTower(3))
	b, okB := FoldSelfEmbedding(asymRefinedTower(3), asymRefinedTower(4))
	if !okA || !okB {
		t.Fatalf("both folds must fire: okA=%v okB=%v", okA, okB)
	}
	if typ.ProductFamilyHash(a) != typ.ProductFamilyHash(b) {
		t.Fatalf("folds of one cyclic family must share family hash: %d vs %d", typ.ProductFamilyHash(a), typ.ProductFamilyHash(b))
	}
	if CanonicalRecursiveFamily(a) != CanonicalRecursiveFamily(b) {
		t.Fatalf("CanonicalRecursiveFamily must map same-family folds to one representative; got distinct reps\n  a=%v\n  b=%v", CanonicalRecursiveFamily(a), CanonicalRecursiveFamily(b))
	}
	if !SameConvergedFact(a, b) {
		t.Fatalf("SameConvergedFact must hold for two folds of the same cyclic family")
	}
}

func TestMergeForConvergence_CollapsesSelfParamSameFamilyUnion(t *testing.T) {
	classRec := metatableCycleTower(3)
	classMu, ok := FoldSelfEmbedding(metatableCycleTower(2), metatableCycleTower(3))
	if !ok {
		t.Fatalf("fold must fire")
	}
	// One observation has the self param as the recursive family; another as a
	// finite unfolding of the same family. Merging the two functions must collapse
	// the self param to a single member, not keep a growing union.
	fnRec := typ.Func().Param("self", classMu).Returns(typ.Nil).Build()
	fnFin := typ.Func().Param("self", classRec).Returns(typ.Nil).Build()
	got := MergeForConvergence(fnRec, fnFin)
	fn, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("merge(fn, fn) = %T %[1]v, want function", got)
	}
	if u, ok := UnwrapStructuralShape(fn.Params[0].Type).(*typ.Union); ok {
		t.Fatalf("self param stayed a union of %d members, want single collapsed family: %v", len(u.Members), fn.Params[0].Type)
	}
}

func TestNormalizeFactType_CollapsesSameFamilyRecursiveUnionInFunctionParam(t *testing.T) {
	muA, okA := FoldSelfEmbedding(metatableCycleTower(2), metatableCycleTower(3))
	muB, okB := FoldSelfEmbedding(metatableCycleTower(3), metatableCycleTower(4))
	if !okA || !okB {
		t.Fatalf("both folds must fire")
	}
	if !SameConvergedFact(muA, muB) {
		t.Fatalf("the two folds must be the same converged family")
	}
	// A function whose self param is a union of two distinct-node observations of
	// one recursive family must normalize to a single self member, otherwise the
	// fixpoint compares fresh recursive ids forever and never converges.
	fn := typ.Func().Param("self", typ.NewUnion(muA, muB)).Returns(typ.Nil).Build()
	got := NormalizeFactType(fn)
	gotFn, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("NormalizeFactType(fn) = %T, want function", got)
	}
	if u, ok := UnwrapStructuralShape(gotFn.Params[0].Type).(*typ.Union); ok {
		t.Fatalf("self param stayed a %d-member union, want single collapsed family: %v", len(u.Members), gotFn.Params[0].Type)
	}
}

// classFamilyWithSelfMethod builds the canonical recursive class: a record whose
// __index re-embeds the class (self) and whose run method takes self. Each call
// mints a fresh recursive node id, mirroring successive fixpoint observations.
func classFamilyWithSelfMethod() typ.Type {
	return typ.NewRecursive("self", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("__index", self).
			Field("new", typ.Func().Returns(typ.NewRecord().Field("pending_ops", typ.Number).Build()).Build()).
			Field("pending_ops", typ.Number).
			Field("run", typ.Func().Param("self", self).Returns(typ.Nil).Build()).
			Field("stopping", typ.Boolean).
			Build()
	})
}

func TestCanonicalRecursiveFamily_StableForFreshNodeClassFamily(t *testing.T) {
	a := classFamilyWithSelfMethod()
	b := classFamilyWithSelfMethod()
	if typ.SameNode(a, b) {
		t.Fatalf("test builds distinct nodes")
	}
	if typ.ProductFamilyHash(a) != typ.ProductFamilyHash(b) {
		t.Fatalf("structurally identical class families must share family hash")
	}
	if !FactTypeEqual(a, b) {
		t.Fatalf("FactTypeEqual must hold for two builds of one class family")
	}
	if CanonicalRecursiveFamily(a) != CanonicalRecursiveFamily(b) {
		t.Fatalf("CanonicalRecursiveFamily must map fresh-node builds of one class family to one representative")
	}
}

func TestRecursiveEvidenceCovers_ClassCoversFiniteClassUnfolding(t *testing.T) {
	mu := classFamilyWithSelfMethod()
	finite := metatableCycleTower(2)
	if !RecursiveEvidenceCovers(mu, finite) {
		t.Fatalf("recursive class family must cover its finite unfolding tower bottoming at an unknown self-edge")
	}
}
