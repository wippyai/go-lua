package manifesttarget_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/typ"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// This file adopts the continuation half of the wire vocabulary: the retained
// sources a produced callable captures, the lifecycle of a provider-invoked
// callback, and the multiplicity of a suspension's reentry.

// declaredCallbackLifecycles names every value in the wire callback lifecycle
// vocabulary. The three axes are independent: a callback is invoked either
// only during the call (Sync) or possibly after it returns (Retained); the
// provider either must invoke it (Required) or may not (Optional); and it may
// be invoked at most once (Once) or repeatedly (Many).
func declaredCallbackLifecycles() map[string]manifestwire.CallbackLifecycle {
	return map[string]manifestwire.CallbackLifecycle{
		"sync-optional-once":     manifestwire.CallbackSyncOptionalOnce,
		"sync-required-once":     manifestwire.CallbackSyncRequiredOnce,
		"sync-optional-many":     manifestwire.CallbackSyncOptionalMany,
		"sync-required-many":     manifestwire.CallbackSyncRequiredMany,
		"retained-optional-once": manifestwire.CallbackRetainedOptionalOnce,
		"retained-required-once": manifestwire.CallbackRetainedRequiredOnce,
		"retained-optional-many": manifestwire.CallbackRetainedOptionalMany,
		"retained-required-many": manifestwire.CallbackRetainedRequiredMany,
	}
}

// callbackTerminals states the five activation outcomes every declared
// callback must terminate through. The throw and cancel arms carry the one
// value the subedge routes onward under a preserving adjustment, which by
// definition may not change the vector it carries.
func callbackTerminals() []manifestwire.Terminal {
	empty := manifestwire.Values{Tail: manifestwire.ValuesClosed}
	return []manifestwire.Terminal{
		{Kind: manifestwire.OutcomeNormal, Values: empty},
		{Kind: manifestwire.OutcomeReturn, Values: empty},
		{Kind: manifestwire.OutcomeThrow, Values: anyClosed()},
		{Kind: manifestwire.OutcomeYield, Values: empty},
		{Kind: manifestwire.OutcomeCancel, Values: anyClosed()},
	}
}

// anyClosed is the one-value closed vector the subedge terminals below carry.
func anyClosed() manifestwire.Values {
	return manifestwire.Values{Fixed: []typ.Type{typ.Any}, Tail: manifestwire.ValuesClosed}
}

// syncCallbackSubedge is the direct application a Sync callback is invoked
// through. A Sync lifecycle promises the callback runs during the call, so the
// declaration must also state the call site that runs it; a Retained callback
// makes no such promise and needs no direct subedge.
//
// A callback callee is a closed union: the callback already owns its admission,
// its arguments and the outcomes it terminates through, so the subedge states
// only the routing of those outcomes back into the operation.
func syncCallbackSubedge(throwOutcome, cancelOutcome uint32) manifestwire.Subedge {
	return manifestwire.Subedge{
		Role:   1,
		Family: manifestwire.SubedgeFamilyCall,
		Callee: manifestwire.SubedgeCallee{Kind: manifestwire.SubedgeCalleeCallback, Callback: 1},
		// No sibling edge routes into this one, so the operation's own rule is
		// its entry authority.
		RuleEntry: true,
		AdmissionFailure: manifestwire.AdmissionFailure{
			Values: anyClosed(),
			Route: manifestwire.AdmissionRoute{
				Route: manifestwire.SubedgeRouteOutcome, Adjustment: manifestwire.AdjustmentPreserve,
				Result: anyClosed(), Placement: manifestwire.PlacementFixed, Outcome: throwOutcome,
			},
		},
		Routes: []manifestwire.SubedgeRoute{
			{Kind: manifestwire.OutcomeNormal, Route: manifestwire.SubedgeRouteContinue, Adjustment: manifestwire.AdjustmentExact, Result: anyClosed()},
			{Kind: manifestwire.OutcomeReturn, Route: manifestwire.SubedgeRouteContinue, Adjustment: manifestwire.AdjustmentExact, Result: anyClosed()},
			{Kind: manifestwire.OutcomeThrow, Route: manifestwire.SubedgeRouteOutcome, Adjustment: manifestwire.AdjustmentPreserve, Result: anyClosed(), Placement: manifestwire.PlacementFixed, Outcome: throwOutcome},
			{Kind: manifestwire.OutcomeYield, Route: manifestwire.SubedgeRouteRejectYield, Adjustment: manifestwire.AdjustmentExact, Result: rejectedYieldValues(), Placement: manifestwire.PlacementFixed, Outcome: throwOutcome},
			{Kind: manifestwire.OutcomeCancel, Route: manifestwire.SubedgeRouteOutcome, Adjustment: manifestwire.AdjustmentPreserve, Result: anyClosed(), Placement: manifestwire.PlacementFixed, Outcome: cancelOutcome},
		},
	}
}

