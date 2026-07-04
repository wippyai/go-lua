// Package callpayload defines generic call-boundary payload DTOs.
package callpayload

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	engineregistry "github.com/wippyai/go-lua/analysis/engine/registry"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CallResult is one indexed abstract result produced by a call.
type CallResult struct {
	Index int
	Value product.Value
}

// CallOutcomeProvider resolves rich call-site evidence into one generic call
// outcome payload.
type CallOutcomeProvider func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) CallOutcome

// CallOutcome is the generic payload produced at a call boundary. It carries
// return-slot values plus normal-return facts expressed over placeholder paths
// such as $0 and $1. Fact application rebases those paths at the caller.
type CallOutcome struct {
	Results []CallResult

	// PostReturnAuthority means this outcome matched a semantic provider that
	// owns result slots and normal-return state facts for the call. Supplemental
	// providers may still add diagnostic ParamObligations, but must not publish
	// weaker return or post-return facts through this call.
	PostReturnAuthority bool

	NormalReturnFacts          callboundary.NormalReturnFacts
	HeapTableObjects           map[identity.ID]heapidentity.TableObject
	Placements                 map[identity.ID]placement.Value
	ParamObligations           []CallParamObligation
	PathObligations            []CallPathObligation
	ParamPathRefinements       []CallParamPathRefinement
	ParamPathWrites            []CallParamPathWrite
	ParamLengthFloors          []CallParamLengthFloor
	ParamPathInvalidations     []CallParamPathInvalidation
	ParamConditions            []CallParamCondition
	ParamPathRelations         []CallParamPathRelation
	ReturnConditionRefinements []CallReturnConditionRefinement
	ReturnConditionSlots       []CallReturnConditionSlotRefinement
	ReturnPresenceRelations    []CallReturnPresenceRelation
	ParamExposures             []CallParamExposure
}

// Empty reports whether the outcome carries no result, authority, diagnostic,
// exposure, or post-return evidence.
func (o CallOutcome) Empty() bool {
	for _, lane := range callOutcomeLanes {
		if lane.has(o) {
			return false
		}
	}
	return true
}

// HasPostReturnEvidence reports whether the outcome carries caller-visible
// facts that hold after a normal return. Result slots are intentionally
// separate because weak top/any/unknown results do not establish authority.
func (o CallOutcome) HasPostReturnEvidence() bool {
	for _, lane := range callOutcomeLanes {
		if lane.postReturn && lane.has(o) {
			return true
		}
	}
	return false
}

type callOutcomeLane struct {
	fieldName  string
	postReturn bool
	has        func(CallOutcome) bool
}

// CallOutcomeFieldRole describes one registered CallOutcome field's boundary
// role. It intentionally omits the lane's predicate: callers may classify
// fields, but only callpayload owns field-presence semantics.
type CallOutcomeFieldRole struct {
	FieldName  string
	PostReturn bool
}

// CallOutcomeFieldRoleBinding pairs a layer-owned handler with the canonical
// call-outcome field role it extends.
type CallOutcomeFieldRoleBinding[T any] struct {
	Role  CallOutcomeFieldRole
	Value T
}

// CallOutcomeFieldRoles returns the registered CallOutcome field roles in
// canonical struct order. The returned slice is a copy.
func CallOutcomeFieldRoles() []CallOutcomeFieldRole {
	out := make([]CallOutcomeFieldRole, len(callOutcomeLanes))
	for i, lane := range callOutcomeLanes {
		out[i] = CallOutcomeFieldRole{
			FieldName:  lane.fieldName,
			PostReturn: lane.postReturn,
		}
	}
	return out
}

// CallOutcomeSupplementalFactRoles returns fields merged through supplemental
// fact lanes. Result slots and post-return authority are handled by separate
// call-outcome merge laws.
func CallOutcomeSupplementalFactRoles() []CallOutcomeFieldRole {
	roles := CallOutcomeFieldRoles()
	out := roles[:0]
	for _, role := range roles {
		switch role.FieldName {
		case "Results", "PostReturnAuthority":
			continue
		default:
			out = append(out, role)
		}
	}
	return out
}

