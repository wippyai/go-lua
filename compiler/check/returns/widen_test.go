package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenFacts_DoesNotOverrideReturnSummariesWithNarrowReturns(t *testing.T) {
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.Integer}},
		},
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.Integer},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Narrow: []typ.Type{typ.Nil}},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			1: []typ.Type{typ.Nil},
		},
	}

	merged := WidenFacts(prev, next)
	got := merged.ReturnSummaries[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected ReturnSummaries[1]=integer, got %v", got)
	}
}

func TestWidenFacts_ElidesOptionalFromNarrowReturns(t *testing.T) {
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.NewOptional(typ.Integer)}},
		},
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.NewOptional(typ.Integer)},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Narrow: []typ.Type{typ.Integer}},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			1: []typ.Type{typ.Integer},
		},
	}

	merged := WidenFacts(prev, next)
	got := merged.ReturnSummaries[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected ReturnSummaries[1]=integer, got %v", got)
	}
}

func TestWidenReturnSummaries_RefinesOptionalForFirstOrderTypes(t *testing.T) {
	prev := api.ReturnSummaries{
		1: []typ.Type{typ.NewOptional(typ.Integer)},
	}
	next := api.ReturnSummaries{
		1: []typ.Type{typ.Integer},
	}

	merged := WidenReturnSummaries(prev, next)
	got := merged[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected integer after first-order refinement, got %v", got)
	}
}

func TestWidenReturnSummaries_UsesMonotoneJoinForHigherOrderReturns(t *testing.T) {
	nestedUnknown := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	nestedString := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.String).Build()).
		Build()

	base := typ.NewRecord().
		Field("build", typ.Func().Returns(nestedUnknown).Build()).
		Build()
	refined := typ.NewRecord().
		Field("build", typ.Func().Returns(nestedString).Build()).
		Build()

	prev := api.ReturnSummaries{
		1: []typ.Type{base},
	}
	next := api.ReturnSummaries{
		1: []typ.Type{refined},
	}

	merged := WidenReturnSummaries(prev, next)
	got := merged[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], base) {
		t.Fatalf("expected stable upper bound for higher-order return, got %v", got)
	}
}

// Incomparable approximations of one higher-order record join into a single
// record whose fields admit both, rather than a union of the approximations.
func TestWidenReturnSummaries_JoinsIncomparableHigherOrderRecords(t *testing.T) {
	method := typ.Func().Returns(typ.Func().Returns(typ.String).Build()).Build()
	earlier := typ.NewRecord().
		Field("run", method).
		Field("id", typ.Any).
		Field("cache", typ.Nil).
		Build()
	current := typ.NewRecord().
		Field("run", method).
		Field("id", typ.String).
		Field("cache", typ.Any).
		Build()

	merged := WidenReturnSummaries(
		api.ReturnSummaries{1: []typ.Type{earlier}},
		api.ReturnSummaries{1: []typ.Type{current}},
	)
	got := merged[1]
	want := typ.NewRecord().
		Field("cache", typ.Any).
		Field("id", typ.Any).
		Field("run", method).
		Build()
	if len(got) != 1 || !typ.TypeEquals(got[0], want) {
		t.Fatalf("expected [%v], got %v", want, got)
	}
}

func TestWidenReturnSummaries_InterfaceMethodsDoNotBlockOptionalElision(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	prev := api.ReturnSummaries{
		1: []typ.Type{typ.NewOptional(dbType)},
	}
	next := api.ReturnSummaries{
		1: []typ.Type{dbType},
	}

	merged := WidenReturnSummaries(prev, next)
	got := merged[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], dbType) {
		t.Fatalf("expected optional elision for interface return, got %v", got)
	}
}

func TestMergeFunctionReturnsIfSameShape_GenericFunctions(t *testing.T) {
	prev := typ.Func().
		TypeParam("T", nil).
		Returns(typ.String).
		Build()
	next := typ.Func().
		TypeParam("T", nil).
		Returns(typ.Integer).
		Build()

	mergedType, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if !ok {
		t.Fatal("expected generic same-shape functions to merge")
	}
	merged, ok := mergedType.(*typ.Function)
	if !ok {
		t.Fatalf("expected merged function type, got %T", mergedType)
	}
	if len(merged.TypeParams) != 1 || merged.TypeParams[0] == nil || merged.TypeParams[0].Name != "T" {
		t.Fatalf("expected merged generic type parameter T, got %+v", merged.TypeParams)
	}
	if len(merged.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(merged.Returns))
	}
	want := typ.NewUnion(typ.String, typ.Integer)
	if !typ.TypeEquals(merged.Returns[0], want) {
		t.Fatalf("expected merged return %v, got %v", want, merged.Returns[0])
	}
}

