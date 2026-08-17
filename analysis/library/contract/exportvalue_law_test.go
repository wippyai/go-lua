package contract

import (
	"encoding/hex"
	"math"
	"testing"
)

// The export-value payload states what a non-callable export is and the
// mutability it is published with. Its whole content is a discriminated value
// and a policy, so these laws hold every spelling still, hold them apart from
// every other payload this package writes, and refuse a value that says two
// things at once.

// exportValueCorpus is every shape the format can carry: both mutabilities over
// an aggregate, and one value of every kind in the closed constant domain.
func exportValueCorpus() []ExportValue {
	return []ExportValue{
		Aggregate(MutabilityMutable),
		Aggregate(MutabilitySealed),
		{Shape: ValueShapeConstant, Mutability: MutabilitySealed, Constant: Constant{Kind: ConstantNil}},
		{Shape: ValueShapeConstant, Mutability: MutabilityMutable, Constant: Constant{Kind: ConstantBoolean, Boolean: true}},
		{Shape: ValueShapeConstant, Mutability: MutabilitySealed, Constant: Constant{Kind: ConstantBoolean}},
		{Shape: ValueShapeConstant, Mutability: MutabilitySealed, Constant: Constant{Kind: ConstantInteger, Integer: math.MaxInt64}},
		{Shape: ValueShapeConstant, Mutability: MutabilitySealed, Constant: Constant{Kind: ConstantInteger, Integer: math.MinInt64}},
		{Shape: ValueShapeConstant, Mutability: MutabilitySealed, Constant: Constant{Kind: ConstantFloat, FloatBits: math.Float64bits(math.Pi)}},
		{Shape: ValueShapeConstant, Mutability: MutabilitySealed, Constant: Constant{Kind: ConstantFloat, FloatBits: math.Float64bits(math.Inf(1))}},
		{Shape: ValueShapeConstant, Mutability: MutabilityMutable, Constant: Constant{Kind: ConstantString, String: "Lua 5.4"}},
		{Shape: ValueShapeConstant, Mutability: MutabilitySealed, Constant: Constant{Kind: ConstantString}},
	}
}

// TestExportValueRoundTripsThroughItsOwnBytes states the format from both ends:
// what it writes it reads back as the same value, for every shape it carries.
func TestExportValueRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, value := range exportValueCorpus() {
		data, err := EncodeExportValue(value)
		if err != nil {
			t.Fatalf("%+v: the export value did not encode: %v", value, err)
		}
		got, err := DecodeExportValue(data)
		if err != nil {
			t.Fatalf("%+v: the export value did not decode: %v", value, err)
		}
		if got != value {
			t.Fatalf("the payload reads back as %+v, want %+v", got, value)
		}
	}
}

// TestExportValueDistinguishesEveryShapeItCarries states that no two authored
// values share a payload. A format whose spellings collided would publish two
// contracts under one identity.
func TestExportValueDistinguishesEveryShapeItCarries(t *testing.T) {
	written := make(map[string]ExportValue)
	for _, value := range exportValueCorpus() {
		data, err := EncodeExportValue(value)
		if err != nil {
			t.Fatalf("%+v: the export value did not encode: %v", value, err)
		}
		key := string(data)
		if prior, collided := written[key]; collided {
			t.Fatalf("%+v and %+v write one payload", prior, value)
		}
		written[key] = value
	}
}

// TestExportValueRejectsWhatItCannotCarry is the boundary law. A value that
// selects one field and carries another says two things at once, and a reader
// would have no ground to choose between them; an undeclared shape, an
// undeclared mutability and another payload's stream are refused for the same
// reason.
func TestExportValueRejectsWhatItCannotCarry(t *testing.T) {
	rejected := []ExportValue{
		{},
		{Shape: ValueShapeAggregate},
		{Shape: ValueShapeConstant, Mutability: MutabilityMutable},
		{Mutability: MutabilityMutable},
		// An aggregate is not a constant, so it carries none.
		{Shape: ValueShapeAggregate, Mutability: MutabilityMutable, Constant: Constant{Kind: ConstantNil}},
		// One selected field, one unselected field carrying a value.
		{Shape: ValueShapeConstant, Mutability: MutabilityMutable, Constant: Constant{Kind: ConstantInteger, Integer: 1, String: "1"}},
		{Shape: ValueShapeConstant, Mutability: MutabilityMutable, Constant: Constant{Kind: ConstantNil, Boolean: true}},
		{Shape: ValueShapeConstant, Mutability: MutabilityMutable, Constant: Constant{Kind: ConstantFloat, Integer: 1}},
		{Shape: valueShapeLimit, Mutability: MutabilityMutable},
		{Shape: ValueShapeAggregate, Mutability: mutabilityLimit},
		{Shape: ValueShapeConstant, Mutability: MutabilityMutable, Constant: Constant{Kind: constantKindLimit}},
	}
	for _, value := range rejected {
		if value.Available() {
			t.Fatalf("%+v is admitted as an export value", value)
		}
		if _, err := EncodeExportValue(value); err == nil {
			t.Fatalf("%+v encoded as a payload", value)
		}
	}
	if _, err := DecodeExportValue(nil); err == nil {
		t.Fatal("an empty payload decoded as an export value")
	}
	path, err := EncodePath(Export("pi"))
	if err != nil {
		t.Fatalf("the export path did not encode: %v", err)
	}
	if _, err := DecodeExportValue(path); err == nil {
		t.Fatal("an export path payload decoded as an export value")
	}
	value, err := EncodeExportValue(Aggregate(MutabilityMutable))
	if err != nil {
		t.Fatalf("the export value did not encode: %v", err)
	}
	if _, err := DecodePath(value); err == nil {
		t.Fatal("an export value payload decoded as an export path")
	}
	if _, err := DecodeRuleDelegation(value); err == nil {
		t.Fatal("an export value payload decoded as a rule delegation")
	}
	if _, err := DecodeExportValue(value[:len(value)-1]); err == nil {
		t.Fatal("a truncated export value decoded as a published value")
	}
	if _, err := DecodeExportValue(append(append([]byte(nil), value...), 0)); err == nil {
		t.Fatal("an export value with trailing bytes decoded as a published value")
	}
}

// TestExportValuePayloadWireIsPinned holds the payload bytes still. An export
// value published in a shipped contract is read by another build, so its
// encoding is a compatibility surface and not an implementation detail.
func TestExportValuePayloadWireIsPinned(t *testing.T) {
	const pinnedAggregate = "0126616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d76616c7565020101050101050101"
	const pinnedConstant = "0126616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d76616c75650201010501020501020301040501050804332e3134"
	aggregate, err := EncodeExportValue(Aggregate(MutabilityMutable))
	if err != nil {
		t.Fatalf("the aggregate export value did not encode: %v", err)
	}
	if got := hex.EncodeToString(aggregate); got != pinnedAggregate {
		t.Errorf("aggregate payload wire is %s, pinned %s", got, pinnedAggregate)
	}
	constant, err := EncodeExportValue(ExportValue{
		Shape:      ValueShapeConstant,
		Mutability: MutabilitySealed,
		Constant:   Constant{Kind: ConstantString, String: "3.14"},
	})
	if err != nil {
		t.Fatalf("the constant export value did not encode: %v", err)
	}
	if got := hex.EncodeToString(constant); got != pinnedConstant {
		t.Errorf("constant payload wire is %s, pinned %s", got, pinnedConstant)
	}
}