// BindCallOutcomeSupplementalFactRoles orders layer-owned handlers by the
// canonical supplemental fact roles and rejects missing, invalid, or orphan
// handlers.
func BindCallOutcomeSupplementalFactRoles[T any](
	owner string,
	handlers map[string]T,
	valid func(T) bool,
) []CallOutcomeFieldRoleBinding[T] {
	if owner == "" {
		owner = "call-outcome supplemental fact"
	}
	bindings := engineregistry.BindOrdered(engineregistry.BindOptions[string, CallOutcomeFieldRole, T]{
		Owner:    owner + " lane",
		Roles:    callOutcomeSupplementalFactRoleEntries(),
		Handlers: handlers,
		Valid:    valid,
		KeyName:  func(fieldName string) string { return fieldName },
	})
	out := make([]CallOutcomeFieldRoleBinding[T], 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, CallOutcomeFieldRoleBinding[T]{
			Role:  binding.Role,
			Value: binding.Handler,
		})
	}
	return out
}

func callOutcomeSupplementalFactRoleEntries() []engineregistry.Role[string, CallOutcomeFieldRole] {
	roles := CallOutcomeSupplementalFactRoles()
	out := make([]engineregistry.Role[string, CallOutcomeFieldRole], len(roles))
	for i, role := range roles {
		out[i] = engineregistry.Role[string, CallOutcomeFieldRole]{
			Key:   role.FieldName,
			Value: role,
		}
	}
	return out
}

var callOutcomeLanes = []callOutcomeLane{
	{
		fieldName: "Results",
		has:       func(o CallOutcome) bool { return len(o.Results) != 0 },
	},
	{
		fieldName: "PostReturnAuthority",
		has:       func(o CallOutcome) bool { return o.PostReturnAuthority },
	},
	{
		fieldName:  "NormalReturnFacts",
		postReturn: true,
		has:        func(o CallOutcome) bool { return !o.NormalReturnFacts.Empty() },
	},
	{
		fieldName:  "HeapTableObjects",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.HeapTableObjects) != 0 },
	},
	{
		fieldName:  "Placements",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.Placements) != 0 },
	},
	{
		fieldName: "ParamObligations",
		has:       func(o CallOutcome) bool { return len(o.ParamObligations) != 0 },
	},
	{
		fieldName: "PathObligations",
		has:       func(o CallOutcome) bool { return len(o.PathObligations) != 0 },
	},
	{
		fieldName:  "ParamPathRefinements",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ParamPathRefinements) != 0 },
	},
	{
		fieldName:  "ParamPathWrites",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ParamPathWrites) != 0 },
	},
	{
		fieldName:  "ParamLengthFloors",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ParamLengthFloors) != 0 },
	},
	{
		fieldName:  "ParamPathInvalidations",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ParamPathInvalidations) != 0 },
	},
	{
		fieldName:  "ParamConditions",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ParamConditions) != 0 },
	},
	{
		fieldName:  "ParamPathRelations",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ParamPathRelations) != 0 },
	},
	{
		fieldName:  "ReturnConditionRefinements",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ReturnConditionRefinements) != 0 },
	},
	{
		fieldName:  "ReturnConditionSlots",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ReturnConditionSlots) != 0 },
	},
	{
		fieldName:  "ReturnPresenceRelations",
		postReturn: true,
		has:        func(o CallOutcome) bool { return len(o.ReturnPresenceRelations) != 0 },
	},
	{
		fieldName: "ParamExposures",
		has:       func(o CallOutcome) bool { return len(o.ParamExposures) != 0 },
	},
}