func TestMergeFunctionReturnsIfSameShape_GenericTypeParamsMustMatch(t *testing.T) {
	prev := typ.Func().
		TypeParam("T", nil).
		Returns(typ.String).
		Build()
	next := typ.Func().
		TypeParam("U", nil).
		Returns(typ.Integer).
		Build()

	_, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if ok {
		t.Fatal("expected mismatched generic params not to merge")
	}
}

func TestMergeFuncTypes_DoesNotRegressToNarrowerNilReturn(t *testing.T) {
	prev := typ.Func().
		Returns(typ.NewOptional(typ.Integer)).
		Build()
	next := typ.Func().
		Returns(typ.Nil).
		Build()

	merged := MergeFunctionFactType(prev, next)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("expected merged function return, got %T", merged)
	}
	if !typ.TypeEquals(fn.Returns[0], typ.NewOptional(typ.Integer)) {
		t.Fatalf("expected integer? return after merge, got %v", fn.Returns[0])
	}
}

func TestMergeFunctionReturnsIfSameShape_NormalizesLeakedTypeParams(t *testing.T) {
	prev := typ.Func().
		Returns(typ.NewTypeParam("T", nil)).
		Build()
	next := typ.Func().
		Returns(typ.Integer).
		Build()

	mergedType, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if !ok {
		t.Fatal("expected same-shape functions to merge")
	}
	merged, ok := mergedType.(*typ.Function)
	if !ok || len(merged.Returns) != 1 {
		t.Fatalf("expected merged function return, got %T", mergedType)
	}
	if !typ.TypeEquals(merged.Returns[0], typ.Integer) {
		t.Fatalf("expected leaked type param to normalize to integer, got %v", merged.Returns[0])
	}
}

func TestMergeFuncTypes_PrefersWiderSupertypeOnSubtypeRelation(t *testing.T) {
	merged := MergeFunctionFactType(typ.Integer, typ.Number)
	if !typ.TypeEquals(merged, typ.Number) {
		t.Fatalf("expected wider supertype number, got %v", merged)
	}

	merged = MergeFunctionFactType(typ.Number, typ.Integer)
	if !typ.TypeEquals(merged, typ.Number) {
		t.Fatalf("expected wider supertype number, got %v", merged)
	}
}

func TestMergeFuncTypes_IsCommutativeForIncomparableSignatures(t *testing.T) {
	coarse := typ.Func().
		Param("entries", typ.Any).
		Returns(typ.Integer).
		Build()
	refined := typ.Func().
		Param("entries", typ.NewArray(typ.String)).
		Returns(typ.Integer).
		Build()

	forward := MergeFunctionFactType(coarse, refined)
	reverse := MergeFunctionFactType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestMergeFuncTypes_AliasInputsUseCanonicalJoin(t *testing.T) {
	coarse := typ.NewAlias("CoarseFn", typ.Func().
		Param("entries", typ.Any).
		Returns(typ.Integer).
		Build())
	refined := typ.NewAlias("RefinedFn", typ.Func().
		Param("entries", typ.NewArray(typ.String)).
		Returns(typ.Integer).
		Build())

	forward := MergeFunctionFactType(coarse, refined)
	reverse := MergeFunctionFactType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative alias merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestMergeFuncTypes_MapVsOpenRecordUsesCanonicalJoin(t *testing.T) {
	coarse := typ.Func().
		Param("t", typ.NewRecord().SetOpen(true).Build()).
		Returns(typ.String).
		Build()
	refined := typ.Func().
		Param("t", typ.NewMap(typ.String, typ.NewArray(typ.String))).
		Returns(typ.String).
		Build()

	forward := MergeFunctionFactType(coarse, refined)
	reverse := MergeFunctionFactType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative map/open-record merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestWidenLiteralSigs_DoesNotNarrowComparableSignature(t *testing.T) {
	lit := &ast.FunctionExpr{}

	prev := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Number).Build(),
	}
	next := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Integer).Build(),
	}

	merged := WidenLiteralSigs(prev, next)
	got := merged[lit]
	if got == nil {
		t.Fatal("expected merged literal signature")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(got.Returns))
	}
	if !subtype.IsSubtype(prev[lit].Returns[0], got.Returns[0]) {
		t.Fatalf("expected merged return to be supertype of prev (%v), got %v", prev[lit].Returns[0], got.Returns[0])
	}
	if !subtype.IsSubtype(next[lit].Returns[0], got.Returns[0]) {
		t.Fatalf("expected merged return to be supertype of next (%v), got %v", next[lit].Returns[0], got.Returns[0])
	}
	if typ.TypeEquals(got.Returns[0], next[lit].Returns[0]) {
		t.Fatalf("expected merged return not to regress to narrower next-only type %v", got.Returns[0])
	}
}