// rejectedYieldValues is the exact refusal a non-yieldable native boundary
// answers when a callback attempts to yield across it.
func rejectedYieldValues() manifestwire.Values {
	return manifestwire.Values{
		Fixed: []typ.Type{typ.LiteralString("attempt to yield across a C-call boundary")},
		Tail:  manifestwire.ValuesClosed,
	}
}

func lifecycleHostManifest(lifecycle manifestwire.CallbackLifecycle) *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	callbackType := typ.Func().Returns(typ.Any).Build()
	memberType := typ.Func().Param("callback", callbackType).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature("invokes", signature.Function{Type: memberType})
	empty := manifestwire.Values{Tail: manifestwire.ValuesClosed}
	operation := manifestwire.Operation{
		Replace: true,
		Input:   manifestwire.Values{Fixed: []typ.Type{callbackType}, Tail: manifestwire.ValuesClosed},
		// The throw and cancel arms carry one value slot each, because the
		// subedge below routes a one-value result into them at a fixed
		// placement and a destination with no slot could not receive it.
		Outcomes: []manifestwire.Outcome{
			{Kind: manifestwire.OutcomeNormal, Values: anyClosed()},
			{Kind: manifestwire.OutcomeThrow, Values: anyClosed()},
			{Kind: manifestwire.OutcomeCancel, Values: anyClosed()},
		},
		Callbacks: []manifestwire.Callback{{
			Function:  manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
			Admission: manifestwire.CallableAdmissionOrdinary,
			Arguments: empty,
			Outcomes:  callbackTerminals(),
			Lifecycle: lifecycle,
			Effects:   manifestwire.RowSpec{Tail: manifestwire.RowClosed},
		}},
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	}
	if !retainedLifecycle(lifecycle) {
		operation.Subedges = []manifestwire.Subedge{syncCallbackSubedge(1, 2)}
	}
	declaration.DefineFunctionOperation("invokes", operation)
	return declaration
}

// retainedLifecycle reports the half of the lifecycle vocabulary that makes no
// during-the-call promise. It mirrors the seal's own span rather than naming
// the four values twice.
func retainedLifecycle(lifecycle manifestwire.CallbackLifecycle) bool {
	return lifecycle >= manifestwire.CallbackRetainedOptionalOnce &&
		lifecycle <= manifestwire.CallbackRetainedRequiredMany
}

// TestManifestDeclaresEveryCallbackLifecycle is the positive law: every
// lifecycle the wire vocabulary offers is declared by a real manifest and
// answered by the sealed Target on the callback that carries it.
func TestManifestDeclaresEveryCallbackLifecycle(t *testing.T) {
	for name, lifecycle := range declaredCallbackLifecycles() {
		t.Run(name, func(t *testing.T) {
			contract, err := sealRelationCatalogue(lifecycleHostManifest(lifecycle))
			if err != nil {
				t.Fatal(err)
			}
			operation, ok := contract.Operations.Lookup(relationBinding("invokes"))
			if !ok {
				t.Fatal("sealed Target holds no invokes operation")
			}
			callback, ok := contract.Operations.CallbackAt(operation, 0)
			if !ok {
				t.Fatal("sealed Target holds no callback for invokes")
			}
			sealed, sealedOK := contract.Operations.CallbackLifecycle(callback)
			if !sealedOK || sealed != vocabulary.CallbackLifecycle(lifecycle) {
				t.Fatalf("callback lifecycle = %d/%t, want %d", sealed, sealedOK, lifecycle)
			}
		})
	}
}

