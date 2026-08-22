// activation_endpoint_context_law_test.go states the assignment law of one
// activation transition to the endpoint Points of the transport edges that
// cross it. One candidate declares one transition and a transport vector that
// crosses it in both directions, so the endpoint modules - not a fixed
// orientation - decide which Context each Point resolves its state in.

package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// activationEndpointLawID mints one distinct law-local identity. The law reads
// only equality of these values, so a dense counter is the whole requirement.
func activationEndpointLawID(ordinal byte) identity.ContentID {
	var id identity.ContentID
	id[0], id[1] = 0xae, ordinal
	return id
}

const (
	activationEndpointLawTriggerContext byte = iota + 1
	activationEndpointLawBodyContext
	activationEndpointLawTransition
	activationEndpointLawTriggerModule
	activationEndpointLawBodyModule
	activationEndpointLawForeignModule
	activationEndpointLawSingleModule
)

func activationEndpointLawContext(t *testing.T) (equation.ActivationContext, identity.ContentID, identity.ContentID) {
	t.Helper()
	trigger := activationEndpointLawID(activationEndpointLawTriggerContext)
	body := activationEndpointLawID(activationEndpointLawBodyContext)
	context := equation.ActivationContext{
		TransitionID:  activationEndpointLawID(activationEndpointLawTransition),
		FromContextID: trigger,
		ToContextID:   body,
	}
	if !context.Available() || trigger == body {
		t.Fatalf("law fixture is not a two-context transition")
	}
	return context, trigger, body
}

// TestActivationEndpointContextsFollowTheEndpointModules states that both
// directions of one candidate's transport vector resolve. The imports run from
// the trigger's module to the body's module and the export runs back, and each
// endpoint resolves its state in the Context of its own module.
func TestActivationEndpointContextsFollowTheEndpointModules(t *testing.T) {
	context, trigger, body := activationEndpointLawContext(t)
	triggerModule := activationEndpointLawID(activationEndpointLawTriggerModule)
	bodyModule := activationEndpointLawID(activationEndpointLawBodyModule)

	source, target, oriented := selectedActivationEndpointContextIDs(context, triggerModule, bodyModule, triggerModule, bodyModule)
	if !oriented || source != trigger || target != body {
		t.Fatalf("import edge resolves %v->%v (oriented=%t), want %v->%v", source, target, oriented, trigger, body)
	}

	source, target, oriented = selectedActivationEndpointContextIDs(context, triggerModule, bodyModule, bodyModule, triggerModule)
	if !oriented || source != body || target != trigger {
		t.Fatalf("export edge resolves %v->%v (oriented=%t), want %v->%v", source, target, oriented, body, trigger)
	}
}

// TestActivationEndpointContextsRefuseForeignEndpoints states that the
// assignment is total over the transition's own two modules and refuses any
// edge whose endpoints name neither end of it.
func TestActivationEndpointContextsRefuseForeignEndpoints(t *testing.T) {
	context, _, _ := activationEndpointLawContext(t)
	triggerModule := activationEndpointLawID(activationEndpointLawTriggerModule)
	bodyModule := activationEndpointLawID(activationEndpointLawBodyModule)
	foreign := activationEndpointLawID(activationEndpointLawForeignModule)

	for _, endpoints := range [][2]identity.ContentID{
		{foreign, bodyModule},
		{triggerModule, foreign},
		{foreign, foreign},
	} {
		if source, target, oriented := selectedActivationEndpointContextIDs(context, triggerModule, bodyModule, endpoints[0], endpoints[1]); oriented {
			t.Fatalf("edge %v->%v is admitted as %v->%v; an endpoint outside the transition names no Context", endpoints[0], endpoints[1], source, target)
		}
	}
}

// TestActivationEndpointContextsCollapseWithinOneModule states that a
// transition whose two Contexts belong to one module makes both assignments
// identical, so a same-module activation is orientation-free.
func TestActivationEndpointContextsCollapseWithinOneModule(t *testing.T) {
	context, trigger, body := activationEndpointLawContext(t)
	module := activationEndpointLawID(activationEndpointLawSingleModule)

	source, target, oriented := selectedActivationEndpointContextIDs(context, module, module, module, module)
	if !oriented || source != trigger || target != body {
		t.Fatalf("same-module edge resolves %v->%v (oriented=%t), want %v->%v", source, target, oriented, trigger, body)
	}
}