func TestWidenLiteralSigs_PrefersMergedSameShapeSignature(t *testing.T) {
	lit := &ast.FunctionExpr{}

	prev := api.LiteralSigs{
		lit: typ.Func().Returns(typ.String).Build(),
	}
	next := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Integer).Build(),
	}

	merged := WidenLiteralSigs(prev, next)
	got := merged[lit]
	if got == nil {
		t.Fatal("expected merged literal signature")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(got.Returns))
	}
	want := typ.NewUnion(typ.String, typ.Integer)
	if !typ.TypeEquals(got.Returns[0], want) {
		t.Fatalf("expected merged return %v, got %v", want, got.Returns[0])
	}
}

func TestTypeContainsFunction_IgnoresInterfaceMethodSignatures(t *testing.T) {
	iface := typ.NewInterface("Reader", []typ.Method{
		{
			Name: "next",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Func().Returns(typ.String).Build()).
				Build(),
		},
	})
	if typeContainsFunction(iface) {
		t.Fatalf("expected interface method signatures to be ignored, got true")
	}
}

func TestHasHigherOrderGrowthRisk_DetectsFunctionReturningFunction(t *testing.T) {
	tp := typ.Func().
		Returns(typ.Func().Returns(typ.String).Build()).
		Build()
	if !hasHigherOrderGrowthRisk(tp) {
		t.Fatalf("expected higher-order growth risk to be detected")
	}
}

func TestMethodTypeHasSelfRecursiveReturn_IgnoresInterfaceMethods(t *testing.T) {
	owner := typ.NewRecord().Field("id", typ.String).Build()
	methodType := typ.NewInterface("HasBuild", []typ.Method{
		{
			Name: "build",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(owner).
				Build(),
		},
	})
	if methodTypeHasSelfRecursiveReturn(methodType, owner) {
		t.Fatalf("expected interface method signatures to be ignored for self-recursive detection")
	}
}

// methodTableApproximation returns the method table after n fixpoint steps
// of `function t:command() return self end`: step n types command against the
// table of step n-1, starting from a command that returns nil.
func methodTableApproximation(n int) *typ.Record {
	ret := typ.Type(typ.Nil)
	var table *typ.Record
	for i := 0; i <= n; i++ {
		table = typ.NewRecord().
			Field("command", typ.Func().Param("self", typ.Unknown).Returns(ret, typ.NewOptional(typ.String)).Build()).
			Field("name", typ.String).
			Build()
		ret = typ.NewOptional(table)
	}
	return table
}

func TestMaybeWidenTypeForConvergence_FoldsSelfReturningMethodTable(t *testing.T) {
	approx := methodTableApproximation(3)

	widened := maybeWidenTypeForConvergence(approx)
	rec, ok := widened.(*typ.Recursive)
	if !ok {
		t.Fatalf("expected a recursive method table, got %s", widened)
	}
	for i := 0; i <= 5; i++ {
		if !subtype.IsSubtype(methodTableApproximation(i), rec) {
			t.Fatalf("approximation %d must be a subtype of the folded table", i)
		}
	}
	if subtype.IsSubtype(rec, methodTableApproximation(0)) {
		t.Fatal("folded table must be strictly wider than the first approximation")
	}
}