// A lifecycle outside the declared vocabulary is refused by name rather than
// silently narrowed to a neighbouring one.
func TestManifestRefusesAnUndeclaredCallbackLifecycle(t *testing.T) {
	beyond := manifestwire.CallbackRetainedRequiredMany + 1
	if _, err := sealRelationCatalogue(lifecycleHostManifest(beyond)); err == nil {
		t.Fatal("a manifest declaring a lifecycle outside the vocabulary sealed, want a refusal")
	}
	if _, err := sealRelationCatalogue(lifecycleHostManifest(manifestwire.CallbackLifecycleInvalid)); err == nil {
		t.Fatal("a manifest declaring the invalid lifecycle sealed, want a refusal")
	}
}

// declaredReentryMultiplicities names both values of the wire reentry
// multiplicity vocabulary. A suspension either resumes at most once or may
// resume repeatedly, and the sealed Target must answer the one declared.
func declaredReentryMultiplicities() map[string]manifestwire.ReentryMultiplicity {
	return map[string]manifestwire.ReentryMultiplicity{
		"once": manifestwire.ReentryOnce,
		"many": manifestwire.ReentryMany,
	}
}

func suspensionHostManifest(multiplicity manifestwire.ReentryMultiplicity) *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	memberType := typ.Func().Param("value", typ.Any).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature("suspends", signature.Function{Type: memberType})
	empty := manifestwire.Values{Tail: manifestwire.ValuesClosed}
	declaration.DefineFunctionOperation("suspends", manifestwire.Operation{
		Replace: true,
		Input:   manifestwire.Values{Fixed: []typ.Type{typ.Any}, Tail: manifestwire.ValuesClosed},
		Outcomes: []manifestwire.Outcome{
			{Kind: manifestwire.OutcomeNormal, Values: manifestwire.Values{Fixed: []typ.Type{typ.Any}, Tail: manifestwire.ValuesClosed}},
			{Kind: manifestwire.OutcomeYield, Values: empty},
		},
		Suspensions: []manifestwire.Suspension{{
			Yield:        1,
			Reentry:      0,
			Source:       manifestwire.ReentryByCall,
			Multiplicity: multiplicity,
		}},
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})
	return declaration
}

// TestManifestDeclaresEveryReentryMultiplicity is the positive law for the
// suspension reentry vocabulary.
func TestManifestDeclaresEveryReentryMultiplicity(t *testing.T) {
	for name, multiplicity := range declaredReentryMultiplicities() {
		t.Run(name, func(t *testing.T) {
			contract, err := sealRelationCatalogue(suspensionHostManifest(multiplicity))
			if err != nil {
				t.Fatal(err)
			}
			operation, ok := contract.Operations.Lookup(relationBinding("suspends"))
			if !ok {
				t.Fatal("sealed Target holds no suspends operation")
			}
			if count := contract.Operations.SuspensionCount(operation); count != 1 {
				t.Fatalf("suspension count = %d, want the one declared suspension", count)
			}
			_, _, source, sealed, ok := contract.Operations.SuspensionAt(operation, 0)
			if !ok || sealed != vocabulary.ReentryMultiplicity(multiplicity) {
				t.Fatalf("suspension multiplicity = %d/%t, want %d", sealed, ok, multiplicity)
			}
			if source != vocabulary.ReentryByCall {
				t.Fatalf("suspension source = %d, want the declared by-call source", source)
			}
		})
	}
}

