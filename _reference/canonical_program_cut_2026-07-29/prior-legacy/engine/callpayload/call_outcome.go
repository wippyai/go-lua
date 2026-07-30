// Package callpayload defines generic call-boundary payload DTOs.
package callpayload

import (
	"reflect"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	engineregistry "github.com/wippyai/go-lua/analysis/engine/registry"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
)

// CallResult is one indexed abstract result produced by a call.
type CallResult struct {
	Index int
	Value product.Value
}

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

	// SuspensionKnown means this outcome came from a callee surface that
	// certified whether the call can suspend. Missing certification is
	// conservative and must be treated by consumers as may-suspend.
	SuspensionKnown bool
	MaySuspend      bool

	NormalReturnFacts          callboundary.NormalReturnFacts
	ProtectedCallTypestate     callboundary.ProtectedCallTypestate
	HeapTableObjects           map[identity.ID]heapidentity.TableObject
	Placements                 map[identity.ID]placement.Value
	ParamObligations           []CallParamObligation
	PathObligations            []CallPathObligation
	TypestateRequirements      []CallTypestateRequirement
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

// CallOutcomeRepresentationEqual reports exact pre-normalization syntax
// equality. It exists for the transient composition carrier: directional
// provider merge laws must see the exact child results before the completed
// composition is normalized into the semantic CallOutcome lattice.
func CallOutcomeRepresentationEqual(left, right CallOutcome) bool {
	// The common provider payload contains only the direct call-outcome lanes.
	// Compare that representation explicitly: reflect.DeepEqual allocates its
	// pointer-visit map even when every product value is already interned.
	//
	// NormalReturnFacts and non-empty heap objects remain on the exact
	// reflection fallback for now. Those boundary carriers own map-backed,
	// nested representations in other packages; duplicating their private
	// representation rules here would make this comparison silently diverge.
	if !callOutcomeRepresentationNormalReturnFactsEmpty(left.NormalReturnFacts) || !callOutcomeRepresentationNormalReturnFactsEmpty(right.NormalReturnFacts) ||
		!left.ProtectedCallTypestate.Empty() || !right.ProtectedCallTypestate.Empty() ||
		len(left.HeapTableObjects) != 0 || len(right.HeapTableObjects) != 0 {
		return callOutcomeRepresentationEqualSlow(left, right)
	}
	if left.PostReturnAuthority != right.PostReturnAuthority ||
		left.SuspensionKnown != right.SuspensionKnown ||
		left.MaySuspend != right.MaySuspend ||
		(left.HeapTableObjects == nil) != (right.HeapTableObjects == nil) ||
		(left.Placements == nil) != (right.Placements == nil) ||
		len(left.Placements) != len(right.Placements) {
		return false
	}
	for key, value := range left.Placements {
		if other, ok := right.Placements[key]; !ok || value != other {
			return false
		}
	}
	return callOutcomeRepresentationEqualResults(left.Results, right.Results) &&
		callOutcomeRepresentationEqualParamObligations(left.ParamObligations, right.ParamObligations) &&
		callOutcomeRepresentationEqualPathObligations(left.PathObligations, right.PathObligations) &&
		callOutcomeRepresentationEqualTypestateRequirements(left.TypestateRequirements, right.TypestateRequirements) &&
		callOutcomeRepresentationEqualParamPathRefinements(left.ParamPathRefinements, right.ParamPathRefinements) &&
		callOutcomeRepresentationEqualParamPathWrites(left.ParamPathWrites, right.ParamPathWrites) &&
		callOutcomeRepresentationEqualParamLengthFloors(left.ParamLengthFloors, right.ParamLengthFloors) &&
		callOutcomeRepresentationEqualParamPathInvalidations(left.ParamPathInvalidations, right.ParamPathInvalidations) &&
		callOutcomeRepresentationEqualParamConditions(left.ParamConditions, right.ParamConditions) &&
		callOutcomeRepresentationEqualParamPathRelations(left.ParamPathRelations, right.ParamPathRelations) &&
		callOutcomeRepresentationEqualReturnConditionRefinements(left.ReturnConditionRefinements, right.ReturnConditionRefinements) &&
		callOutcomeRepresentationEqualReturnConditionSlots(left.ReturnConditionSlots, right.ReturnConditionSlots) &&
		callOutcomeRepresentationEqualReturnPresenceRelations(left.ReturnPresenceRelations, right.ReturnPresenceRelations) &&
		callOutcomeRepresentationEqualParamExposures(left.ParamExposures, right.ParamExposures)
}

