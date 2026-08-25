package engine

import "testing"

type exactDestinationProjectorLaw struct{ local uint64 }

func (projector exactDestinationProjectorLaw) ProjectExactDestination(candidate uint32) (uint64, bool) {
	return projector.local + uint64(candidate), true
}

// TestConsumerExactDestinationUsesOnlyItsClaimedFamilyProjector states the
// construction fence.  A cross-axis exact destination is projected by the
// family claimed for that exact rule and output Factor; a missing or foreign
// claim cannot fall through to a candidate-owner ordinal.
func TestConsumerExactDestinationUsesOnlyItsClaimedFamilyProjector(t *testing.T) {
	const rule, factor = uint64(3), uint64(5)
	state := &schemaBindingState{
		phase: schemaBindingSealed, authority: &schemaBindingAuthority{},
		ruleFamilies: map[uint64]ruleFamilyClaim{
			rule: {factor: factor, installer: exactDestinationProjectorLaw{local: 11}},
		},
	}
	projector, ok := generatedExactDestinationProjector(state, rule, factor)
	if !ok {
		t.Fatal("the exact rule's claimed projector was not resolved")
	}
	if local, projected := projector.ProjectExactDestination(7); !projected || local != 18 {
		t.Fatalf("projected local = %d/%t, want 18", local, projected)
	}
	if _, ok := generatedExactDestinationProjector(state, rule+1, factor); ok {
		t.Fatal("a missing rule claim resolved a destination projector")
	}
	if _, ok := generatedExactDestinationProjector(state, rule, factor+1); ok {
		t.Fatal("a projector claimed by another Factor crossed the output fence")
	}
	state.ruleFamilies[rule] = ruleFamilyClaim{factor: factor, installer: struct{}{}}
	if _, ok := generatedExactDestinationProjector(state, rule, factor); ok {
		t.Fatal("a family with no typed destination projector was accepted")
	}
}
