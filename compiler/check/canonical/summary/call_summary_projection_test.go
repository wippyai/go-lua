package summary_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallSummaryProjection_ReturnValuesSlotwiseJoin(t *testing.T) {
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.Number)}}},
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.String), product.FromType(typ.Boolean)}}},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 2 {
		t.Fatalf("ReturnValues len = %d, want 2", len(got))
	}
	wantSlot0 := product.Domain.Join(product.FromType(typ.Number), product.FromType(typ.String))
	if !product.Domain.Equal(got[0], wantSlot0) {
		t.Fatalf("slot 0 = %v, want %v", got[0].ProjectValue(), wantSlot0.ProjectValue())
	}
	if !product.Domain.Equal(got[1], product.FromType(typ.Boolean)) {
		t.Fatalf("slot 1 = %v, want %v", got[1].ProjectValue(), typ.Boolean)
	}
}

func TestCallSummaryProjection_ReturnValuesUseDeclaredSignatureReturns(t *testing.T) {
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				DeclaredReturns:  true,
				SignatureReturns: []typ.Type{typ.Number},
				Summary:          summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.Boolean)}},
			},
			{
				DeclaredReturns: false,
				Summary:         summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.String), product.FromType(typ.Boolean)}},
			},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 2 {
		t.Fatalf("ReturnValues len = %d, want 2", len(got))
	}
	wantSlot0 := product.Domain.Join(product.FromType(typ.Number), product.FromType(typ.String))
	if !product.Domain.Equal(got[0], wantSlot0) {
		t.Fatalf("slot 0 = %v, want %v", got[0].ProjectValue(), wantSlot0.ProjectValue())
	}
	if !product.Domain.Equal(got[1], product.FromType(typ.Boolean)) {
		t.Fatalf("slot 1 = %v, want %v", got[1].ProjectValue(), typ.Boolean)
	}
}

func TestCallSummaryProjection_DeclaredStructuralReturnUsesCompatibleBodyEvidence(t *testing.T) {
	rowAnnotation := typ.NewMap(typ.String, typ.Any)
	rowEvidence := typ.NewRecord().
		Field("count", typ.Integer).
		Field("exists", typ.Boolean).
		Build()
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				DeclaredReturns:  true,
				SignatureReturns: []typ.Type{typ.NewOptional(typ.NewArray(rowAnnotation))},
				Summary:          summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.NewArray(rowEvidence))}},
			},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 1 {
		t.Fatalf("ReturnValues len = %d, want 1", len(got))
	}
	opt, ok := got[0].ProjectValue().(*typ.Optional)
	if !ok {
		t.Fatalf("slot 0 = %T %[1]v, want optional array", got[0].ProjectValue())
	}
	arr, ok := opt.Inner.(*typ.Array)
	if !ok {
		t.Fatalf("slot 0 inner = %T %[1]v, want array", opt.Inner)
	}
	row, ok := arr.Element.(*typ.Record)
	if !ok {
		t.Fatalf("array element = %T %[1]v, want refined row record", arr.Element)
	}
	if !row.HasMapComponent() {
		t.Fatalf("refined row = %v, want declared map component preserved", row)
	}
	count := row.GetField("count")
	if count == nil || count.Optional || !typ.TypeEquals(count.Type, typ.Integer) {
		t.Fatalf("count field = %v, want required integer", count)
	}
}

func TestCallSummaryProjection_DeclaredAnyReturnRemainsCarrierValue(t *testing.T) {
	body := typ.NewRecord().Field("count", typ.Integer).Build()
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				DeclaredReturns:  true,
				SignatureReturns: []typ.Type{typ.Any},
				Summary:          summary.Summary{Returns: []product.AbstractValue{product.FromType(body)}},
			},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 1 {
		t.Fatalf("ReturnValues len = %d, want 1", len(got))
	}
	if got[0].IsZero() {
		t.Fatal("declared any return remained a zero product slot")
	}
	if projected := got[0].ProjectValue(); !typ.IsAny(projected) {
		t.Fatalf("slot 0 = %v, want declared any", projected)
	}
}

