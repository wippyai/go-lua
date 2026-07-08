package callcontract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/contract"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestInstantiateGenericCallWithTraceConvertsTypecallVocabulary(t *testing.T) {
	param := typ.NewTypeParam("T", typetable.NewRecord().Field("name", typ.String).Build())
	fn := typ.Func().
		TypeParamRef(param).
		Param("value", param).
		Returns(param).
		Build()

	got, violations, trace := InstantiateGenericCallWithTrace(fn, []typ.Type{typ.LiteralInt(42)})
	if got == nil {
		t.Fatal("InstantiateGenericCallWithTrace returned nil function")
	}
	if len(violations) != 1 {
		t.Fatalf("violations = %#v, want one", violations)
	}
	if violations[0].Index != 0 || violations[0].Got == nil || violations[0].Constraint == nil {
		t.Fatalf("violation = %#v, want converted index/got/constraint", violations[0])
	}
	if len(trace.Contributions) == 0 {
		t.Fatalf("trace = %#v, want converted contributions", trace)
	}
	if trace.Contributions[0].Param != param || trace.Contributions[0].Index != 0 || trace.Contributions[0].Type == nil {
		t.Fatalf("contribution = %#v, want converted param/index/type", trace.Contributions[0])
	}
}

func TestMemberCallableAndReceiverConsumptionHideTypecallStatus(t *testing.T) {
	run := typ.Func().Param("self", typ.Self).Returns(typ.String).Build()
	receiver := typetable.NewRecord().
		Field("run", run).
		Build()

	if fn, ok := Callable(run); !ok || fn == nil {
		t.Fatalf("Callable(receiver.run) = %v, %v; want function witness", fn, ok)
	}
	fn, status, ok := MemberCallable(receiver, "run")
	if !ok || status != MemberCallOK || fn == nil {
		t.Fatalf("MemberCallable = %v, %v, %v; want callable ok", fn, status, ok)
	}
	if !ParamConsumesReceiver("self", typ.Self, receiver) {
		t.Fatal("ParamConsumesReceiver(self) = false, want true")
	}
	if !ReceiverTypeUsable(receiver) {
		t.Fatal("ReceiverTypeUsable(record) = false, want true")
	}
	if ReceiverTypeUsable(typ.Any) {
		t.Fatal("ReceiverTypeUsable(any) = true, want false")
	}
	if ReceiverTypeUsable(typ.Unknown) {
		t.Fatal("ReceiverTypeUsable(unknown) = true, want false")
	}
	if ReceiverTypeUsable(nil) {
		t.Fatal("ReceiverTypeUsable(nil) = true, want false")
	}
}

func TestTypeCallableOwnsUnionAndNilPolicy(t *testing.T) {
	left := typ.Func().Returns(typ.String).Build()
	right := typ.Func().Returns(typ.Integer).Build()
	if !TypeCallable(typeexpr.Union(left, right)) {
		t.Fatal("TypeCallable(function union) = false, want all-callable union accepted")
	}
	if TypeCallable(typeexpr.Union(left, typ.String)) {
		t.Fatal("TypeCallable(function|string) = true, want mixed union rejected")
	}
	if !TypeCallable(typeexpr.Optional(left)) {
		t.Fatal("TypeCallable(function?) = false, want existing callable policy preserved")
	}
	if !TypeCallableIgnoringNil(typeexpr.Optional(left)) {
		t.Fatal("TypeCallableIgnoringNil(function?) = false, want callable after nil removal")
	}
}

func TestMemberTypeOwnsStaticSegmentLookup(t *testing.T) {
	receiver := typetable.NewRecord().
		Field("run", typ.Func().Returns(typ.String).Build()).
		StaticStringIndex("mode", typ.LiteralString("auto")).
		StaticIntIndex(2, typ.Integer).
		Build()

	if got, ok := MemberType(receiver, segment.Segment{Kind: segment.SegmentField, Name: "run"}); !ok || !TypeCallable(got) {
		t.Fatalf("MemberType(.run) = %v, %v; want callable function", got, ok)
	}
	if got, ok := MemberType(receiver, segment.Segment{Kind: segment.SegmentIndexString, Name: "mode"}); !ok || !typ.TypeEquals(got, typ.LiteralString("auto")) {
		t.Fatalf("MemberType([mode]) = %v, %v; want literal auto", got, ok)
	}
	if got, ok := MemberType(receiver, segment.Segment{Kind: segment.SegmentIndexInt, Index: 2}); !ok || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("MemberType([2]) = %v, %v; want integer", got, ok)
	}
	if got, ok := MemberType(receiver, segment.Segment{Kind: segment.SegmentIndexString, Name: "missing"}); ok || got != nil {
		t.Fatalf("MemberType([missing]) = %v, %v; want missing", got, ok)
	}
}