// A multiplicity outside the declared vocabulary is refused by name.
func TestManifestRefusesAnUndeclaredReentryMultiplicity(t *testing.T) {
	_, err := sealRelationCatalogue(suspensionHostManifest(manifestwire.ReentryMany + 1))
	if err == nil {
		t.Fatal("a manifest declaring a multiplicity outside the vocabulary sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "invalid multiplicity") {
		t.Fatalf("refusal = %v, want the named invalid-multiplicity refusal", err)
	}
}

// captureHostManifest declares a produced iterator whose callable result
// retains the sources the wire capture vocabulary names. A TypeValue capture
// states the identity of the produced callable itself, so the seal also
// requires the same result to be the outcome's nominal fresh function.
func captureHostManifest(captures []manifestwire.Capture) *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	stepType := typ.Func().Returns(typ.Any).Build()
	memberType := typ.Func().Param("subject", typ.Any).Variadic(typ.Any).Returns(stepType).Build()
	declaration.DefineFunctionSignature("step", signature.Function{Type: stepType})
	declaration.DefineFunctionSignature("produces", signature.Function{Type: memberType})
	declaration.DefineFunctionOperation("produces", manifestwire.Operation{
		Replace:    true,
		ValuesVars: 1,
		Input: manifestwire.Values{
			Fixed: []typ.Type{typ.Any}, Tail: manifestwire.ValuesVariable, Var: 0, TailType: typ.Any,
		},
		Outcomes: []manifestwire.Outcome{{
			Kind:   manifestwire.OutcomeNormal,
			Values: manifestwire.Values{Fixed: []typ.Type{stepType}, Tail: manifestwire.ValuesClosed},
			Produced: []manifestwire.Produced{{
				Result: 0, Operation: relationHostModule + ".step", Captures: captures,
			}},
			FreshResults: []manifestwire.FreshResult{{Result: 0, Class: manifestwire.FreshFunction}},
		}, {
			Kind: manifestwire.OutcomeThrow, Values: manifestwire.Values{Tail: manifestwire.ValuesClosed},
		}},
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})
	return declaration
}

// TestManifestDeclaresEveryCaptureKind is the positive law for the retained
// source vocabulary: a produced callable may retain one fixed input value, the
// runtime type value of one fixed input, and the open input pack, and the
// sealed Target answers each retained source in the order it was declared.
func TestManifestDeclaresEveryCaptureKind(t *testing.T) {
	declared := []manifestwire.Capture{
		{Kind: manifestwire.CaptureValue, Ordinal: 0},
		{Kind: manifestwire.CaptureTypeValue, Ordinal: 0},
		{Kind: manifestwire.CaptureValues, Ordinal: 0},
	}
	want := []vocabulary.CaptureKind{
		vocabulary.CaptureValueFormal,
		vocabulary.CaptureTypeValueFormal,
		vocabulary.CaptureValuesVar,
	}
	contract, err := sealRelationCatalogue(captureHostManifest(declared))
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := contract.Operations.Lookup(relationBinding("produces"))
	if !ok {
		t.Fatal("sealed Target holds no produces operation")
	}
	if count := contract.Operations.ProducedCount(operation, 0); count != 1 {
		t.Fatalf("produced count = %d, want the one declared iterator", count)
	}
	if count := contract.Operations.ProducedCaptureCount(operation, 0, 0); count != len(declared) {
		t.Fatalf("capture count = %d, want %d", count, len(declared))
	}
	for index, wantKind := range want {
		kind, ordinal, captureOK := contract.Operations.ProducedCaptureAt(operation, 0, 0, index)
		if !captureOK || kind != wantKind || ordinal != 0 {
			t.Fatalf("capture %d = kind %d ordinal %d (ok %t), want kind %d ordinal 0",
				index, kind, ordinal, captureOK, wantKind)
		}
	}
	// The retained runtime TypeValue is addressable on its own, because it
	// names the identity of the produced callable rather than a plain value.
	formal, formalOK := contract.Operations.ProducedTypeValueCapture(operation, 0, 0)
	if !formalOK || formal != 0 {
		t.Fatalf("TypeValue capture formal = %d/%t, want the declared input 0", formal, formalOK)
	}
	_, class, _, freshOK := contract.Operations.FreshResultForResult(operation, 0, 0)
	if !freshOK || class != schematype.FreshClassFunction {
		t.Fatalf("produced result fresh class = %d/%t, want the nominal fresh function", class, freshOK)
	}
}