func TestCallSummaryProjection_OpenGenericDeclaredReturnKeepsSolvedSummary(t *testing.T) {
	// Open generic returns such as `apply<T,U>(...): U` are binder relations.
	// The target metadata must leave DeclaredReturns false for that shape so the
	// exact-context solved summary, not the broad signature fallback, owns the
	// product return value.
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				DeclaredReturns:  false,
				SignatureReturns: []typ.Type{typ.Number},
				Summary:          summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.Integer)}},
			},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 1 {
		t.Fatalf("ReturnValues len = %d, want 1", len(got))
	}
	if !product.Domain.Equal(got[0], product.FromType(typ.Integer)) {
		t.Fatalf("slot 0 = %v, want solved integer summary", got[0].ProjectValue())
	}
}

func TestCallSummaryProjection_OpenGenericReturnRepairsWithSignatureFallback(t *testing.T) {
	open := typ.NewTypeParam("T", nil)
	summaryRec := typ.NewRecord().
		Field("value", typ.LiteralString("hello")).
		Field("get", typ.Func().OptParam("self", typ.Any).Returns(open).Build()).
		Build()
	boxParam := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam},
		typ.NewRecord().
			Field("value", boxParam).
			Field("get", typ.Func().OptParam("self", typ.Self).Returns(boxParam).Build()).
			Build(),
	)
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				DeclaredReturns:  false,
				SignatureReturns: []typ.Type{typ.Instantiate(box, typ.String)},
				Summary:          summary.Summary{Returns: []product.AbstractValue{product.FromType(summaryRec)}},
			},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 1 {
		t.Fatalf("ReturnValues len = %d, want 1", len(got))
	}
	rec, ok := got[0].ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("slot 0 = %v, want repaired record", got[0].ProjectValue())
	}
	get := rec.GetField("get")
	if get == nil {
		t.Fatal("missing get field")
	}
	fn, ok := get.Type.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestCallSummaryProjection_NonDeclaredReturnDoesNotReplaceWholeSlotWithFallback(t *testing.T) {
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				DeclaredReturns:  false,
				SignatureReturns: []typ.Type{typ.LiteralString("signature-only")},
				Summary:          summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.String)}},
			},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 1 {
		t.Fatalf("ReturnValues len = %d, want 1", len(got))
	}
	if !typ.TypeEquals(got[0].ProjectValue(), typ.String) {
		t.Fatalf("slot 0 = %v, want solved summary string", got[0].ProjectValue())
	}
}

func TestCallSummaryProjection_DeclaredUnknownReturnsAreCarrierValues(t *testing.T) {
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				DeclaredReturns:  true,
				SignatureReturns: []typ.Type{typ.Unknown},
			},
			{
				DeclaredReturns: false,
				Summary:         summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.String)}},
			},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 1 {
		t.Fatalf("ReturnValues len = %d, want 1", len(got))
	}
	if got[0].IsZero() {
		t.Fatal("declared unknown return remained a zero product slot")
	}
	want := product.Domain.Join(product.FromType(typ.Unknown), product.FromType(typ.String))
	if !product.Domain.Equal(got[0], want) {
		t.Fatalf("slot 0 = %v, want %v", got[0].ProjectValue(), want.ProjectValue())
	}
	if projected := projection.InferredReturnTypes(); len(projected) != 1 || !typ.IsUnknown(projected[0]) {
		t.Fatalf("InferredReturnTypes = %v, want [unknown]", projected)
	}
}

func TestCallSummaryProjection_ZeroSummarySlotsNormalizeToBottom(t *testing.T) {
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.AbstractValue{}}}},
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.Number)}}},
		},
	}

	got := projection.ReturnValues()
	if len(got) != 1 {
		t.Fatalf("ReturnValues len = %d, want 1", len(got))
	}
	if !product.Domain.Equal(got[0], product.FromType(typ.Number)) {
		t.Fatalf("slot 0 = %v, want number", got[0].ProjectValue())
	}
}