func TestMaybeWidenTypeForConvergence_FoldingReachesFixpoint(t *testing.T) {
	first := maybeWidenTypeForConvergence(methodTableApproximation(3))
	second := maybeWidenTypeForConvergence(maybeWidenTypeForConvergence(methodTableApproximation(7)))
	if !typ.TypeEquals(first, second) {
		t.Fatalf("folding different approximations must converge:\n%s\n%s", first, second)
	}

	// The next fixpoint step types command against the folded table.
	step := typ.NewRecord().
		Field("command", typ.Func().Param("self", typ.Unknown).Returns(typ.NewOptional(first), typ.NewOptional(typ.String)).Build()).
		Field("name", typ.String).
		Build()
	if again := maybeWidenTypeForConvergence(step); !typ.TypeEquals(again, first) {
		t.Fatalf("step over the folded table must fold back to it:\n%s\n%s", again, first)
	}
}

func TestMaybeWidenTypeForConvergence_KeepsRecordsWithOtherFields(t *testing.T) {
	inner := typ.NewRecord().
		Field("command", typ.Func().Param("self", typ.Unknown).Returns(typ.Nil).Build()).
		Build()
	outer := typ.NewRecord().
		Field("command", typ.Func().Param("self", typ.Unknown).Returns(inner).Build()).
		Field("name", typ.String).
		Build()

	if widened := maybeWidenTypeForConvergence(outer); !typ.TypeEquals(widened, subtype.WidenForInference(outer)) {
		t.Fatalf("a nested record with different fields is not an approximation of the table, got %s", widened)
	}
}

func TestJoinIterationFact_ReadonlyFieldJoinsToUpperBound(t *testing.T) {
	narrow := typ.NewRecord().
		ReadonlyField("route", typ.Func().Returns(typ.Nil).Build()).
		Build()
	wide := typ.NewRecord().
		ReadonlyField("route", typ.Func().Returns(typ.NewOptional(typ.Boolean)).Build()).
		Build()

	for _, got := range []typ.Type{joinIterationFact(narrow, wide), joinIterationFact(wide, narrow)} {
		if !typ.TypeEquals(got, wide) {
			t.Fatalf("expected the wider record %s, got %s", wide, got)
		}
	}
}

// A mutable field is invariant: a stale wider field type would reject the
// values the current fact describes, so the current field type wins.
func TestJoinIterationFact_MutableFieldTakesCurrentType(t *testing.T) {
	previous := typ.NewRecord().Field("days", typ.NewArray(typ.NewOptional(typ.Number))).Build()
	current := typ.NewRecord().Field("days", typ.NewArray(typ.Number)).Build()

	got := joinIterationFact(previous, current)
	if !typ.TypeEquals(got, current) {
		t.Fatalf("expected %s, got %s", current, got)
	}
	if !subtype.IsSubtype(current, got) {
		t.Fatalf("joined fact %s must admit the current values %s", got, current)
	}
}

// An optional fact joins its present values field by field and stays
// optional, so an earlier approximation of a record does not remain as a
// separate union member beside the resolved record.
func TestJoinIterationFact_OptionalRecordsJoinTheirPresentValues(t *testing.T) {
	earlier := typ.NewOptional(typ.NewRecord().
		Field("cache", typ.Nil).
		Field("session_id", typ.Unknown).
		Build())
	current := typ.NewOptional(typ.NewRecord().
		Field("cache", typ.Any).
		Field("session_id", typ.String).
		Build())

	got := joinIterationFact(earlier, current)
	if !typ.TypeEquals(got, current) {
		t.Fatalf("expected %s, got %s", current, got)
	}
}

