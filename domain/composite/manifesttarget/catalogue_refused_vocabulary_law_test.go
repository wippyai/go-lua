package manifesttarget_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// This file states the refusal half of the declared-vocabulary law. A wire
// constant the engine cannot honour must be refused by name when a manifest
// declares it. A silent no-op would let a provider state a consequence the
// checker never applies, which is exactly the unsound gap the law forbids.

func refusalHostManifest(member string, operation manifestwire.Operation) *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	memberType := typ.Func().Param("value", typ.Any).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature(member, signature.Function{Type: memberType})
	declaration.DefineFunctionOperation(member, operation)
	return declaration
}

func closedValues(fixed ...typ.Type) manifestwire.Values {
	return manifestwire.Values{Fixed: fixed, Tail: manifestwire.ValuesClosed}
}

// Lua's break and goto cannot cross a function boundary, and an authored
// Outcome describes exactly such a boundary crossing. The two kinds are
// therefore inapplicable to any provider declaration, and the seal must say so
// rather than accept an outcome arm no call site can ever take.
func TestManifestRefusesNonActivationOutcomeKinds(t *testing.T) {
	for name, kind := range map[string]manifestwire.OutcomeKind{
		"break": manifestwire.OutcomeBreak,
		"goto":  manifestwire.OutcomeGoto,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := sealRelationCatalogue(refusalHostManifest("escapes", manifestwire.Operation{
				Replace: true,
				Input:   closedValues(typ.Any),
				Outcomes: []manifestwire.Outcome{
					{Kind: manifestwire.OutcomeNormal, Values: closedValues(typ.Any)},
					{Kind: kind, Values: closedValues()},
				},
			}))
			if err == nil {
				t.Fatalf("a manifest declaring %s sealed, want a named refusal", name)
			}
			if !strings.Contains(err.Error(), "invalid outcome kind") {
				t.Fatalf("refusal = %v, want the named invalid-outcome-kind refusal", err)
			}
		})
	}
}

// The same operation shape seals cleanly with the activation outcome kinds a
// provider may really declare, so the refusal above is about the two kinds and
// not about the declaration shape carrying them.
func TestManifestAdmitsActivationOutcomeKinds(t *testing.T) {
	for name, kind := range map[string]manifestwire.OutcomeKind{
		"throw":  manifestwire.OutcomeThrow,
		"yield":  manifestwire.OutcomeYield,
		"cancel": manifestwire.OutcomeCancel,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := sealRelationCatalogue(refusalHostManifest("escapes", manifestwire.Operation{
				Replace: true,
				Input:   closedValues(typ.Any),
				Outcomes: []manifestwire.Outcome{
					{Kind: manifestwire.OutcomeNormal, Values: closedValues(typ.Any)},
					{Kind: kind, Values: closedValues()},
				},
				Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
			})); err != nil {
				t.Fatalf("a manifest declaring %s was refused: %v", name, err)
			}
		})
	}
}

// ValuesUnknown is the tail of the engine's own synthesized opaque operation:
// it states that the value vector is not knowable at all. A provider that
// declares a callable knows its own arity, so the tail is not authorable and
// the seal refuses it by name.
func TestManifestRefusesTheUnknownValuesTail(t *testing.T) {
	_, err := sealRelationCatalogue(refusalHostManifest("opaque", manifestwire.Operation{
		Replace:  true,
		Input:    manifestwire.Values{Tail: manifestwire.ValuesUnknown},
		Outcomes: []manifestwire.Outcome{{Kind: manifestwire.OutcomeNormal, Values: closedValues(typ.Any)}},
	}))
	if err == nil {
		t.Fatal("a manifest declaring the unknown Values tail sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "invalid Values tail") {
		t.Fatalf("refusal = %v, want the named invalid-Values-tail refusal", err)
	}
}

// The two tails a provider may really declare seal on the same operation
// shape, so the refusal above is about ValuesUnknown and not about the shape.
func TestManifestAdmitsTheAuthorableValuesTails(t *testing.T) {
	if _, err := sealRelationCatalogue(refusalHostManifest("closed", manifestwire.Operation{
		Replace:  true,
		Input:    closedValues(typ.Any),
		Outcomes: []manifestwire.Outcome{{Kind: manifestwire.OutcomeNormal, Values: closedValues(typ.Any)}},
		Effects:  manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})); err != nil {
		t.Fatalf("a manifest declaring the closed Values tail was refused: %v", err)
	}
}

// RowUnknownOpen is the effect row of the synthesized opaque operation. An
// authored invocation row is a finite, provider-owned statement of what the
// callable invokes, so the open-unknown tail is not authorable there.
func TestManifestRefusesTheUnknownOpenEffectRow(t *testing.T) {
	_, err := sealRelationCatalogue(refusalHostManifest("open_row", manifestwire.Operation{
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowUnknownOpen},
	}))
	if err == nil {
		t.Fatal("a manifest declaring the unknown-open effect row sealed, want a named refusal")
	}
	if !strings.Contains(err.Error(), "row has invalid tail") {
		t.Fatalf("refusal = %v, want the named invalid-row-tail refusal", err)
	}
}

// RowVariable names a row formal of the declaring operation. The wire
// Operation carries a ValuesVars arity but no row-formal arity, so a manifest
// can name the tail and can never declare a row formal for it to address:
// every authored RowVariable row is out of scope by construction. The refusal
// is named rather than silent, but the constant remains unauthorable, which is
// a wire-vocabulary gap rather than a provider mistake.
func TestManifestCannotAddressARowVariable(t *testing.T) {
	for _, variable := range []manifestwire.RowVar{0, 1} {
		_, err := sealRelationCatalogue(refusalHostManifest("row_var", manifestwire.Operation{
			Effects: manifestwire.RowSpec{Tail: manifestwire.RowVariable, Var: variable},
		}))
		if err == nil {
			t.Fatalf("a manifest declaring row variable %d sealed; the wire carries no row-formal arity to admit it", variable)
		}
		if !strings.Contains(err.Error(), "row variable outside operation scope") {
			t.Fatalf("refusal for row variable %d = %v, want the named out-of-scope refusal", variable, err)
		}
	}
}

// The closed row a provider can really declare seals on the same shape.
func TestManifestAdmitsTheClosedEffectRow(t *testing.T) {
	if _, err := sealRelationCatalogue(refusalHostManifest("closed_row", manifestwire.Operation{
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})); err != nil {
		t.Fatalf("a manifest declaring the closed effect row was refused: %v", err)
	}
}