func TestCallSummaryProjection_InferredReturnTypesUseDeclaredTargets(t *testing.T) {
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.Number)}}},
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.String)}}},
		},
	}
	got := projection.InferredReturnTypes()
	if len(got) != 1 {
		t.Fatalf("InferredReturnTypes len = %d, want 1", len(got))
	}
	want := product.Domain.Join(product.FromType(typ.Number), product.FromType(typ.String)).ProjectValue()
	if !typ.TypeEquals(got[0], want) {
		t.Fatalf("InferredReturnTypes slot 0 = %s, want %s", got[0], want)
	}

	withDeclared := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.Number)}}},
			{
				DeclaredReturns:  true,
				SignatureReturns: []typ.Type{typ.String},
				Summary:          summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.Boolean)}},
			},
		},
	}
	got = withDeclared.InferredReturnTypes()
	if len(got) != 1 {
		t.Fatalf("InferredReturnTypes with declared target len = %d, want 1", len(got))
	}
	want = product.Domain.Join(product.FromType(typ.Number), product.FromType(typ.String)).ProjectValue()
	if !typ.TypeEquals(got[0], want) {
		t.Fatalf("InferredReturnTypes with declared target slot 0 = %s, want %s", got[0], want)
	}
	missingDeclared := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				DeclaredReturns: true,
				Summary:         summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.String)}},
			},
		},
	}
	if got := missingDeclared.InferredReturnTypes(); len(got) != 0 {
		t.Fatalf("missing declared signature returns = %#v, want nil fallback", got)
	}
}

func TestCallSummaryProjection_ReturnFunctionRefsFold(t *testing.T) {
	refA := flow.FunctionRef{GraphID: 1}
	refB := flow.FunctionRef{GraphID: 2}
	refC := flow.FunctionRef{GraphID: 3}
	slot0A := flow.WithFunctionRef(nil, constraint.NewPlaceholder(0).Field("a").Key(), flow.FunctionRefSetOf(refA))
	slot0B := flow.WithFunctionRef(nil, constraint.NewPlaceholder(0).Field("b").Key(), flow.FunctionRefSetOf(refB))
	slot2 := flow.WithFunctionRef(nil, constraint.NewPlaceholder(2).Field("c").Key(), flow.FunctionRefSetOf(refC))

	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{ReturnFunctionRefs: []flow.FunctionRefs{slot0A}}},
			{Summary: summary.Summary{ReturnFunctionRefs: []flow.FunctionRefs{slot0B, flow.FunctionRefsDomain.Bottom(), slot2}}},
		},
	}

	got := projection.ReturnFunctionRefs()
	if len(got) != 3 {
		t.Fatalf("ReturnFunctionRefs len = %d, want 3", len(got))
	}
	want0 := flow.FunctionRefsDomain.Join(slot0A, slot0B)
	if !flow.FunctionRefsDomain.Equal(got[0], want0) {
		t.Fatalf("slot 0 = %#v, want %#v", got[0], want0)
	}
	if !flow.FunctionRefsDomain.Equal(got[1], flow.FunctionRefsDomain.Bottom()) {
		t.Fatalf("slot 1 = %v, want bottom", got[1])
	}
	if !flow.FunctionRefsDomain.Equal(got[2], slot2) {
		t.Fatalf("slot 2 = %#v, want %#v", got[2], slot2)
	}
}