// CallParamExposure records that the callee exposes one explicit argument (or a
// member sub-path of one) through a wider mutable view the caller cannot track
// writes back through: the parameter is aliased, at a wider mutable contract
// type, into a slot the callee returns, stores into another parameter's member,
// or retains in a captured sink. A write through that wider view can launder a
// wide value back into the argument object, so the caller must eager-widen the
// argument's source object toward Contract at the call to keep a later narrow
// read of the argument sound. Source is the callee-relative placeholder path of
// the exposed argument ($i, or a member sub-path $i.child); the caller rebases it
// onto the concrete argument path. Contract is a witness-bearing value whose type
// is the destination slot type (a returned member, a destination parameter's slot
// type, or a sink's slot type), not the parameter's own declared type, which
// would under-widen a covariant narrow-source/wider-destination store. Kind
// selects the widening routine (record field rebuild vs opaque array element
// witness). This is the single unified call-boundary exposure lane for every
// interprocedural covariant-exposure route.
type CallParamExposure struct {
	Source   pathdom.Path
	Contract product.Value
	Kind     factflow.CovariantExposureKind
}

// CallParamObligation records a pre-call value constraint for one explicit
// argument. It is diagnostic evidence only and is not applied as a normal-return
// refinement to caller state.
type CallParamObligation struct {
	ParamIndex int
	Value      product.Value
	Origin     CallParamObligationOrigin
}

// CallPathObligation records a pre-call value constraint for a caller-visible
// local/captured path. It is diagnostic evidence only: callers may project it
// back to their own parameters when the path is a stable local derived from a
// parameter, but fact application never writes it as a post-call refinement.
type CallPathObligation struct {
	Path  pathdom.Path
	Value product.Value
}

// CallParamObligationOrigin records why a diagnostic-only parameter
// obligation exists. Plain function-signature obligations leave HasOrigin
// false; member-call obligations use this to render the provider/member path
// that imposed the requirement.
type CallParamObligationOrigin struct {
	HasOrigin        bool
	ReceiverParam    int
	ReceiverPath     pathaddr.SuffixKey
	Member           segment.Segment
	ArgParam         int
	MemberParamIndex int
	SubjectLabel     string
	ProviderLabel    string
}

// CallParamPathRefinement records a normal-return value constraint for a
// parameter placeholder path. Parameter placeholders are indexed by explicit
// argument position and do not include the receiver slot.
type CallParamPathRefinement struct {
	Path  pathdom.Path
	Value product.Value
}

// CallParamPathWrite records a normal-return path update for a mutable
// parameter. Unlike a refinement, this replaces the caller's current path value:
// it models an effect such as table.insert changing a target table's container
// shape rather than a guard proving the old value was narrower.
type CallParamPathWrite struct {
	Path  pathdom.Path
	Value product.Value
}

// CallParamLengthFloor records a normal-return lower bound on len(param path).
type CallParamLengthFloor struct {
	Path  pathdom.Path
	Floor int64
}

// CallParamPathInvalidation records that descendants below a parameter
// placeholder path were invalidated by a normal-returning call.
type CallParamPathInvalidation struct {
	Path                      pathdom.Path
	PreserveStructuralWitness bool
}

// CallParamCondition records that a normal return selects the truthiness facts
// for one call argument expression.
type CallParamCondition struct {
	ParamIndex int
	Value      bool
}

// CallPathRelationKind classifies a normal-return relation over placeholder
// paths.
type CallPathRelationKind uint8

const (
	CallPathRelationEqual CallPathRelationKind = iota + 1
)

// CallParamPathRelation records a normal-return relation over parameter
// placeholder paths. Parameter placeholders are indexed by explicit argument
// position and do not include the receiver slot.
type CallParamPathRelation struct {
	Kind  CallPathRelationKind
	Left  pathdom.Path
	Right pathdom.Path
}

// CallReturnConditionRefinement records a parameter-relative value refinement
// selected by the boolean value of one call return slot.
type CallReturnConditionRefinement struct {
	ReturnIndex int
	ReturnValue bool
	Target      pathdom.Path
	Value       product.Value
}

// CallReturnConditionSlotRefinement records a sibling return-slot value
// refinement selected by the Lua truthiness of another call return slot.
type CallReturnConditionSlotRefinement struct {
	ReturnIndex int
	ReturnValue bool
	TargetIndex int
	Value       product.Value
}

// CallReturnPresenceRelation records a must implication between two call
// return slots.
type CallReturnPresenceRelation struct {
	TriggerIndex    int
	TriggerPresence presence.Value
	TargetIndex     int
	TargetPresence  presence.Value
}
