package manifesttarget_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// A callback declaration states two facts about one coordinate: the input
// formal that carries the callable, and the Values relation the operation
// applies to it. This file is the law that the seal joins them. Without it a
// manifest may declare a formal typed () -> any and apply three operands to
// it, and nothing downstream ever compares the two.

// conformanceMember is the member name every declaration in this file authors.
const conformanceMember = "applies"

// conformanceHostManifest declares one callable whose sole parameter carries a
// callback, and one operation that applies the given Values relation to it.
// The callback is retained, so the declaration needs no direct subedge and the
// argument vector is the only relation under test.
func conformanceHostManifest(formal typ.Type, source manifestwire.InputSource, admission manifestwire.CallableAdmission, arguments manifestwire.Values, valuesVars uint32) *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	memberType := typ.Func().Param("callback", formal).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature(conformanceMember, signature.Function{Type: memberType})
	declaration.DefineFunctionOperation(conformanceMember, manifestwire.Operation{
		Replace:    true,
		ValuesVars: valuesVars,
		Input:      manifestwire.Values{Fixed: []typ.Type{formal}, Tail: manifestwire.ValuesClosed},
		Outcomes: []manifestwire.Outcome{
			{Kind: manifestwire.OutcomeNormal, Values: anyClosed()},
			{Kind: manifestwire.OutcomeThrow, Values: anyClosed()},
		},
		Callbacks: []manifestwire.Callback{{
			Function: source, Admission: admission, Arguments: arguments,
			Outcomes: callbackTerminals(), Lifecycle: manifestwire.CallbackRetainedOptionalOnce,
			Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
		}},
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})
	return declaration
}

func firstFormal() manifestwire.InputSource {
	return manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0}
}

func closedVector(fixed ...typ.Type) manifestwire.Values {
	return manifestwire.Values{Fixed: fixed, Tail: manifestwire.ValuesClosed}
}

func openVector(element typ.Type, fixed ...typ.Type) manifestwire.Values {
	return manifestwire.Values{Fixed: fixed, Tail: manifestwire.ValuesVariable, Var: 0, TailType: element}
}

// conformanceCase is one declared (formal type, applied vector) pair and the
// answer the seal owes it.
type conformanceCase struct {
	name      string
	formal    typ.Type
	source    manifestwire.InputSource
	admission manifestwire.CallableAdmission
	arguments manifestwire.Values
	vars      uint32
	refusal   string
}