func TestCallSummaryProjection_ReturnClosureRefsFold(t *testing.T) {
	closureA := flow.ClosureRefOf(flow.FunctionRef{GraphID: 1}, flow.CaptureCellsDomain.Bottom(), nil)
	closureB := flow.ClosureRefOf(flow.FunctionRef{GraphID: 2}, flow.CaptureCellsDomain.Bottom(), nil)
	slot0A := flow.WithClosureRef(nil, constraint.NewPlaceholder(0).Field("a").Key(), flow.ClosureRefSetOf(closureA))
	slot0B := flow.WithClosureRef(nil, constraint.NewPlaceholder(0).Field("b").Key(), flow.ClosureRefSetOf(closureB))
	slot2 := flow.WithClosureRef(nil, constraint.NewPlaceholder(2).Field("c").Key(), flow.ClosureRefSetOf(closureA))

	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{ReturnClosureRefs: []flow.ClosureRefs{slot0A}}},
			{Summary: summary.Summary{ReturnClosureRefs: []flow.ClosureRefs{slot0B, flow.ClosureRefsDomain.Bottom(), slot2}}},
		},
	}

	got := projection.ReturnClosureRefs()
	if len(got) != 3 {
		t.Fatalf("ReturnClosureRefs len = %d, want 3", len(got))
	}
	want0 := flow.ClosureRefsDomain.Join(slot0A, slot0B)
	if !flow.ClosureRefsDomain.Equal(got[0], want0) {
		t.Fatalf("slot 0 = %#v, want %#v", got[0], want0)
	}
	if !flow.ClosureRefsDomain.Equal(got[1], flow.ClosureRefsDomain.Bottom()) {
		t.Fatalf("slot 1 = %v, want bottom", got[1])
	}
	if !flow.ClosureRefsDomain.Equal(got[2], slot2) {
		t.Fatalf("slot 2 = %#v, want %#v", got[2], slot2)
	}
}

func TestCallSummaryProjection_EmptyOrBottomFoldBehavior(t *testing.T) {
	returnsProjection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.Domain.Bottom()}}},
			{Summary: summary.Summary{Returns: []product.AbstractValue{product.Domain.Bottom()}}},
		},
	}
	if got := returnsProjection.ReturnValues(); len(got) != 0 {
		t.Fatalf("ReturnValues for all-bottom inputs = %#v, want empty", got)
	}

	functionRefsProjection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{ReturnFunctionRefs: []flow.FunctionRefs{flow.FunctionRefsDomain.Bottom()}}},
			{Summary: summary.Summary{ReturnFunctionRefs: []flow.FunctionRefs{flow.FunctionRefsDomain.Bottom()}}},
		},
	}
	if got := functionRefsProjection.ReturnFunctionRefs(); len(got) != 0 {
		t.Fatalf("ReturnFunctionRefs for all-bottom inputs = %#v, want empty", got)
	}

	closureRefsProjection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{ReturnClosureRefs: []flow.ClosureRefs{flow.ClosureRefsDomain.Bottom()}}},
			{Summary: summary.Summary{ReturnClosureRefs: []flow.ClosureRefs{flow.ClosureRefsDomain.Bottom()}}},
		},
	}
	if got := closureRefsProjection.ReturnClosureRefs(); len(got) != 0 {
		t.Fatalf("ReturnClosureRefs for all-bottom inputs = %#v, want empty", got)
	}
}

func TestCallSummaryProjection_CellEffects(t *testing.T) {
	effectA := flow.CaptureMustWrite(cfg.SymbolID(10), product.FromType(typ.String))
	effectB := flow.CaptureMustWrite(cfg.SymbolID(11), product.FromType(typ.Boolean))

	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{CellEffects: effectA}},
			{Summary: summary.Summary{CellEffects: effectB}},
		},
	}

	got := projection.CellEffects()
	want := flow.CooccurringCaptureEffects(effectA, effectB)
	if !flow.CaptureEffectsDomain.Equal(got, want) {
		t.Fatalf("CellEffects = %s, want %s", got.Format(), want.Format())
	}
}

func TestCallSummaryProjection_CellEffectsEmptyIsBottom(t *testing.T) {
	projection := summary.CallSummaryProjection{}
	got := projection.CellEffects()
	if !flow.CaptureEffectsDomain.Equal(got, flow.CaptureEffectsDomain.Bottom()) {
		t.Fatalf("CellEffects = %s, want bottom", got.Format())
	}
}

func TestCallSummaryProjection_ReturnRelationsFromSummary(t *testing.T) {
	summaryRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 2}})
	fallbackRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 2, ErrorIndex: 3}})
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				Summary:            summary.Summary{Relations: summaryRel},
				SignatureRelations: fallbackRel,
			},
		},
	}
	got := projection.ReturnRelations()
	if !flow.ReturnRelationsDomain.Equal(got, summaryRel) {
		t.Fatalf("ReturnRelations = %#v, want %#v", got, summaryRel)
	}
}

