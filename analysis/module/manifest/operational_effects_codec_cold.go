package manifest

import (
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// operationalEffectsColdWireLanes preserves migration-only manifest lanes.
//
// The canonical analysis/check engine neither produces nor consumes these
// fields. They cannot be deleted while the __legacy migration tree remains a
// reader and producer: check/exportmanifest/function_signatures.go publishes
// the return, refinement, path, escape, store, parameter, and return-flow
// lanes; engine/effectlowering/provider_operational.go consumes them; and
// root_tests/fixture_harness_test.go still supplies lifecycle and typestate
// contracts. Keep new work out of this table.
var operationalEffectsColdWireLanes = []operationalEffectsWireLane{
	wireLane("ReturnPresenceRelations",
		func(e *signature.OperationalEffects) *[]signature.ReturnPresenceRelation {
			return &e.ReturnPresenceRelations
		},
		func(w *operationalEffectsWire) *[]returnPresenceRelationWire { return &w.ReturnPresenceRelations },
		encodeReturnPresenceRelation, decodeReturnPresenceRelation, compareReturnPresenceRelationWire, nil),
	wireLane("NormalReturnPresenceRefinements",
		func(e *signature.OperationalEffects) *[]signature.PathPresenceRefinement {
			return &e.NormalReturnPresenceRefinements
		},
		func(w *operationalEffectsWire) *[]pathPresenceRefinementWire {
			return &w.NormalReturnPresenceRefinements
		},
		encodeNormalReturnPresenceRefinement, decodeNormalReturnPresenceRefinement, comparePathPresenceRefinementWire, nil),
	contextWireLane("NormalReturnTypeRefinements",
		func(e *signature.OperationalEffects) *[]signature.PathTypeRefinement {
			return &e.NormalReturnTypeRefinements
		},
		func(w *operationalEffectsWire) *[]pathTypeRefinementWire { return &w.NormalReturnTypeRefinements },
		encodeNormalReturnTypeRefinementContext, decodeNormalReturnTypeRefinement, comparePathTypeRefinementWire, nil),
	contextWireLane("PathStaticMembers",
		func(e *signature.OperationalEffects) *[]signature.PathStaticMemberFact { return &e.PathStaticMembers },
		func(w *operationalEffectsWire) *[]pathStaticMemberWire { return &w.PathStaticMembers },
		encodePathStaticMemberContext, decodePathStaticMember, comparePathStaticMemberWire, nil),
	contextWireLane("PathStaticMemberDeltas",
		func(e *signature.OperationalEffects) *[]signature.PathStaticMemberDelta {
			return &e.PathStaticMemberDeltas
		},
		func(w *operationalEffectsWire) *[]pathStaticMemberDeltaWire {
			return &w.PathStaticMemberDeltas
		},
		encodePathStaticMemberDeltaContext, decodePathStaticMemberDelta, comparePathStaticMemberDeltaWire, nil),
	wireLane("PathInvalidations",
		func(e *signature.OperationalEffects) *[]signature.PathInvalidation { return &e.PathInvalidations },
		func(w *operationalEffectsWire) *[]pathInvalidationWire { return &w.PathInvalidations },
		encodePathInvalidation, decodePathInvalidation, comparePathInvalidationWire, nil),
	wireLane("FrozenTables",
		func(e *signature.OperationalEffects) *[]signature.FrozenTable { return &e.FrozenTables },
		func(w *operationalEffectsWire) *[]frozenTableWire { return &w.FrozenTables },
		encodeFrozenTable, decodeFrozenTable, compareFrozenTableWire, nil),
	wireLane("EscapeEvents",
		func(e *signature.OperationalEffects) *[]signature.EscapeEvent { return &e.EscapeEvents },
		func(w *operationalEffectsWire) *[]escapeEventWire { return &w.EscapeEvents },
		encodeEscapeEvent, decodeEscapeEvent, compareEscapeEventWire, nil),
	wireLane("StoreRelations",
		func(e *signature.OperationalEffects) *[]signature.StoreRelation { return &e.StoreRelations },
		func(w *operationalEffectsWire) *[]storeRelationWire { return &w.StoreRelations },
		encodeStoreRelation, decodeStoreRelation, compareStoreRelationWire, nil),
	wireLane("ParamRelations",
		func(e *signature.OperationalEffects) *[]signature.ParamRelation { return &e.ParamRelations },
		func(w *operationalEffectsWire) *[]paramRelationWire { return &w.ParamRelations },
		encodeParamRelation, decodeParamRelation, compareParamRelationWire, nil),
	wireLane("LifecycleEffects",
		func(e *signature.OperationalEffects) *[]signature.LifecycleEffect { return &e.LifecycleEffects },
		func(w *operationalEffectsWire) *[]lifecycleEffectWire { return &w.LifecycleEffects },
		encodeLifecycleEffect, decodeLifecycleEffect, compareLifecycleEffectWire, nil),
	wireLane("TypestateRequirements",
		func(e *signature.OperationalEffects) *[]signature.TypestateRequirement {
			return &e.TypestateRequirements
		},
		func(w *operationalEffectsWire) *[]typestateRequirementWire { return &w.TypestateRequirements },
		encodeTypestateRequirement, decodeTypestateRequirement, compareTypestateRequirementWire, nil),
}
