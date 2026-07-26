package signature

// operationalEffectColdLanes owns the migration-only carrier fields. The
// canonical analysis/check engine has no OperationalEffects reference.
//
// These fields remain wire-visible because __legacy/check/exportmanifest/
// function_signatures.go produces them and __legacy/engine/effectlowering/
// provider_operational.go reads them. LifecycleEffects and
// TypestateRequirements additionally have authored readers in
// __legacy/root_tests/fixture_harness_test.go. Keep new lanes in the live or
// roadmap registry in operational_effects.go, not here.
var operationalEffectColdLanes = []operationalEffectLane{
	operationalEffectSliceLane("ReturnPresenceRelations",
		func(e OperationalEffects) []ReturnPresenceRelation { return e.ReturnPresenceRelations },
		func(e *OperationalEffects, facts []ReturnPresenceRelation) { e.ReturnPresenceRelations = facts },
		cloneReturnPresenceRelations, equalReturnPresenceRelations, nil),
	operationalEffectSliceLane("NormalReturnPresenceRefinements",
		func(e OperationalEffects) []PathPresenceRefinement { return e.NormalReturnPresenceRefinements },
		func(e *OperationalEffects, facts []PathPresenceRefinement) { e.NormalReturnPresenceRefinements = facts },
		clonePathPresenceRefinements, equalPathPresenceRefinements, nil),
	operationalEffectSliceLane("NormalReturnTypeRefinements",
		func(e OperationalEffects) []PathTypeRefinement { return e.NormalReturnTypeRefinements },
		func(e *OperationalEffects, facts []PathTypeRefinement) { e.NormalReturnTypeRefinements = facts },
		clonePathTypeRefinements, equalPathTypeRefinements, substitutePathTypeRefinementTypes),
	operationalEffectSliceLane("PathStaticMembers",
		func(e OperationalEffects) []PathStaticMemberFact { return e.PathStaticMembers },
		func(e *OperationalEffects, facts []PathStaticMemberFact) { e.PathStaticMembers = facts },
		clonePathStaticMemberFacts, equalPathStaticMemberFacts, substitutePathStaticMemberTypes),
	operationalEffectSliceLane("PathStaticMemberDeltas",
		func(e OperationalEffects) []PathStaticMemberDelta { return e.PathStaticMemberDeltas },
		func(e *OperationalEffects, facts []PathStaticMemberDelta) { e.PathStaticMemberDeltas = facts },
		clonePathStaticMemberDeltas, equalPathStaticMemberDeltas, substitutePathStaticMemberDeltaTypes),
	operationalEffectSliceLane("PathInvalidations",
		func(e OperationalEffects) []PathInvalidation { return e.PathInvalidations },
		func(e *OperationalEffects, facts []PathInvalidation) { e.PathInvalidations = facts },
		clonePathInvalidations, equalPathInvalidations, nil),
	operationalEffectSliceLane("FrozenTables",
		func(e OperationalEffects) []FrozenTable { return e.FrozenTables },
		func(e *OperationalEffects, facts []FrozenTable) { e.FrozenTables = facts },
		cloneFrozenTables, equalFrozenTables, nil),
	operationalEffectSliceLane("EscapeEvents",
		func(e OperationalEffects) []EscapeEvent { return e.EscapeEvents },
		func(e *OperationalEffects, facts []EscapeEvent) { e.EscapeEvents = facts },
		cloneEscapeEvents, equalEscapeEvents, nil),
	operationalEffectSliceLane("StoreRelations",
		func(e OperationalEffects) []StoreRelation { return e.StoreRelations },
		func(e *OperationalEffects, facts []StoreRelation) { e.StoreRelations = facts },
		cloneStoreRelations, equalStoreRelations, nil),
	operationalEffectSliceLane("ParamRelations",
		func(e OperationalEffects) []ParamRelation { return e.ParamRelations },
		func(e *OperationalEffects, facts []ParamRelation) { e.ParamRelations = facts },
		cloneParamRelations, equalParamRelations, nil),
	operationalEffectSliceLane("ReturnFlows",
		func(e OperationalEffects) []ReturnFlow { return e.ReturnFlows },
		func(e *OperationalEffects, facts []ReturnFlow) { e.ReturnFlows = facts },
		cloneReturnFlows, equalReturnFlows, nil),
	operationalEffectSliceLane("LifecycleEffects",
		func(e OperationalEffects) []LifecycleEffect { return e.LifecycleEffects },
		func(e *OperationalEffects, facts []LifecycleEffect) { e.LifecycleEffects = facts },
		cloneLifecycleEffects, equalLifecycleEffects, nil),
	operationalEffectSliceLane("TypestateRequirements",
		func(e OperationalEffects) []TypestateRequirement { return e.TypestateRequirements },
		func(e *OperationalEffects, facts []TypestateRequirement) { e.TypestateRequirements = facts },
		cloneTypestateRequirements, equalTypestateRequirements, nil),
}