func conformanceCases() []conformanceCase {
	nullary := typ.Func().Returns(typ.Any).Build()
	unary := typ.Func().Param("s", typ.String).Returns(typ.Any).Build()
	optional := typ.Func().Param("s", typ.String).OptParam("n", typ.Integer).Returns(typ.Any).Build()
	variadic := typ.Func().Param("s", typ.String).Variadic(typ.String).Returns(typ.Any).Build()
	openVariadic := typ.Func().Variadic(typ.String).Returns(typ.Any).Build()
	direct := manifestwire.CallableAdmissionDirectFunction

	return []conformanceCase{
		{
			name: "nullary formal takes the empty vector", formal: nullary, source: firstFormal(),
			admission: direct, arguments: closedVector(),
		},
		{
			name: "unary formal takes its one operand", formal: unary, source: firstFormal(),
			admission: direct, arguments: closedVector(typ.String),
		},
		{
			name: "optional parameter may be left unapplied", formal: optional, source: firstFormal(),
			admission: direct, arguments: closedVector(typ.String),
		},
		{
			name: "optional parameter may be applied", formal: optional, source: firstFormal(),
			admission: direct, arguments: closedVector(typ.String, typ.Integer),
		},
		{
			name: "variadic formal takes an open tail of the element type", formal: openVariadic, source: firstFormal(),
			admission: direct, arguments: openVector(typ.String), vars: 1,
		},
		{
			name: "variadic formal takes its required prefix and an open tail", formal: variadic, source: firstFormal(),
			admission: direct, arguments: openVector(typ.String, typ.String), vars: 1,
		},
		{
			name: "variadic formal takes an end-anchored suffix", formal: openVariadic, source: firstFormal(),
			admission: direct, vars: 1,
			arguments: manifestwire.Values{Tail: manifestwire.ValuesVariable, Var: 0, TailType: typ.String, Suffix: []typ.Type{typ.String}},
		},
		{
			name: "a gradual formal states no arity", formal: typ.Any, source: firstFormal(),
			admission: direct, arguments: closedVector(typ.String, typ.Integer, typ.Boolean),
		},
		{
			name: "arity overflow is refused", formal: nullary, source: firstFormal(),
			admission: direct, arguments: closedVector(typ.Any),
			refusal: "applies 1 operand(s) to a parameter list of at most 0",
		},
		{
			name: "arity underflow is refused", formal: unary, source: firstFormal(),
			admission: direct, arguments: closedVector(),
			refusal: "applies at least 0 operand(s) to a parameter list requiring 1",
		},
		{
			name: "an open tail against a non-variadic formal is refused", formal: unary, source: firstFormal(),
			admission: direct, arguments: openVector(typ.String, typ.String), vars: 1,
			refusal: "applies an open argument tail to a parameter list of at most 1 fixed parameter(s)",
		},
		{
			name: "a type-incompatible fixed operand is refused", formal: unary, source: firstFormal(),
			admission: direct, arguments: closedVector(typ.Boolean),
			refusal: "fixed operand 0 is declared boolean",
		},
		{
			name: "a type-incompatible tail element is refused", formal: openVariadic, source: firstFormal(),
			admission: direct, arguments: openVector(typ.Boolean), vars: 1,
			refusal: "tail element at the variadic parameter is declared boolean",
		},
		{
			name: "a type-incompatible suffix operand is refused", formal: openVariadic, source: firstFormal(),
			admission: direct, vars: 1,
			arguments: manifestwire.Values{Tail: manifestwire.ValuesVariable, Var: 0, TailType: typ.String, Suffix: []typ.Type{typ.Boolean}},
			refusal:   "suffix operand 0 at the variadic parameter is declared boolean",
		},
		{
			name: "a formal that admits no callable is refused", formal: typ.String, source: firstFormal(),
			admission: direct, arguments: closedVector(),
			refusal: "does not admit the callback's declared callable admission",
		},
		{
			name: "an input coordinate the declaration does not declare is refused", formal: nullary,
			source:    manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 3},
			admission: direct, arguments: closedVector(),
			refusal: "names input formal 3, which the declaration does not declare",
		},
	}
}

// TestCallbackArgumentVectorConformsToItsDeclaredFormal is the seal law for the
// callback argument relation.
func TestCallbackArgumentVectorConformsToItsDeclaredFormal(t *testing.T) {
	for _, item := range conformanceCases() {
		t.Run(item.name, func(t *testing.T) {
			_, err := sealRelationCatalogue(conformanceHostManifest(item.formal, item.source, item.admission, item.arguments, item.vars))
			if item.refusal == "" {
				if err != nil {
					t.Fatalf("conforming declaration was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("declaration sealed, want the refusal %q", item.refusal)
			}
			if !strings.Contains(err.Error(), item.refusal) {
				t.Fatalf("refusal = %v, want it to name %q", err, item.refusal)
			}
		})
	}
}

// A callback declared on an amending law reaches no sealed operation, so the
// declaration is refused rather than dropped.
func TestCallbackOnAnAmendingLawIsRefused(t *testing.T) {
	declaration := manifestwire.New(relationHostModule)
	callbackType := typ.Func().Returns(typ.Any).Build()
	declaration.DefineFunctionSignature(conformanceMember, signature.Function{
		Type: typ.Func().Param("callback", callbackType).Returns(typ.Any).Build(),
	})
	declaration.DefineFunctionOperation(conformanceMember, manifestwire.Operation{
		Callbacks: []manifestwire.Callback{{
			Function: firstFormal(), Admission: manifestwire.CallableAdmissionDirectFunction,
			Arguments: closedVector(), Outcomes: callbackTerminals(),
			Lifecycle: manifestwire.CallbackRetainedOptionalOnce,
			Effects:   manifestwire.RowSpec{Tail: manifestwire.RowClosed},
		}},
	})
	_, err := sealRelationCatalogue(declaration)
	if err == nil {
		t.Fatal("a callback declared on an amending law sealed, want a refusal")
	}
	if !strings.Contains(err.Error(), "amending operation law") {
		t.Fatalf("refusal = %v, want it to name the amending law", err)
	}
}
