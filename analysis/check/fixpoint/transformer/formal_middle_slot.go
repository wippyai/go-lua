package transformer

import statekey "github.com/wippyai/go-lua/analysis/domain/state/key"

// formalMiddleSlotForStateKey is the sole binding from an evolving CFG-point
// Values cell to its formal tuple coordinate. Even when slot names a parameter,
// capture, global, or ambient, a point-local read observes the Middle register
// seeded from that immutable input root, never the input root itself.
func formalMiddleSlotForStateKey(program *RelationProgram, body *relationProgramBody, slot statekey.Value) (FormalSlot, bool) {
	if program == nil || program.formalSlots == nil || body == nil || body.relation.arena == nil || slot == 0 {
		return FormalSlot{}, false
	}
	root, ok := body.relation.arena.middleRoot(slot)
	if !ok || root.Kind != RootMiddle {
		return FormalSlot{}, false
	}
	return program.formalSlots.Slot(body.body, root)
}

// formalLiveValueSlotForDependency binds both alternatives of the registered
// ValueDependency sum to the Values coordinate observed by one CFG-point
// transaction. Concrete TransferAccess slots use the arena's sealed
// slot-to-Middle relation; coordinate dependencies already carry their exact
// formal root. Structural Input roots remain the canonical identity for paths
// and coordinates, but their evolving scalar value lives in the Middle
// register seeded from that input.
func formalLiveValueSlotForDependency(program *RelationProgram, body *relationProgramBody, dependency statekey.ValueDependency) (FormalSlot, bool) {
	if program == nil || program.formalFibers == nil || body == nil || body.variable == 0 || !dependency.Valid() {
		return FormalSlot{}, false
	}
	span, exact := program.formalFibers.span(body.variable)
	if !exact || span.forest.program != program || span.liveValues == nil {
		return FormalSlot{}, false
	}
	slot, present := span.liveValues[dependency]
	return slot, present && slot.Valid() && slot.Body() == body.body
}
