package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestProveStaticMarksCallTypeSubtreeOnly(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyNil, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyTypeValue, 1),
		c(keyspace.FamilyCall, 1),
		c(keyspace.FamilyTypePrimitive, 2),
		c(keyspace.FamilyTypeOptional, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilValue := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	typeValue := keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	primitiveChild := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	primitivePeer := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)
	optional := keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{call}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{typeValue},
			},
			TypeValues: []authored.TypeValue{{Owner: body}},
			Calls:      []authored.Call{{Owner: body, Callee: nilValue, Actuals: values}},
		},
		static: static.Input{
			Types: static.TypesInput{
				Primitive: []static.Primitive{{Kind: static.PrimitiveAny}, {Kind: static.PrimitiveString}},
				Optional:  []static.Optional{{Inner: primitiveChild}},
			},
			Contracts: static.ContractsInput{Call: []static.CallContract{{TypeArguments: []keyspace.Term{optional}}}},
			Operands:  static.OperandsInput{TypeValue: []static.TypeValueTarget{{Target: primitivePeer}}},
		},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	for _, term := range []keyspace.Term{optional, primitiveChild} {
		if !result.Static(term) {
			t.Fatalf("Call-owned type term %v is not static", term)
		}
	}
	if result.Static(primitivePeer) {
		t.Fatalf("non-Call static peer %v was marked", primitivePeer)
	}
	if result.Static(call) {
		t.Fatalf("runtime Call %v was marked from its type argument", call)
	}
}

// TestProveStaticMarksStorageIdentitiesAndReferenceExclusions checks the
// legacy storage asymmetry at the complete Result boundary. Static closure
// includes storage identities owned by marked Bind/Function/Loop constructs,
// but it does not follow Write, capture-outer, or TableField reference
// relations as expression edges.