func TestBindReceiverOwnsReceiverConsumptionPolicy(t *testing.T) {
	receiver := typetable.NewRecord().Field("id", typ.String).Build()
	withSelf := contract.FromFunctionType(typ.Func().
		Param("self", typ.Self).
		Param("name", typ.String).
		Build())
	bound := BindReceiver(withSelf, receiver, true)
	if bound.ParamCount() != 1 {
		t.Fatalf("bound ParamCount = %d, want self consumed", bound.ParamCount())
	}
	if param, ok := bound.ParamAt(0); !ok || param.Name != "name" {
		t.Fatalf("bound first param = %#v/%v, want name", param, ok)
	}

	boundWithoutPreciseReceiver := BindReceiver(withSelf, nil, true)
	if boundWithoutPreciseReceiver.ParamCount() != 1 {
		t.Fatalf("nil receiver ParamCount = %d, want implicit self consumed by syntax", boundWithoutPreciseReceiver.ParamCount())
	}

	unboundWithoutReceiverSyntax := BindReceiver(withSelf, nil, false)
	if unboundWithoutReceiverSyntax.ParamCount() != 2 {
		t.Fatalf("plain call ParamCount = %d, want implicit self retained without receiver syntax", unboundWithoutReceiverSyntax.ParamCount())
	}

	withoutReceiverSlot := contract.FromFunctionType(typ.Func().
		Param("name", typ.String).
		Build())
	unchanged := BindReceiver(withoutReceiverSlot, receiver, true)
	if unchanged.ParamCount() != 1 {
		t.Fatalf("non-receiver ParamCount = %d, want unchanged", unchanged.ParamCount())
	}
	if param, ok := unchanged.ParamAt(0); !ok || param.Name != "name" {
		t.Fatalf("unchanged first param = %#v/%v, want name", param, ok)
	}
}

func TestInferenceContributionKeyAndSegmentMatchingAreOwnedByCallContract(t *testing.T) {
	contribution := InferenceContribution{
		Type: typ.String,
		Path: []InferencePathStep{
			{Kind: InferencePathField, Name: "payload"},
			{Kind: InferencePathStaticInt, Index: 2},
			{Kind: InferencePathFunctionReturn, Index: 0},
		},
	}

	if got := InferenceContributionKey(contribution); got != ".payload[2] return 0\x00string" {
		t.Fatalf("InferenceContributionKey = %q", got)
	}
	if !InferenceContributionHasSegmentPrefix(contribution, []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentIndexInt, Index: 2},
	}) {
		t.Fatal("InferenceContributionHasSegmentPrefix = false, want field/index prefix match")
	}
	if !InferenceContributionMatchesSegments(contribution, []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentIndexInt, Index: 2},
	}) {
		t.Fatal("InferenceContributionMatchesSegments = false, want value path match")
	}
	if InferenceContributionMatchesSegments(contribution, []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
	}) {
		t.Fatal("InferenceContributionMatchesSegments = true, want false for partial value path")
	}
}

func TestGenericInferenceConflictClassificationIsOwnedByCallContract(t *testing.T) {
	if InferenceTypesConflict(typ.LiteralString("start"), typ.LiteralString("stop")) {
		t.Fatal("literal strings from the same family should not be reported as a generic conflict")
	}
	if !InferenceTypesConflict(typ.String, typ.Integer) {
		t.Fatal("string vs integer should be reported as a generic conflict")
	}

	left := typ.NewTypeParam("T", nil)
	right := typ.NewTypeParam("T", nil)
	if !SameInferenceParam(left, right) {
		t.Fatal("same-named equal type params should match")
	}
	if !InferenceParamSetContains([]*typ.TypeParam{left}, right) {
		t.Fatal("InferenceParamSetContains = false, want equal type param match")
	}
}

