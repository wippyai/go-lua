package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestStaticCrossOwnerCardinalityLaws(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyNil] = 1
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyTypePrimitive] = 4
	counts[keyspace.FamilyTypeValue] = 1
	counts[keyspace.FamilyValueClaim] = 3
	counts[keyspace.FamilyFunction] = 1
	counts[keyspace.FamilyCall] = 1
	flowCounts := counts
	flowCounts[keyspace.FamilyFunction] = 0
	flowCounts[keyspace.FamilyCall] = 0

	flowDraft, err := authored.Build(authored.Input{
		Counts: flowCounts,
		Claims: []authored.ValueClaim{
			{Owner: crossOwnerTerm(keyspace.FamilyBody, 1), Operand: crossOwnerTerm(keyspace.FamilyNil, 1), Kind: kind.ValueClaimTypeAs},
			{Owner: crossOwnerTerm(keyspace.FamilyBody, 1), Operand: crossOwnerTerm(keyspace.FamilyNil, 1), Kind: kind.ValueClaimTypeColonColon},
			{Owner: crossOwnerTerm(keyspace.FamilyBody, 1), Operand: crossOwnerTerm(keyspace.FamilyNil, 1), Kind: kind.ValueClaimNonNil},
		},
		TypeValues: []authored.TypeValue{{Owner: crossOwnerTerm(keyspace.FamilyBody, 1)}},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer func() { _ = flowFinalizer.Abort() }()
	flowView := flowFinalizer.View()

	staticInput := static.Input{
		Counts: counts,
		Types: static.TypesInput{Primitive: []static.Primitive{
			{Kind: static.PrimitiveNumber}, {Kind: static.PrimitiveString},
			{Kind: static.PrimitiveBoolean}, {Kind: static.PrimitiveNil},
		}},
		Contracts: static.ContractsInput{
			Function: []static.FunctionContract{{}},
			Call:     []static.CallContract{{}},
		},
		Operands: static.OperandsInput{
			Claim: []static.ClaimTarget{
				{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 1), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 1)},
				{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 2), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 2)},
			},
			TypeValue: []static.TypeValueTarget{{Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 3)}},
		},
	}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer func() { _ = staticFinalizer.Abort() }()

	if err := validateStaticCrossOwnerCardinalities(staticFinalizer.View(), flowView, counts); err != nil {
		t.Fatalf("valid cross-owner sidecars rejected: %v", err)
	}
	for _, test := range []struct {
		name   string
		family keyspace.Family
	}{
		{name: "Function contract denominator", family: keyspace.FamilyFunction},
		{name: "Call contract denominator", family: keyspace.FamilyCall},
		{name: "TypeValue target denominator", family: keyspace.FamilyTypeValue},
	} {
		t.Run(test.name, func(t *testing.T) {
			mismatch := counts
			mismatch[test.family]++
			if err := validateStaticCrossOwnerCardinalities(staticFinalizer.View(), flowView, mismatch); err == nil {
				t.Fatal("cross-owner denominator mismatch accepted")
			}
		})
	}

	for _, test := range []struct {
		name   string
		claims []static.ClaimTarget
	}{
		{
			name: "TypeAs requires target",
			claims: []static.ClaimTarget{
				{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 2), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 2)},
			},
		},
		{
			name: "ColonColon requires target",
			claims: []static.ClaimTarget{
				{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 1), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 1)},
			},
		},
		{
			name: "NonNil forbids target",
			claims: []static.ClaimTarget{
				{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 1), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 1)},
				{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 2), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 2)},
				{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 3), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 4)},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := staticInput
			input.Operands.Claim = test.claims
			draft, err := static.Build(input)
			if err != nil {
				t.Fatalf("static.Build: %v", err)
			}
			finalizer, err := draft.Finalizer()
			if err != nil {
				t.Fatalf("static.Finalizer: %v", err)
			}
			defer func() { _ = finalizer.Abort() }()
			if err := validateStaticCrossOwnerCardinalities(finalizer.View(), flowView, counts); err == nil {
				t.Fatal("invalid sparse ClaimTarget relation accepted")
			}
		})
	}

	for _, test := range []struct {
		name  string
		count uint32
	}{
		{name: "Flow claims undercount", count: counts[keyspace.FamilyValueClaim] + 1},
		{name: "Flow claims overcount", count: counts[keyspace.FamilyValueClaim] - 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			mismatch := counts
			mismatch[keyspace.FamilyValueClaim] = test.count
			if err := validateStaticCrossOwnerCardinalities(staticFinalizer.View(), flowView, mismatch); err == nil {
				t.Fatal("Flow claim denominator mismatch accepted")
			}
		})
	}

	extraFlowCounts := flowCounts
	extraFlowCounts[keyspace.FamilyValueClaim]++
	extraFlowInput := authored.Input{
		Counts: extraFlowCounts,
		Claims: []authored.ValueClaim{
			{Owner: crossOwnerTerm(keyspace.FamilyBody, 1), Operand: crossOwnerTerm(keyspace.FamilyNil, 1), Kind: kind.ValueClaimTypeAs},
			{Owner: crossOwnerTerm(keyspace.FamilyBody, 1), Operand: crossOwnerTerm(keyspace.FamilyNil, 1), Kind: kind.ValueClaimTypeColonColon},
			{Owner: crossOwnerTerm(keyspace.FamilyBody, 1), Operand: crossOwnerTerm(keyspace.FamilyNil, 1), Kind: kind.ValueClaimNonNil},
			{Owner: crossOwnerTerm(keyspace.FamilyBody, 1), Operand: crossOwnerTerm(keyspace.FamilyNil, 1), Kind: kind.ValueClaimNonNil},
		},
		TypeValues: []authored.TypeValue{{Owner: crossOwnerTerm(keyspace.FamilyBody, 1)}},
	}
	extraFlowDraft, err := authored.Build(extraFlowInput)
	if err != nil {
		t.Fatalf("extra authored.Build: %v", err)
	}
	extraFlowFinalizer, err := extraFlowDraft.Finalizer()
	if err != nil {
		t.Fatalf("extra authored.Finalizer: %v", err)
	}
	if err := validateStaticCrossOwnerCardinalities(staticFinalizer.View(), extraFlowFinalizer.View(), counts); err == nil {
		t.Fatal("extra Flow ValueClaim accepted")
	}
	if err := extraFlowFinalizer.Abort(); err != nil {
		t.Fatalf("extra authored.Abort: %v", err)
	}

	outOfRangeInput := staticInput
	outOfRangeInput.Operands.Claim = []static.ClaimTarget{
		{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 1), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 4)},
		{Claim: crossOwnerTerm(keyspace.FamilyValueClaim, 2), Target: crossOwnerTerm(keyspace.FamilyTypePrimitive, 2)},
	}
	outOfRangeDraft, err := static.Build(outOfRangeInput)
	if err != nil {
		t.Fatalf("out-of-range static.Build: %v", err)
	}
	outOfRangeFinalizer, err := outOfRangeDraft.Finalizer()
	if err != nil {
		t.Fatalf("out-of-range static.Finalizer: %v", err)
	}
	outOfRangeCounts := counts
	outOfRangeCounts[keyspace.FamilyTypePrimitive]--
	if err := validateStaticCrossOwnerCardinalities(outOfRangeFinalizer.View(), flowView, outOfRangeCounts); err == nil {
		t.Fatal("out-of-range Static target accepted")
	}
	if err := outOfRangeFinalizer.Abort(); err != nil {
		t.Fatalf("out-of-range static.Abort: %v", err)
	}

	capturedFlowView := flowView
	if err := flowFinalizer.Abort(); err != nil {
		t.Fatalf("authored.Abort: %v", err)
	}
	if err := validateStaticCrossOwnerCardinalities(staticFinalizer.View(), capturedFlowView, counts); err == nil {
		t.Fatal("expired Flow view accepted")
	}
	capturedStaticView := staticFinalizer.View()
	if err := staticFinalizer.Abort(); err != nil {
		t.Fatalf("static.Abort: %v", err)
	}
	if err := validateStaticCrossOwnerCardinalities(capturedStaticView, authored.View{}, counts); err == nil {
		t.Fatal("expired Static view accepted")
	}
}

func crossOwnerTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}