// A retained TypeValue states the identity of the produced callable, so the
// seal requires that exact result to be the outcome's fresh function. Dropping
// the fresh relation is refused by name rather than leaving the retained
// identity unanchored.
func TestManifestTypeValueCaptureRequiresItsFreshFunction(t *testing.T) {
	declaration := captureHostManifest([]manifestwire.Capture{{Kind: manifestwire.CaptureTypeValue, Ordinal: 0}})
	// Re-declare the operation without the nominal fresh-function relation.
	stepType := typ.Func().Returns(typ.Any).Build()
	declaration.DefineFunctionOperation("produces", manifestwire.Operation{
		Replace:    true,
		ValuesVars: 1,
		Input: manifestwire.Values{
			Fixed: []typ.Type{typ.Any}, Tail: manifestwire.ValuesVariable, Var: 0, TailType: typ.Any,
		},
		Outcomes: []manifestwire.Outcome{{
			Kind:   manifestwire.OutcomeNormal,
			Values: manifestwire.Values{Fixed: []typ.Type{stepType}, Tail: manifestwire.ValuesClosed},
			Produced: []manifestwire.Produced{{
				Result: 0, Operation: relationHostModule + ".step",
				Captures: []manifestwire.Capture{{Kind: manifestwire.CaptureTypeValue, Ordinal: 0}},
			}},
		}, {
			Kind: manifestwire.OutcomeThrow, Values: manifestwire.Values{Tail: manifestwire.ValuesClosed},
		}},
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})
	_, err := sealRelationCatalogue(declaration)
	if err == nil {
		t.Fatal("a TypeValue capture without its fresh function sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "lacks FreshFunction") {
		t.Fatalf("refusal = %v, want the named missing-FreshFunction refusal", err)
	}
}

// Each capture ordinal addresses a coordinate the declaring operation really
// has. An ordinal past the declared arity is refused rather than clamped.
func TestManifestCaptureOrdinalsAddressDeclaredCoordinates(t *testing.T) {
	for name, capture := range map[string]manifestwire.Capture{
		"value":      {Kind: manifestwire.CaptureValue, Ordinal: 9},
		"type-value": {Kind: manifestwire.CaptureTypeValue, Ordinal: 9},
		"values":     {Kind: manifestwire.CaptureValues, Ordinal: 9},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := sealRelationCatalogue(captureHostManifest([]manifestwire.Capture{capture}))
			if err == nil {
				t.Fatalf("capture %s with an out-of-scope ordinal sealed, want a named refusal", name)
			}
			if !strings.Contains(err.Error(), "outside scope") {
				t.Fatalf("refusal = %v, want the named out-of-scope refusal", err)
			}
		})
	}
}

// A Lua Values relation is a fixed prefix, an optional variable-length middle,
// and an end-anchored suffix. ArgumentSuffix is the argument coordinate for
// that last segment: the operands a native rule places after the caller's
// forwarded pack, counted from the end of the applied vector. A prefix operand
// is addressed by its index and the pack by its variable, so an operand that
// sits behind a variable-length middle has no other spelling, and without it
// an argument vector carrying one could never receive the complete origin set
// every applied vector requires.
//
// The segment is meaningful only behind a variable middle. A closed vector has
// no end-relative coordinate, so the seal canonicalizes its authored suffix
// into the prefix and the same operand is then addressed as ArgumentFixed.
//
// suffixApplicationHostManifest declares one host member that applies its
// callback with exactly that shape: the caller's subject, the caller's
// forwarded pack, and one trailing operand the member's own rule constructs.
func suffixApplicationHostManifest(mutate func(*manifestwire.Operation)) *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	callbackType := typ.Func().Param("subject", typ.Any).Variadic(typ.Any).Returns(typ.Any).Build()
	memberType := typ.Func().
		Param("handler", callbackType).Param("subject", typ.Number).Variadic(typ.Any).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature("folds", signature.Function{Type: memberType})
	subedge := syncCallbackSubedge(1, 2)
	subedge.RuleEntry = false
	subedge.ArgumentOrigins = []manifestwire.ArgumentOrigin{
		{Segment: manifestwire.ArgumentFixed, Index: 0, Kind: manifestwire.ArgumentSourceInput,
			Source: manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 1}},
		{Segment: manifestwire.ArgumentTail, Index: 0, Kind: manifestwire.ArgumentSourceInput,
			Source: manifestwire.InputSource{Kind: manifestwire.InputSourceValues, Ordinal: 0}},
		{Segment: manifestwire.ArgumentSuffix, Index: 0, Kind: manifestwire.ArgumentSourceRule},
	}
	operation := manifestwire.Operation{
		Replace:    true,
		ValuesVars: 1,
		Input: manifestwire.Values{
			Fixed: []typ.Type{callbackType, typ.Number}, Tail: manifestwire.ValuesVariable, Var: 0, TailType: typ.Any,
		},
		Outcomes: []manifestwire.Outcome{
			{Kind: manifestwire.OutcomeNormal, Values: anyClosed()},
			{Kind: manifestwire.OutcomeThrow, Values: anyClosed()},
			{Kind: manifestwire.OutcomeCancel, Values: anyClosed()},
		},
		Callbacks: []manifestwire.Callback{{
			Function:  manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
			Admission: manifestwire.CallableAdmissionOrdinary,
			Arguments: manifestwire.Values{
				Fixed:    []typ.Type{typ.Any},
				Tail:     manifestwire.ValuesVariable,
				Var:      0,
				TailType: typ.Any,
				Suffix:   []typ.Type{typ.String},
			},
			Outcomes:  callbackTerminals(),
			Lifecycle: manifestwire.CallbackSyncRequiredOnce,
			Effects:   manifestwire.RowSpec{Tail: manifestwire.RowClosed},
		}},
		Subedges: []manifestwire.Subedge{subedge},
		Effects:  manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	}
	if mutate != nil {
		mutate(&operation)
	}
	declaration.DefineFunctionOperation("folds", operation)
	return declaration
}