func callOutcomeRepresentationEqualSlow(left, right CallOutcome) bool {
	return reflect.DeepEqual(left, right)
}

// callOutcomeRepresentationNormalReturnFactsEmpty is deliberately stricter
// than NormalReturnFacts.Empty: raw representation equality distinguishes a
// nil lane from an allocated-but-empty lane, while semantic emptiness does not.
func callOutcomeRepresentationNormalReturnFactsEmpty(f callboundary.NormalReturnFacts) bool {
	return f.PathRefinements == nil &&
		f.PersistentPathWrites == nil &&
		f.PathStaticMembers == nil &&
		f.PathStaticMemberDeltas == nil &&
		f.PathInvalidations == nil &&
		f.DynamicIndexFacts == nil &&
		f.KeyMemberships == nil &&
		f.DynamicValueKeys == nil &&
		f.DynamicAllValues == nil &&
		f.BranchProofs == nil &&
		f.PathPresenceImplications == nil &&
		f.ChannelSelects == nil &&
		f.FrozenTables == nil &&
		f.EffectDeltas == nil &&
		f.EscapeEvents == nil &&
		f.StoreRelations == nil &&
		f.LifecycleFacts == nil &&
		f.NumFloors == nil &&
		f.NumCeils == nil &&
		f.RelConstraints == nil
}

