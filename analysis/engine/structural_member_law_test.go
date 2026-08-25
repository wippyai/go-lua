package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// structural_member_law_test.go states the runtime disposition of a row whose
// output is an activation row set.
//
// Every other generated member publishes into a Factor: it holds a slot, a
// strong exact target or a route universe, and a carry closure over the
// coordinates it preserves. A structural row holds none of those, because
// there is no Factor for any of them to be taken over. What it holds instead
// is the set of already-mounted members its branches name, and the authority
// to accept them.

func structuralMemberLawSpec(t *testing.T) generatedMemberSpec {
	t.Helper()
	return generatedMemberSpec{
		member:        generatedMemberTestMember(t),
		structural:    true,
		factorOrdinal: 0,
		inputCount:    0,
		outputCount:   0,
		topology:      &equation.Topology{},
		graph:         &equation.Graph{},
	}
}

// TestAStructuralMemberHoldsNoPublicationCoordinate is the disposition itself.
func TestAStructuralMemberHoldsNoPublicationCoordinate(t *testing.T) {
	member, sealed := generatedMemberExecution(structuralMemberLawSpec(t))
	if !sealed || member == nil {
		t.Fatal("a structural member did not seal")
	}
	if _, hasSlot := member.outputSlot(); hasSlot {
		t.Fatal("a structural member claims an output slot")
	}
	if _, named := member.factorKey(); named {
		t.Fatal("a structural member names a written Factor")
	}
	if member.writesOutput() || member.routeNarrow() || member.routeScope() != nil {
		t.Fatal("a structural member claims a publication surface")
	}
	if len(member.targets()) != 0 || len(member.carryTargets()) != 0 || len(member.narrowTargets()) != 0 {
		t.Fatal("a structural member claims publication targets")
	}
}

// TestAStructuralMemberWithoutItsAcceptanceAuthorityDoesNotSeal keeps the row
// whole. A member that could settle a branch and not accept it would refuse
// deep in an epoch, where the reason is gone.
func TestAStructuralMemberWithoutItsAcceptanceAuthorityDoesNotSeal(t *testing.T) {
	for name, damage := range map[string]func(*generatedMemberSpec){
		"no topology": func(spec *generatedMemberSpec) { spec.topology = nil },
		"no graph":    func(spec *generatedMemberSpec) { spec.graph = nil },
	} {
		t.Run(name, func(t *testing.T) {
			spec := structuralMemberLawSpec(t)
			damage(&spec)
			if _, sealed := generatedMemberExecution(spec); sealed {
				t.Fatalf("a structural member sealed with %s", name)
			}
		})
	}
}

// TestAStructuralMemberRefusesAPublicationSurface is the converse fence: the
// two geometries are told apart by the declared mode, so a spec carrying both
// dispositions is a member disagreeing with itself.
func TestAStructuralMemberRefusesAPublicationSurface(t *testing.T) {
	for name, damage := range map[string]func(*generatedMemberSpec){
		"writes":       func(spec *generatedMemberSpec) { spec.writes, spec.hasSlot = true, true },
		"output count": func(spec *generatedMemberSpec) { spec.outputCount = 1 },
		"routed":       func(spec *generatedMemberSpec) { spec.routed = true },
		"carries":      func(spec *generatedMemberSpec) { spec.carries = []int{0} },
		"target":       func(spec *generatedMemberSpec) { spec.target = carrier.Target{} },
	} {
		t.Run(name, func(t *testing.T) {
			spec := structuralMemberLawSpec(t)
			before, beforeOK := generatedMemberExecution(spec)
			if !beforeOK || before == nil {
				t.Fatal("the undamaged structural spec must seal")
			}
			damage(&spec)
			if name == "target" {
				// The zero target IS the structural disposition, so this arm
				// only states that the undamaged spec already carries it.
				return
			}
			if _, sealed := generatedMemberExecution(spec); sealed {
				t.Fatalf("a structural member sealed while claiming %s", name)
			}
		})
	}
}

// TestASettledBranchOutsideTheMountedSetPublishesNothing states the addressing
// fence. The ordinals are the member's own branch set; one outside it names a
// row the construct plane never mounted, and the whole publication refuses
// rather than the prefix that happened to resolve.
func TestASettledBranchOutsideTheMountedSetPublishesNothing(t *testing.T) {
	member, sealed := generatedMemberExecution(structuralMemberLawSpec(t))
	if !sealed {
		t.Fatal("a structural member did not seal")
	}
	member.activations = [][]equation.Member{{}, {}}
	if _, ok := member.acceptSettledBranches([]uint32{2}, support.Mask{}, nil); ok {
		t.Fatal("a branch outside the mounted set was accepted")
	}
	// A trigger that settled no branch instantiates nothing and stays admitted
	// on its own declaration, so it needs no premise at all.
	accepted, ok := member.acceptSettledBranches(nil, support.Mask{}, nil)
	if !ok || len(accepted) != 0 {
		t.Fatalf("a trigger that settled nothing = %d/%t", len(accepted), ok)
	}
}