func TestInferenceTypeSetConflictClassificationIsOwnedByCallContract(t *testing.T) {
	if InferenceTypeSetHasConflict(nil) {
		t.Fatal("empty inferred type set should not conflict")
	}
	if InferenceTypeSetHasConflict([]typ.Type{typ.LiteralString("start"), typ.LiteralString("stop")}) {
		t.Fatal("compatible literal string family should not conflict")
	}
	if !InferenceTypeSetHasConflict([]typ.Type{typ.String, typ.Integer}) {
		t.Fatal("string/integer inferred type set should conflict")
	}
	if !InferenceTypeSetHasConflict([]typ.Type{typ.LiteralString("start"), typ.LiteralString("stop"), typ.Integer}) {
		t.Fatal("later incompatible pair should conflict")
	}
}

func TestPlanGenericInferenceConflictsOwnsGroupingAndDedup(t *testing.T) {
	stringParam := typ.NewTypeParam("T", nil)
	literalParam := typ.NewTypeParam("Literal", nil)
	trace := GenericCallTrace{Contributions: []InferenceContribution{
		{
			Param: literalParam,
			Index: 1,
			Type:  typ.LiteralString("start"),
			Path:  []InferencePathStep{{Kind: InferencePathField, Name: "kind"}},
		},
		{
			Param: literalParam,
			Index: 1,
			Type:  typ.LiteralString("stop"),
			Path:  []InferencePathStep{{Kind: InferencePathField, Name: "other"}},
		},
		{
			Param: stringParam,
			Index: 2,
			Type:  typ.String,
			Path:  []InferencePathStep{{Kind: InferencePathField, Name: "name"}},
		},
		{
			Param: stringParam,
			Index: 2,
			Type:  typ.String,
			Path:  []InferencePathStep{{Kind: InferencePathField, Name: "name"}},
		},
		{
			Param: stringParam,
			Index: 2,
			Type:  typ.Integer,
			Path:  []InferencePathStep{{Kind: InferencePathField, Name: "count"}},
		},
	}}

	got := PlanGenericInferenceConflicts(trace)
	if len(got) != 1 {
		t.Fatalf("PlanGenericInferenceConflicts produced %d conflicts, want one: %#v", len(got), got)
	}
	if got[0].Index != 2 || got[0].ParamName != "T" {
		t.Fatalf("conflict identity = %#v, want index 2 param T", got[0])
	}
	if len(got[0].Contributions) != 2 {
		t.Fatalf("contributions = %#v, want duplicate path/type removed", got[0].Contributions)
	}
	if got[0].Contributions[0].Type != typ.String || got[0].Contributions[1].Type != typ.Integer {
		t.Fatalf("contribution types = %#v, want string then integer", got[0].Contributions)
	}
}

func TestPlanGenericInferenceConflictsCapsEvidenceContributions(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	trace := GenericCallTrace{Contributions: []InferenceContribution{
		{Param: param, Index: 0, Type: typ.String, Path: []InferencePathStep{{Kind: InferencePathField, Name: "a"}}},
		{Param: param, Index: 0, Type: typ.Integer, Path: []InferencePathStep{{Kind: InferencePathField, Name: "b"}}},
		{Param: param, Index: 0, Type: typ.Boolean, Path: []InferencePathStep{{Kind: InferencePathField, Name: "c"}}},
		{Param: param, Index: 0, Type: typ.Number, Path: []InferencePathStep{{Kind: InferencePathField, Name: "d"}}},
		{Param: param, Index: 0, Type: typ.Nil, Path: []InferencePathStep{{Kind: InferencePathField, Name: "e"}}},
	}}

	got := PlanGenericInferenceConflicts(trace)
	if len(got) != 1 {
		t.Fatalf("PlanGenericInferenceConflicts produced %d conflicts, want one", len(got))
	}
	if len(got[0].Contributions) != 4 {
		t.Fatalf("contributions = %d, want cap at 4: %#v", len(got[0].Contributions), got[0].Contributions)
	}
}

func TestInferenceParamNameIsOwnedByCallContract(t *testing.T) {
	if got := InferenceParamName(typ.NewTypeParam("Event", nil)); got != "Event" {
		t.Fatalf("InferenceParamName(named) = %q", got)
	}
	if got := InferenceParamName(typ.NewTypeParam("", nil)); got != "type parameter" {
		t.Fatalf("InferenceParamName(empty) = %q", got)
	}
	if got := InferenceParamName(nil); got != "type parameter" {
		t.Fatalf("InferenceParamName(nil) = %q", got)
	}
}