func TestCallSummaryProjection_ReturnRelationsUsesSignatureFallback(t *testing.T) {
	fallbackRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				Summary:            summary.Summary{Relations: flow.ReturnRelationsDomain.Top()},
				SignatureRelations: fallbackRel,
			},
		},
	}
	got := projection.ReturnRelations()
	if !flow.ReturnRelationsDomain.Equal(got, fallbackRel) {
		t.Fatalf("ReturnRelations = %#v, want %#v", got, fallbackRel)
	}
}

func TestCallSummaryProjection_ReturnRelationsTreatsLengthParamAsProof(t *testing.T) {
	summaryRel := flow.ReturnRelationsOfLengthParams([]flow.ReturnLengthParamRelation{{ReturnIndex: 0, ParamIndex: 1}})
	fallbackRel := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{
				Summary:            summary.Summary{Relations: summaryRel},
				SignatureRelations: fallbackRel,
			},
		},
	}

	got := projection.ReturnRelations()
	if !got.HasLengthParam(flow.ReturnLengthParamRelation{ReturnIndex: 0, ParamIndex: 1}) {
		t.Fatalf("ReturnRelations = %#v, want summary length-param proof", got)
	}
	if len(got.ErrorReturns()) != 0 {
		t.Fatalf("ReturnRelations used fallback despite summary proof: %#v", got)
	}
}

func TestCallSummaryProjection_ReturnRelationsJoinIsMustOrTopWhenNoProof(t *testing.T) {
	relA := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{
		{ValueIndex: 0, ErrorIndex: 1},
		{ValueIndex: 0, ErrorIndex: 2},
	})
	relB := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})

	projection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Relations: relA}},
			{Summary: summary.Summary{Relations: relB}},
			{Summary: summary.Summary{Relations: relB}},
		},
	}
	got := projection.ReturnRelations()
	want := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
	if !flow.ReturnRelationsDomain.Equal(got, want) {
		t.Fatalf("ReturnRelations = %#v, want %#v", got, want)
	}

	topProjection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Relations: relA}},
			{Summary: summary.Summary{Relations: relB}},
			{Summary: summary.Summary{Relations: flow.ReturnRelationsDomain.Top()}},
		},
	}
	got = topProjection.ReturnRelations()
	if !flow.ReturnRelationsDomain.Equal(got, flow.ReturnRelationsDomain.Top()) {
		t.Fatalf("ReturnRelations = %#v, want Top", got)
	}

	bottomFirstProjection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Relations: flow.ReturnRelationsDomain.Bottom()}},
			{Summary: summary.Summary{Relations: relA}},
		},
	}
	got = bottomFirstProjection.ReturnRelations()
	if !flow.ReturnRelationsDomain.Equal(got, flow.ReturnRelationsDomain.Top()) {
		t.Fatalf("ReturnRelations = %#v, want Top when first target has no finite proof", got)
	}
}

func TestCallSummaryProjection_ReturnRelationsEmptyOrBottomBecomesTop(t *testing.T) {
	got := summary.CallSummaryProjection{}.ReturnRelations()
	if !flow.ReturnRelationsDomain.Equal(got, flow.ReturnRelationsDomain.Top()) {
		t.Fatalf("empty projection ReturnRelations = %#v, want Top", got)
	}

	noProofProjection := summary.CallSummaryProjection{
		Targets: []summary.CallSummaryTarget{
			{Summary: summary.Summary{Relations: flow.ReturnRelationsDomain.Top()}},
			{Summary: summary.Summary{Relations: flow.ReturnRelationsDomain.Bottom()}},
		},
	}
	if got := noProofProjection.ReturnRelations(); !flow.ReturnRelationsDomain.Equal(got, flow.ReturnRelationsDomain.Top()) {
		t.Fatalf("no-proof projection ReturnRelations = %#v, want Top", got)
	}
}