// TestManifestDeclaresASuffixArgumentOrigin is the positive law: a manifest
// declares the suffix segment, the seal admits it, and the sealed Target
// answers the segment, its exact element type, and the authority that supplies
// it. The three coordinates together are total over the applied vector, which
// is what makes the origin set complete.
func TestManifestDeclaresASuffixArgumentOrigin(t *testing.T) {
	contract, err := sealRelationCatalogue(suffixApplicationHostManifest(nil))
	if err != nil {
		t.Fatalf("a subedge carrying a suffix argument origin was refused: %v", err)
	}
	operation, ok := contract.Operations.Lookup(relationBinding("folds"))
	if !ok {
		t.Fatal("sealed Target holds no folds operation")
	}
	edge, ok := contract.Operations.SubedgeAt(operation, 0)
	if !ok {
		t.Fatal("sealed Target holds no application for folds")
	}
	arguments, ok := contract.Operations.SubedgeArguments(edge)
	if !ok {
		t.Fatal("sealed application holds no argument vector")
	}
	if count := contract.Operations.ValuesCount(arguments); count != 1 {
		t.Fatalf("argument prefix width = %d, want the one declared fixed operand", count)
	}
	if count := contract.Operations.ValuesSuffixCount(arguments); count != 1 {
		t.Fatalf("argument suffix width = %d, want the one declared end-anchored operand", count)
	}
	element, ok := contract.Operations.ValuesSuffixAt(arguments, 0)
	if !ok {
		t.Fatal("sealed argument suffix holds no element type")
	}
	declared, ok := contract.Operations.ValuesAt(arguments, 0)
	if !ok || element == declared {
		t.Fatalf("suffix element type %d is not distinct from the prefix element %d", element, declared)
	}
	// Origins are stored in canonical (segment, index) order, so the suffix
	// operand is the second of the three the vector requires.
	if count := contract.Operations.SubedgeArgumentOriginCount(edge); count != 3 {
		t.Fatalf("argument origin count = %d, want one per declared segment", count)
	}
	segment, index, source, input, ok := contract.Operations.SubedgeArgumentOriginAt(edge, 1)
	if !ok || segment != vocabulary.ArgumentSuffix || index != 0 ||
		source != vocabulary.ArgumentSourceRule || input != (vocabulary.InputSource{}) {
		t.Fatalf("suffix origin = segment %d index %d source %d input %+v (ok %t), want the rule-supplied suffix operand 0",
			segment, index, source, input, ok)
	}
}