func callOutcomeRepresentationEqualPath(left, right pathdom.Path) bool {
	if left.Root != right.Root || left.Symbol != right.Symbol || left.Version != right.Version ||
		(left.Segments == nil) != (right.Segments == nil) || len(left.Segments) != len(right.Segments) {
		return false
	}
	for index := range left.Segments {
		if left.Segments[index] != right.Segments[index] {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualResults(left, right []CallResult) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Index != right[index].Index || left[index].Value != right[index].Value {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualParamObligationOrigin(left, right CallParamObligationOrigin) bool {
	return left.HasOrigin == right.HasOrigin && left.ReceiverParam == right.ReceiverParam &&
		left.ReceiverPath == right.ReceiverPath && left.Member == right.Member && left.ArgParam == right.ArgParam &&
		left.MemberParamIndex == right.MemberParamIndex && left.SubjectLabel == right.SubjectLabel && left.ProviderLabel == right.ProviderLabel
}

func callOutcomeRepresentationEqualParamObligations(left, right []CallParamObligation) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.ParamIndex != b.ParamIndex || a.Value != b.Value || a.SignatureSurface != b.SignatureSurface ||
			!callOutcomeRepresentationEqualParamObligationOrigin(a.Origin, b.Origin) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualPathObligations(left, right []CallPathObligation) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Value != right[index].Value || !callOutcomeRepresentationEqualPath(left[index].Path, right[index].Path) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualTypestateRequirements(left, right []CallTypestateRequirement) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.Protocol != b.Protocol || a.State != b.State || !callOutcomeRepresentationEqualPath(a.Target, b.Target) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualParamPathRefinements(left, right []CallParamPathRefinement) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Value != right[index].Value || !callOutcomeRepresentationEqualPath(left[index].Path, right[index].Path) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualParamPathWrites(left, right []CallParamPathWrite) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Value != right[index].Value || !callOutcomeRepresentationEqualPath(left[index].Path, right[index].Path) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualParamLengthFloors(left, right []CallParamLengthFloor) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Floor != right[index].Floor || !callOutcomeRepresentationEqualPath(left[index].Path, right[index].Path) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualParamPathInvalidations(left, right []CallParamPathInvalidation) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].PreserveStructuralWitness != right[index].PreserveStructuralWitness ||
			!callOutcomeRepresentationEqualPath(left[index].Path, right[index].Path) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualParamConditions(left, right []CallParamCondition) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualParamPathRelations(left, right []CallParamPathRelation) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.Kind != b.Kind || !callOutcomeRepresentationEqualPath(a.Left, b.Left) || !callOutcomeRepresentationEqualPath(a.Right, b.Right) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualReturnConditionRefinements(left, right []CallReturnConditionRefinement) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.ReturnIndex != b.ReturnIndex || a.ReturnValue != b.ReturnValue || a.Value != b.Value ||
			!callOutcomeRepresentationEqualPath(a.Target, b.Target) {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualReturnConditionSlots(left, right []CallReturnConditionSlotRefinement) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.ReturnIndex != b.ReturnIndex || a.ReturnValue != b.ReturnValue || a.TargetIndex != b.TargetIndex || a.Value != b.Value {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualReturnPresenceRelations(left, right []CallReturnPresenceRelation) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func callOutcomeRepresentationEqualParamExposures(left, right []CallParamExposure) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.Contract != b.Contract || a.Kind != b.Kind || !callOutcomeRepresentationEqualPath(a.Source, b.Source) {
			return false
		}
	}
	return true
}

// FingerprintCallOutcomeRepresentation is a collision-tolerant accelerator
// for CallOutcomeRepresentationEqual. It deliberately hashes only stable raw
// shape and result identities; exact representation equality remains the
// authority inside buckets. Unlike FingerprintCallOutcome it performs no
// normalization and allocates no canonical fact collections.
func FingerprintCallOutcomeRepresentation(reg *axis.Registry, outcome CallOutcome) uint64 {
	w := internalhash.NewWriter()
	_, _ = w.WriteString("callpayload.CallOutcome/raw/v1")
	for _, lane := range callOutcomeLanes {
		_, _ = w.WriteString(lane.fieldName)
		w.WriteBool(lane.has(outcome))
	}
	w.WriteIntDecimal(int64(len(outcome.Results)))
	for _, result := range outcome.Results {
		w.WriteIntDecimal(int64(result.Index))
		if reg != nil {
			w.WriteUintHex(product.Hash(reg, result.Value))
		}
	}
	return w.Sum64()
}

type callOutcomeLane struct {
	fieldName   string
	postReturn  bool
	has         func(CallOutcome) bool
	transaction *callOutcomeTransactionRole
}

// CallOutcomeFieldRole describes one registered CallOutcome field's boundary
// role. It intentionally omits the lane's predicate: callers may classify
// fields, but only callpayload owns field-presence semantics.
type CallOutcomeFieldRole struct {
	FieldName   string
	PostReturn  bool
	transaction *callOutcomeTransactionRole
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
			FieldName:   lane.fieldName,
			PostReturn:  lane.postReturn,
			transaction: lane.transaction,
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

var callOutcomeLanes = derivedCallOutcomeLanes()

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
	ParamIndex       int
	Value            product.Value
	Origin           CallParamObligationOrigin
	SignatureSurface bool
}

// CallPathObligation records a pre-call value constraint for a caller-visible
// local/captured path. It is diagnostic evidence only: callers may project it
// back to their own parameters when the path is a stable local derived from a
// parameter, but fact application never writes it as a post-call refinement.
type CallPathObligation struct {
	Path  pathdom.Path
	Value product.Value
}

// CallTypestateRequirement is a diagnostic-only call-entry precondition over
// a path-bound resource. Unlike LifecycleFacts it neither mutates nor assumes
// a normal return; the obligation pass evaluates it against the caller state.
type CallTypestateRequirement struct {
	Target   pathdom.Path
	Protocol typestate.Protocol
	State    typestate.State
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
