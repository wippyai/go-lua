package rules

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	staticowner "github.com/wippyai/go-lua/analysis/domain/static/owner"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestDeclarePublishesTheSoleRuntimeTypeofRule(t *testing.T) {
	schema, authority, source := typeOfFixture(t)
	composition := engine.NewComposition()
	values, ok := valueowner.Declare(composition, typeOfKey(1), typeOfKey(900_001), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	statics, ok := staticowner.Declare(composition, typeOfKey(2), authority)
	if !ok {
		t.Fatal("Static owner")
	}
	rule, ok := Declare(composition, typeOfKey(3), typeOfKey(4), typeOfKey(5), statics, values)
	if !ok || rule == nil {
		t.Fatal("typeof Rule")
	}
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: typeOfKey(6),
		Project:  func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: typeOfKey(7),
			Freeze:   func(value bool) bool { return value },
			Clone:    func(value bool) bool { return value },
			Equal:    func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		_, ok := engine.QueryReadFrom(query, statics.ExactRead())
		return ok
	})
	if !ok || query == nil {
		t.Fatal("typeof query")
	}
	if !composition.Seal() {
		t.Fatal("typeof composition seal")
	}
	inventory, ok := composition.RuleAdmissionInventory()
	if !ok || len(inventory.Rules) != 1 || inventory.Rules[0] != (engine.RuleAdmissionRecord{Rule: typeOfKey(3), Basis: engine.RuleAdmissionBasisDerivation, Identity: typeOfKey(5)}) {
		t.Fatal("typeof Rule admission inventory")
	}
	var runtime, pure int
	digests := make(map[[32]byte]struct{})
	for index := 0; index < source.Static().Inputs().Count(); index++ {
		operand, ok := source.Static().Inputs().At(index)
		if !ok {
			t.Fatalf("StaticInputAt(%d)", index)
		}
		_, contained, ok := authority.TypeOf(operand)
		if !ok {
			t.Fatalf("TypeOf(%d)", index)
		}
		if _, stateful := contained.RuntimeSubject(); stateful {
			runtime++
			frozen, digest, frozenOK := staticInputContent(authority, values, operand)
			_, replayDigest, replayOK := staticInputContent(authority, values, frozen)
			if !frozenOK || !replayOK || digest == [32]byte{} || digest != replayDigest {
				t.Fatalf("runtime typeof %d OperandContent is not pure and idempotent", index)
			}
			if _, duplicate := digests[digest]; duplicate {
				t.Fatalf("runtime typeof %d collided with a distinct StaticInput", index)
			}
			digests[digest] = struct{}{}
			if instance, ok := rule.Instance(operand); !ok || instance == nil {
				t.Fatalf("runtime typeof %d did not produce its atomic RuleInstance", index)
			}
		} else {
			pure++
			if instance, ok := rule.Instance(operand); ok || instance != nil {
				t.Fatalf("pure typeof %d entered the stateful Rule", index)
			}
		}
	}
	if runtime != 2 || pure != 1 || len(digests) != runtime {
		t.Fatalf("typeof denominator = runtime:%d pure:%d", runtime, pure)
	}
}

func TestDeclareRejectsForeignOwnersAndInvalidSemantics(t *testing.T) {
	schema, authority, _ := typeOfFixture(t)
	composition := engine.NewComposition()
	values, ok := valueowner.Declare(composition, typeOfKey(10), typeOfKey(900_010), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	statics, ok := staticowner.Declare(composition, typeOfKey(11), authority)
	if !ok {
		t.Fatal("Static owner")
	}
	if rule, ok := Declare(composition, typeOfKey(12), typeOfKey(12), typeOfKey(14), statics, values); ok || rule != nil {
		t.Fatal("colliding semantic keys declared typeof Rule")
	}
	foreignComposition := engine.NewComposition()
	foreignValues, ok := valueowner.Declare(foreignComposition, typeOfKey(15), typeOfKey(900_015), schema)
	if !ok {
		t.Fatal("foreign Value owner")
	}
	if rule, ok := Declare(composition, typeOfKey(16), typeOfKey(17), typeOfKey(18), statics, foreignValues); ok || rule != nil {
		t.Fatal("foreign owner declared typeof Rule")
	}
	otherSchema, otherAuthority, _ := typeOfFixture(t)
	if authority.LinkID() != otherAuthority.LinkID() || authority.Link() == otherAuthority.Link() {
		t.Fatal("same-content fixture did not retain distinct Link owners")
	}
	sameContentComposition := engine.NewComposition()
	leftStatics, ok := staticowner.Declare(sameContentComposition, typeOfKey(19), authority)
	if !ok {
		t.Fatal("same-content left Static owner")
	}
	rightValues, ok := valueowner.Declare(sameContentComposition, typeOfKey(20), typeOfKey(900_020), otherSchema)
	if !ok {
		t.Fatal("same-content right Value owner")
	}
	if rule, ok := Declare(sameContentComposition, typeOfKey(21), typeOfKey(22), typeOfKey(23), leftStatics, rightValues); ok || rule != nil {
		t.Fatal("same-content foreign Link owners declared typeof Rule")
	}
}

// The domain can neither create a live derivation nor substitute a derivation
// from another Rule.  A zero-shaped forged capability must fail closed rather
// than minting a Static result.
func TestTypeofCheckerRejectsForgedDerivation(t *testing.T) {
	schema, authority, _ := typeOfFixture(t)
	composition := engine.NewComposition()
	values, ok := valueowner.Declare(composition, typeOfKey(30), typeOfKey(900_030), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	statics, ok := staticowner.Declare(composition, typeOfKey(31), authority)
	if !ok {
		t.Fatal("Static owner")
	}
	var read engine.Read[engine.OrderedCells[value.Value]]
	check := checker(statics, values, typeOfKey(32), &read)
	if evidence, accepted := check(engine.RuleDerivation[staticdomain.Value, linkstatic.InputRef]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged derivation minted Static typeof evidence")
	}
}

func typeOfFixture(t testing.TB) (*value.Schema, *staticdomain.Authority, *link.Link) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "typeof_rule.lua", Text: []byte("local subject = 1\ntype Runtime = typeof(subject)\ntype Again = typeof(subject)\ntype Known = typeof('literal')\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "typeof_rule", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, ok := heap.Seal(source)
	if !ok {
		t.Fatal("Heap schema")
	}
	schema, ok := value.Seal(source, heaps)
	if !ok {
		t.Fatal("Value schema")
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("type authority")
	}
	authority, _, err := staticdomain.Seal(source, types)
	if err != nil {
		t.Fatal(err)
	}
	return schema, authority, source
}

func typeOfKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("typeof test key")
	}
	return key
}