func TestJoinParamHint_KeepsFieldsDiscoveredByEitherIteration(t *testing.T) {
	earlier := typ.NewRecord().
		Field("route", typ.Func().Returns(typ.Nil).Build()).
		Field("id", typ.Integer).
		Build()
	later := typ.NewRecord().
		Field("route", typ.Func().Returns(typ.NewOptional(typ.Boolean)).Build()).
		Field("name", typ.String).
		Build()
	want := typ.NewRecord().
		Field("route", typ.Func().Returns(typ.NewOptional(typ.Boolean)).Build()).
		Field("id", typ.Integer).
		Field("name", typ.String).
		Build()

	if got := joinIterationFact(earlier, later); !typ.TypeEquals(got, want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestJoinParamHint_UnknownFieldYieldsToResolvedField(t *testing.T) {
	card := typ.NewRecord().Field("name", typ.String).Build()
	earlier := typ.NewRecord().SetOpen(true).
		Field("card", typ.Unknown).
		Field("limit", typ.Integer).
		Build()
	later := typ.NewRecord().SetOpen(true).
		Field("card", card).
		Field("limit", typ.Integer).
		Build()

	for _, got := range []typ.Type{joinIterationFact(earlier, later), joinIterationFact(later, earlier)} {
		if !typ.TypeEquals(got, later) {
			t.Fatalf("expected %s, got %s", later, got)
		}
	}
}

func TestJoinParamHint_KeepsPreviousHintWhenItAdmitsTheCurrentOne(t *testing.T) {
	narrow := typ.NewRecord().ReadonlyField("size", typ.Integer).Build()
	wide := typ.NewRecord().ReadonlyField("size", typ.Number).Build()
	previous := typ.NewUnion(typ.String, narrow, wide)
	current := typ.NewUnion(typ.String, wide)
	if !subtype.IsSubtype(previous, current) || !subtype.IsSubtype(current, previous) {
		t.Fatal("test hints must be equivalent")
	}

	if got := joinIterationFact(previous, current); got != previous {
		t.Fatalf("expected the previous hint %s, got %s", previous, got)
	}
	if got := joinIterationFact(current, previous); got != current {
		t.Fatalf("expected the previous hint %s, got %s", current, got)
	}
}

func TestJoinParamHint_UnknownYieldsToHintWithPlaceholderMembers(t *testing.T) {
	withPlaceholder := typ.NewUnion(typ.NewRecord().SetOpen(true).Build(), typ.NewMap(typ.String, typ.Any))

	for _, got := range []typ.Type{joinIterationFact(typ.Unknown, withPlaceholder), joinIterationFact(withPlaceholder, typ.Unknown)} {
		if !typ.TypeEquals(got, withPlaceholder) {
			t.Fatalf("expected %s, got %s", withPlaceholder, got)
		}
	}
}

// linkedNodeApproximation is the written type of `node.parent = current;
// current = node` after n fixpoint steps; the first step saw the name as
// unresolved.
func linkedNodeApproximation(n int) typ.Type {
	var parent typ.Type = typ.Nil
	name := typ.Unknown
	var node *typ.Record
	for i := 0; i <= n; i++ {
		node = typ.NewRecord().
			Field("name", name).
			Field("parent", parent).
			Build()
		parent = typ.NewOptional(node)
		name = typ.String
	}
	return node
}

func TestWidenFieldWrites_FoldsNestedRecordApproximations(t *testing.T) {
	const fn, target = 1, 2
	write := func(t typ.Type) api.FieldWrites {
		return api.FieldWrites{fn: {target: {"current": t}}}
	}

	first := WidenFieldWrites(write(linkedNodeApproximation(2)), write(linkedNodeApproximation(3)))[fn][target]["current"]
	if _, ok := first.(*typ.Recursive); !ok {
		t.Fatalf("expected a recursive node type, got %s", first)
	}
	for i := 0; i <= 5; i++ {
		if !informationBelow(linkedNodeApproximation(i), first, make(map[[2]typ.Type]bool)) {
			t.Fatalf("approximation %d must lie below the folded node", i)
		}
	}

	// The next step writes a node whose parent is the folded type.
	step := typ.NewRecord().
		Field("name", typ.String).
		Field("parent", typ.NewOptional(first)).
		Build()
	second := WidenFieldWrites(write(first), write(step))[fn][target]["current"]
	if !typ.TypeEquals(first, second) {
		t.Fatalf("a step over the folded node must fold back to it:\n%s\n%s", first, second)
	}
}

func TestInformationBelow_UnresolvedFieldIsBelowResolved(t *testing.T) {
	early := typ.NewRecord().Field("name", typ.Unknown).Build()
	late := typ.NewRecord().Field("name", typ.String).Build()
	if !informationBelow(early, late, make(map[[2]typ.Type]bool)) {
		t.Fatal("an unresolved field must lie below its resolved type")
	}
	other := typ.NewRecord().Field("name", typ.Integer).Build()
	if informationBelow(other, late, make(map[[2]typ.Type]bool)) {
		t.Fatal("a conflicting resolved field must not lie below another")
	}
}
