package rule

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/ownership"
	ownershipowner "github.com/wippyai/go-lua/analysis/domain/ownership/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestAllocationSourceSeedsOnlyExactAllocationOwnerAndLifetimeDuties(t *testing.T) {
	source, schema := allocationFixture(t, "allocation_source")
	_, owner, declaration := allocationComposition(t, schema, 0)

	accepted := 0
	for index := 0; index < schema.CoordinateCount(); index++ {
		coordinate, ok := schema.CoordinateAt(index)
		if !ok {
			t.Fatalf("coordinate %d", index)
		}
		operand, acceptedOperand := NewAllocationOperand(source, schema, coordinate)
		origin, _ := schema.Origin(coordinate)
		role, _ := schema.Role(coordinate)
		if origin.Kind() != ownership.OriginAllocationRoot || (role != ownership.Owner && role != ownership.Lifetime) {
			if acceptedOperand {
				t.Fatalf("non-allocation ownership coordinate %v/%v entered allocation source", origin.Kind(), role)
			}
			continue
		}
		if !acceptedOperand || !operand.valid() {
			t.Fatalf("allocation %v operand", role)
		}
		accepted++
		if instance, ok := declaration.Instance(operand); !ok || instance == nil {
			t.Fatalf("allocation %v exact instance", role)
		}
		value, ok := allocationResult(owner, operand)
		if !ok || !schema.Admit(coordinate, value) || value.Count() != 1 {
			t.Fatalf("allocation %v source result", role)
		}
		root, age, minimum, maximum, member := value.At(0)
		if !member || root != operand.root || age != materialization.Recent || minimum != ownership.One || maximum != ownership.One {
			t.Fatalf("allocation %v result = root:%v age:%v range:%v..%v", role, root, age, minimum, maximum)
		}
		_, digest, contentOK := allocationOperandContent(operand)
		if !contentOK || digest == [32]byte{} {
			t.Fatalf("allocation %v lacks stable cold operand identity", role)
		}
	}
	if accepted == 0 {
		t.Fatal("fixture did not yield an allocation ownership source")
	}
}

func TestAllocationSourceFencesForeignAndNonAllocationCoordinates(t *testing.T) {
	left, leftSchema := allocationFixture(t, "allocation_fence")
	right, rightSchema := allocationFixture(t, "allocation_fence")
	if left.ContentID() != right.ContentID() {
		t.Fatal("fixture Links should be replay-equivalent")
	}
	var local ownership.Coordinate
	for index := 0; index < leftSchema.CoordinateCount(); index++ {
		candidate, ok := leftSchema.CoordinateAt(index)
		if !ok {
			t.Fatal("left coordinate")
		}
		origin, _ := leftSchema.Origin(candidate)
		role, _ := leftSchema.Role(candidate)
		if origin.Kind() == ownership.OriginAllocationRoot && role == ownership.Owner {
			local = candidate
			break
		}
	}
	if !local.Valid() {
		t.Fatal("local allocation owner coordinate")
	}
	if _, ok := NewAllocationOperand(left, rightSchema, local); ok {
		t.Fatal("foreign/replayed schema accepted local coordinate")
	}
	if _, ok := NewAllocationOperand(right, leftSchema, local); ok {
		t.Fatal("foreign Link accepted local coordinate")
	}
	if _, ok := NewAllocationOperand(left, leftSchema, ownership.Coordinate{}); ok {
		t.Fatal("forged coordinate entered allocation source")
	}

	_, _, declaration := allocationComposition(t, leftSchema, 20)
	if instance, ok := declaration.Instance(AllocationOperand{}); ok || instance != nil {
		t.Fatal("empty allocation operand produced an instance")
	}
	foreignCoordinate, ok := rightSchema.CoordinateAt(0)
	if !ok {
		t.Fatal("foreign coordinate")
	}
	foreignOperand, ok := NewAllocationOperand(right, rightSchema, foreignCoordinate)
	if ok {
		if instance, accepted := declaration.Instance(foreignOperand); accepted || instance != nil {
			t.Fatal("foreign allocation operand produced an instance")
		}
	}
}

// RuleDerivation is opaque beyond engine. An empty derivation is the only
// constructible forged proof at this boundary and must never mint evidence.
func TestAllocationSourceCheckerRejectsForgedDerivation(t *testing.T) {
	_, schema := allocationFixture(t, "allocation_evidence")
	_, _, declaration := allocationComposition(t, schema, 40)
	if evidence, accepted := declaration.check(engine.RuleDerivation[ownership.Value, AllocationOperand]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged allocation source derivation minted evidence")
	}
}

func allocationFixture(t testing.TB, name string) (*link.Link, ownership.Schema) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte(`local a = {}; local b = function() return a end; return b`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heapSchema, heapOK := heap.Seal(source)
	schema, ok := ownership.NewSchema(source, heapSchema)
	if !heapOK || !ok {
		t.Fatal("Ownership schema")
	}
	return source, schema
}

func allocationComposition(t testing.TB, schema ownership.Schema, offset uint64) (*engine.Composition, *ownershipowner.Owner, *AllocationRule) {
	t.Helper()
	composition := engine.NewComposition()
	owner, ownerOK := ownershipowner.Declare(composition, allocationKey(offset+1), schema)
	declaration, ruleOK := DeclareAllocation(composition, allocationKey(offset+2), allocationKey(offset+3), allocationKey(offset+4), owner)
	if !ownerOK || !ruleOK || declaration == nil || !allocationQuery(composition, owner, offset+5) || !composition.Seal() {
		t.Fatal("allocation ownership composition")
	}
	return composition, owner, declaration
}

func allocationQuery(composition *engine.Composition, owner *ownershipowner.Owner, offset uint64) bool {
	if composition == nil || owner == nil {
		return false
	}
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: allocationKey(offset), Project: func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: allocationKey(offset + 1), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		_, read := engine.QueryReadFrom(query, owner.Read())
		return read
	})
	return ok && query != nil
}

func allocationKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("allocation ownership test key")
	}
	return key
}