// The suffix is a coordinate of the applied vector, not of the declaring
// member's own parameters. A Lua parameter list is a prefix and an optional
// pack, so the operation input has no end-relative coordinate and the seal
// refuses one by name.
func TestManifestRefusesASuffixOnTheOperationInput(t *testing.T) {
	_, err := sealRelationCatalogue(suffixApplicationHostManifest(func(operation *manifestwire.Operation) {
		operation.Input.Suffix = []typ.Type{typ.String}
	}))
	if err == nil {
		t.Fatal("an operation input carrying a suffix sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "input Values cannot have a suffix") {
		t.Fatalf("refusal = %v, want the named input-suffix refusal", err)
	}
}

// RuleEntry is the nullary form of ArgumentSourceRule: it states that the
// owner rule is the entry authority for an application with no operands at
// all. An application that really has operands states one origin per segment
// instead, so mixing the two is refused rather than leaving segments unowned.
func TestManifestRefusesARuleEntryApplicationWithASuffix(t *testing.T) {
	_, err := sealRelationCatalogue(suffixApplicationHostManifest(func(operation *manifestwire.Operation) {
		operation.Subedges[0].RuleEntry = true
		operation.Subedges[0].ArgumentOrigins = nil
	}))
	if err == nil {
		t.Fatal("a RuleEntry application carrying a suffix sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "RuleEntry requires an empty argument product") {
		t.Fatalf("refusal = %v, want the named empty-argument-product refusal", err)
	}
}

// Every segment of an applied vector needs exactly one authority. Dropping the
// suffix origin leaves the trailing operand with no source, and the seal
// refuses the partial set rather than treating the absent segment as unused.
func TestManifestRefusesAnArgumentOriginSetMissingItsSuffix(t *testing.T) {
	_, err := sealRelationCatalogue(suffixApplicationHostManifest(func(operation *manifestwire.Operation) {
		origins := operation.Subedges[0].ArgumentOrigins
		operation.Subedges[0].ArgumentOrigins = origins[:len(origins)-1]
	}))
	if err == nil {
		t.Fatal("an application whose suffix operand has no origin sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "argument origins are incomplete") {
		t.Fatalf("refusal = %v, want the named incomplete-origins refusal", err)
	}
}

// A closed vector has no end-relative coordinate: the seal canonicalizes its
// authored suffix into the prefix so equivalent vectors share one handle, and
// the ArgumentSuffix coordinate then names no segment. The operand is real and
// is addressed as ArgumentFixed at its prefix index.
func TestManifestRefusesASuffixCoordinateOnAClosedArgumentVector(t *testing.T) {
	_, err := sealRelationCatalogue(suffixApplicationHostManifest(func(operation *manifestwire.Operation) {
		callback := &operation.Callbacks[0]
		callback.Arguments.Tail, callback.Arguments.TailType = manifestwire.ValuesClosed, nil
		origins := operation.Subedges[0].ArgumentOrigins
		operation.Subedges[0].ArgumentOrigins = []manifestwire.ArgumentOrigin{origins[0], origins[2]}
	}))
	if err == nil {
		t.Fatal("a suffix coordinate on a closed argument vector sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "argument origin does not name a Values segment") {
		t.Fatalf("refusal = %v, want the named unaddressed-segment refusal", err)
	}
}

// A suffix operand fed from a declared input is type-checked exactly as a
// prefix operand is: the owner coordinate it names must be assignable to the
// element type the applied vector declares. The trailing operand is a string
// and the input it would read is a number, so the seal refuses the relation
// rather than admitting an unchecked end-anchored operand.
func TestManifestRefusesATypeIncompatibleSuffixArgumentOrigin(t *testing.T) {
	_, err := sealRelationCatalogue(suffixApplicationHostManifest(func(operation *manifestwire.Operation) {
		origins := operation.Subedges[0].ArgumentOrigins
		origins[len(origins)-1] = manifestwire.ArgumentOrigin{
			Segment: manifestwire.ArgumentSuffix, Index: 0, Kind: manifestwire.ArgumentSourceInput,
			Source: manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 1},
		}
	}))
	if err == nil {
		t.Fatal("a suffix operand read from a type-incompatible input sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "type-incompatible Values") {
		t.Fatalf("refusal = %v, want the named type-incompatible refusal", err)
	}
}

// A RuleEntry subedge does seal once it carries the empty argument product its
// entry authority requires, so the refusals above are about the declared
// operands and not about rule-entry subedges as such.
func TestManifestAdmitsARuleEntrySubedgeWithNoArgumentProduct(t *testing.T) {
	declaration := lifecycleHostManifest(manifestwire.CallbackSyncRequiredOnce)
	if _, err := sealRelationCatalogue(declaration); err != nil {
		t.Fatalf("a rule-entry subedge with no argument product was refused: %v", err)
	}
}
