// Package formal consumes Target operation ownership metadata at ordinary
// mounted call sites and projects the resulting demand onto Placement's
// Heap-aligned allocation roots.
//
// The package is deliberately a consumer only.  Target remains the sole
// owner of FormalEffect rows, Value remains the sole owner of Value
// coordinates and rooted references, and Heap remains the sole owner of
// allocation keys.  No publication descriptor or Effect result is retained
// here.
package formal

import (
	"crypto/sha256"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type operand struct {
	packs   *packdomain.Schema
	mounted calldomain.MountedCall
	key     calldomain.Key
	id      identity.ContentID
}

func mountedOperandID(module, occurrence, application, keyID identity.ContentID) identity.ContentID {
	const prefix = "wippy.analysis.placement.formal.invocation.v1\x00"
	hash := sha256.New()
	_, _ = hash.Write([]byte(prefix))
	_, _ = hash.Write(module[:])
	_, _ = hash.Write(occurrence[:])
	_, _ = hash.Write(application[:])
	_, _ = hash.Write(keyID[:])
	return identity.ContentID(hash.Sum(nil))
}

func mountedForOperand(packs *packdomain.Schema, algebra *calldomain.Algebra, candidate operand) (packdomain.MountedActualProjection, identity.ContentID, identity.ContentID, identity.ContentID, bool) {
	if packs == nil || algebra == nil || !algebra.Valid() || candidate.packs != packs || !candidate.mounted.Valid() ||
		!packs.LinkOwner().Available() || !packs.LinkOwner().Matches(algebra.LinkOwner()) {
		return packdomain.MountedActualProjection{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	application, occurrence, module, _, _, identityOK := algebra.MountedCallIdentity(candidate.mounted)
	key, keyOK := algebra.KeyForMountedCall(candidate.mounted)
	keyID, keyIDOK := key.ContentID()
	actual, actualOK := packs.MountedActualProjection(module, occurrence)
	if !actualOK || !actual.Valid() || !actual.OwnedBy(packs) || !identityOK || !keyOK || !key.IsApplication() || !key.Valid() || !keyIDOK || !keyID.Available() || candidate.key != key {
		return packdomain.MountedActualProjection{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	expected := mountedOperandID(module, occurrence, application, keyID)
	if candidate.id != expected || !expected.Available() {
		return packdomain.MountedActualProjection{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	return actual, module, occurrence, application, true
}

func operandContent(packs *packdomain.Schema, algebra *calldomain.Algebra, candidate operand) (operand, [32]byte, bool) {
	_, _, _, _, ok := mountedForOperand(packs, algebra, candidate)
	if !ok {
		return operand{}, [32]byte{}, false
	}
	return candidate, [32]byte(candidate.id), true
}

func operandForOccurrence(packs *packdomain.Schema, algebra *calldomain.Algebra, module, occurrence identity.ContentID) (operand, bool) {
	if packs == nil || algebra == nil || !algebra.Valid() || !packs.LinkOwner().Available() || !packs.LinkOwner().Matches(algebra.LinkOwner()) {
		return operand{}, false
	}
	mounted, mountedOK := algebra.MountedCallForOccurrence(module, occurrence)
	application, callID, moduleID, _, _, identityOK := algebra.MountedCallIdentity(mounted)
	key, keyOK := algebra.KeyForMountedCall(mounted)
	keyID, keyIDOK := key.ContentID()
	if !mountedOK || !identityOK || callID != occurrence || moduleID != module || !keyOK || !key.IsApplication() || !key.Valid() || !keyIDOK || !keyID.Available() {
		return operand{}, false
	}
	actual, actualOK := packs.MountedActualProjection(module, occurrence)
	if !actualOK || !actual.Valid() || !actual.OwnedBy(packs) {
		return operand{}, false
	}
	id := mountedOperandID(module, occurrence, application, keyID)
	return operand{packs: packs, mounted: mounted, key: key, id: id}, id.Available()
}

// formalSelectorRange is the allocation-free representation used by the
// solve-time reducer. A formal selector is always either empty, one fixed
// index, or a contiguous suffix of the mounted actual prefix. Keeping the
// half-open interval here lets planFor inspect the selector semantics without
// materializing a selected-index slice.
type formalSelectorRange struct {
	start   int
	end     int
	unknown bool
	owns    bool
	// valid distinguishes an authenticated empty/non-displacing selector from
	// an authored selector that cannot be redeemed at this mounted actual
	// cut.  Unknown is reserved for a real open boundary (the mounted Pack
	// tail), never for a fixed coordinate that happened to be unavailable.
	valid bool
}

func resolveFormalSelectorRange(spec vocabulary.FormalEffectSpec, actualCount int, runtimeTail bool) formalSelectorRange {
	result := formalSelectorRange{valid: true}
	if actualCount < 0 {
		result.valid = false
		return result
	}
	switch spec.Kind {
	case vocabulary.FormalEffectBorrowAll:
		return result
	case vocabulary.FormalEffectBorrow, vocabulary.FormalEffectFreeze:
		// Borrow and Freeze do not write Placement, but their fixed selector is
		// still part of the authenticated formal row.  A missing fixed actual
		// is a refusal, not evidence for an all-root route.  The -1 form is an
		// authored unresolved/non-displacing selector and carries no route.
		if spec.Param == -1 {
			return result
		}
		if spec.Param < 0 || int(spec.Param) >= actualCount {
			result.valid = false
		}
		return result
	case vocabulary.FormalEffectRetain,
		vocabulary.FormalEffectStore,
		vocabulary.FormalEffectSendParam,
		vocabulary.FormalEffectExport,
		vocabulary.FormalEffectOpaque:
		result.owns = true
		if spec.Param == -1 {
			if runtimeTail {
				result.unknown = true
				return result
			}
			if actualCount == 0 {
				result.valid = false
			} else {
				result.start = actualCount - 1
				result.end = actualCount
			}
			return result
		}
		if spec.Param < 0 {
			result.valid = false
			return result
		}
		if int(spec.Param) >= actualCount {
			// A mounted open Pack tail is the only authority that can make an
			// otherwise unavailable fixed ordinal redeemable.  Keep the
			// selector unresolved and widen only because that boundary is
			// explicitly authenticated by Pack.
			if runtimeTail {
				result.unknown = true
				return result
			}
			result.valid = false
			return result
		}
		result.start = int(spec.Param)
		result.end = result.start + 1
		return result
	case vocabulary.FormalEffectSendSuffix:
		result.owns = true
		if spec.FromParam < 0 {
			result.valid = false
			return result
		}
		if int(spec.FromParam) > actualCount {
			if runtimeTail {
				result.start = actualCount
				result.end = actualCount
				result.unknown = true
				return result
			}
			result.valid = false
			return result
		}
		result.start = int(spec.FromParam)
		result.end = actualCount
		if runtimeTail {
			// A runtime tail can contain an actual at every position after the
			// fixed prefix; the conservative choice is therefore unknown for
			// every suffix boundary, including one beyond the prefix.
			result.unknown = true
		}
		return result
	default:
		result.valid = false
		return result
	}
}

// FormalEscape maps an ownership-bearing formal kind to Placement's stable
// escape vocabulary. Non-displacing kinds return false.
func FormalEscape(kind vocabulary.FormalEffectKind) (placement.Escape, bool) {
	switch kind {
	case vocabulary.FormalEffectRetain:
		return placement.Retain, true
	case vocabulary.FormalEffectStore:
		return placement.Store, true
	case vocabulary.FormalEffectSendSuffix, vocabulary.FormalEffectSendParam:
		return placement.Send, true
	case vocabulary.FormalEffectExport:
		return placement.Export, true
	case vocabulary.FormalEffectOpaque:
		return placement.Opaque, true
	default:
		return placement.None, false
	}
}

type actualObservation struct {
	fact    valuedomain.Value
	present bool
	valid   bool
}

type route struct {
	key     heap.Key
	escape  placement.Escape
	unknown bool
	tag     routeTag
}

type routePlan struct {
	routes []route
}

const (
	routeTagShift    = uint(4)
	routeTagMask     = uint64(0x0f)
	routeCodeUnknown = uint64(0x0f)
)

type routeTag uint64

func routeTagFor(schema placement.Schema, key heap.Key, escape placement.Escape, unknown bool) (routeTag, bool) {
	if !schema.Valid() || !key.Valid() || key.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(key) {
		return 0, false
	}
	dense, denseOK := schema.Heap().KeyIndex(key)
	canonical, canonicalOK := schema.KeyAt(dense)
	if !denseOK || !canonicalOK || canonical != key || dense < 0 {
		return 0, false
	}
	code := uint64(escape) + 1
	if unknown {
		code = routeCodeUnknown
	}
	if code == 0 || code > routeTagMask {
		return 0, false
	}
	return routeTag((uint64(dense)+1)<<routeTagShift | code), true
}

func strongerEscape(left, right placement.Escape) placement.Escape {
	leftPlacement, leftOK := left.Placement()
	rightPlacement, rightOK := right.Placement()
	if !leftOK {
		return right
	}
	if !rightOK {
		return left
	}
	if placement.Join(leftPlacement, rightPlacement) == rightPlacement && leftPlacement != rightPlacement {
		return right
	}
	if placement.Join(leftPlacement, rightPlacement) == leftPlacement && leftPlacement != rightPlacement {
		return left
	}
	// Retain/Store and Send/Export/Opaque have equal Placement but retaining
	// the later canonical kind keeps provenance deterministic without making
	// it semantically stronger.
	if right > left {
		return right
	}
	return left
}

type routeDemand struct {
	escape  placement.Escape
	unknown bool
}

func (plan routePlan) seal(schema placement.Schema, demands map[heap.Key]routeDemand) (routePlan, bool) {
	if !schema.Valid() {
		return routePlan{}, false
	}
	// Demands are an ephemeral reduction directory: they deduplicate roots
	// while formal rows are being joined, but they do not define route order.
	// Emit by walking Heap's canonical dense coordinates once. This preserves
	// the routeTag order required by routeForTag without sorting the map's R
	// entries (and without retaining a second dense index).
	//
	// Keep the expected count so a foreign, malformed, or non-allocation key
	// cannot be silently dropped merely because it is absent from the dense
	// Heap walk. The zero key is the private all-root sentinel and is excluded
	// exactly as in the previous map iteration.
	expected := len(demands)
	if _, sentinel := demands[heap.Key{}]; sentinel {
		expected--
	}
	if expected < 0 {
		return routePlan{}, false
	}
	sealed := routePlan{routes: make([]route, 0, expected)}
	for dense := 0; dense < schema.DenseKeyCount(); dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return routePlan{}, false
		}
		if key.Kind() != heap.RootAllocation {
			continue
		}
		demand, demanded := demands[key]
		if !demanded {
			continue
		}
		tag, tagOK := routeTagFor(schema, key, demand.escape, demand.unknown)
		if !tagOK {
			return routePlan{}, false
		}
		sealed.routes = append(sealed.routes, route{key: key, escape: demand.escape, unknown: demand.unknown, tag: tag})
	}
	if len(sealed.routes) != expected {
		return routePlan{}, false
	}
	return sealed, true
}

func addUnknownAll(schema placement.Schema, demands map[heap.Key]routeDemand) bool {
	if !schema.Valid() || demands == nil {
		return false
	}
	// A zero Heap key is never issued. Keep it as a private sentinel so
	// repeated conservative-widening requests become O(1) after the first
	// all-root pass; the sentinel is stripped before route sealing.
	if _, alreadyUnknown := demands[heap.Key{}]; alreadyUnknown {
		return true
	}
	// Widen directly over Heap's dense coordinate space. This is the same
	// canonical order used by routePlan.seal, so widening needs no temporary
	// allocation-key slice and does not create a second root index.
	for dense := 0; dense < schema.DenseKeyCount(); dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return false
		}
		if key.Kind() != heap.RootAllocation {
			continue
		}
		previous := demands[key]
		previous.unknown = true
		demands[key] = previous
	}
	demands[heap.Key{}] = routeDemand{unknown: true}
	return true
}

func addFactDemand(schema placement.Schema, values *valuedomain.Schema, fact valuedomain.Value, present bool, escape placement.Escape, demands map[heap.Key]routeDemand) (unknown bool, ok bool) {
	if schema.Valid() == false || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) || demands == nil {
		return false, false
	}
	// Presence is a Value observation fact, not a conservative seed.  An
	// absent fixed actual cannot identify a root and therefore refuses the
	// formal plan.  Authenticate the Value before honoring Top as a widening
	// witness; a foreign Top must not cross this factor's owner fence.
	if !present || !values.Equal(fact, fact) {
		return false, false
	}
	if fact.IsTop() {
		return true, true
	}
	heapSchema := schema.Heap()
	valid := true
atoms:
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			valid = false
			break
		}
		classification, classificationOK := placement.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			valid = false
			break atoms
		}
		switch classification.Class {
		case placement.AtomClassAllocation:
			key := classification.Key
			if !heapSchema.OwnsKey(key) || key.Kind() != heap.RootAllocation {
				valid = false
				break atoms
			}
			if !planAddDemand(schema, key, escape, false, demands) {
				valid = false
				break atoms
			}
		case placement.AtomClassOpaque:
			unknown = true
		}
	}
	return unknown, valid
}

// addOpenTailDemand projects one fixed actual under an unknown formal tail.
// It mirrors the conservative tail rule while traversing Value atoms
// directly, avoiding a temporary atom slice on every actual.
func addOpenTailDemand(schema placement.Schema, values *valuedomain.Schema, fact valuedomain.Value, demands map[heap.Key]routeDemand) bool {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) || demands == nil {
		return false
	}
	// Top is a lawful widening witness only after the actual Value has been
	// authenticated against this exact Value schema.
	if !values.Equal(fact, fact) {
		return false
	}
	if fact.IsTop() {
		return addUnknownAll(schema, demands)
	}
	heapSchema := schema.Heap()
	valid := true
atoms:
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			valid = false
			break
		}
		classification, classificationOK := placement.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			valid = false
			break atoms
		}
		switch classification.Class {
		case placement.AtomClassAllocation:
			key := classification.Key
			if !heapSchema.OwnsKey(key) || key.Kind() != heap.RootAllocation || !planAddDemand(schema, key, placement.None, true, demands) {
				valid = false
				break atoms
			}
		case placement.AtomClassOpaque:
			if !addUnknownAll(schema, demands) {
				valid = false
				break atoms
			}
		}
	}
	return valid
}

// addUnknownOpenTailObservationDemand applies the unknown formal tail to one
// present, authenticated fixed actual. Missing observations refuse the plan;
// an open Pack tail is widened only by planFor's separate authenticated tail
// branch.
func addUnknownOpenTailObservationDemand(schema placement.Schema, values *valuedomain.Schema, observation actualObservation, demands map[heap.Key]routeDemand) bool {
	// An open formal row can widen an authenticated fixed Value, but it cannot
	// turn a missing/ill-shaped observation into an all-root claim.  Pack's
	// independently authenticated open actual tail is handled by planFor's
	// explicit runtimeTail branch.
	if !observation.valid || !observation.present {
		return false
	}
	return addOpenTailDemand(schema, values, observation.fact, demands)
}

func planAddDemand(schema placement.Schema, key heap.Key, escape placement.Escape, unknown bool, demands map[heap.Key]routeDemand) bool {
	if !schema.Valid() || !key.Valid() || key.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(key) || demands == nil {
		return false
	}
	allUnknown := false
	if _, marked := demands[heap.Key{}]; marked {
		allUnknown = true
	}
	previous, found := demands[key]
	if !found {
		demands[key] = routeDemand{escape: escape, unknown: unknown || allUnknown}
		return true
	}
	if previous.unknown || unknown || allUnknown {
		previous.unknown = true
		demands[key] = previous
		return true
	}
	previous.escape = strongerEscape(previous.escape, escape)
	demands[key] = previous
	return true
}

// planFor is the one formal-to-placement reduction used by both transfer and
// derivation evidence. It accepts only already-selected Call/Value facts and
// emits exact owner-fenced allocation keys or conservative all-root routes.
func planFor(packs *packdomain.Schema, calls *calldomain.Algebra, schema placement.Schema, values *valuedomain.Schema, targetContract *contract.Contract, mounted calldomain.MountedCall, callFact calldomain.Value, observations []actualObservation) (routePlan, bool) {
	if packs == nil || calls == nil {
		return routePlan{}, false
	}
	_, callID, module, _, _, identityOK := calls.MountedCallIdentity(mounted)
	actual, actualOK := packs.MountedActualProjection(module, callID)
	key, keyOK := calls.KeyForMountedCall(mounted)
	actualCount := actual.ActualCount()
	_, runtimeTail := actual.TailID()
	if !calls.Valid() || !identityOK || !packs.LinkOwner().Available() || !packs.LinkOwner().Matches(calls.LinkOwner()) ||
		!actualOK || !actual.Valid() || !actual.OwnedBy(packs) || !keyOK ||
		values == nil || !values.Valid() || targetContract == nil || !schema.Valid() || !values.OwnsHeapSchema(schema.Heap()) ||
		!values.LinkOwner().Matches(calls.LinkOwner()) ||
		!calls.OwnsTargetContract(targetContract) || len(observations) != actualCount || !calls.Admits(key, callFact) {
		return routePlan{}, false
	}
	demands := make(map[heap.Key]routeDemand, len(observations))
	// Call uncertainty is not a Placement witness.  An open/Top dispatch
	// value does not authenticate a Target operation or its formal rows, so a
	// route must be refused rather than compensated with all-root Unknown.
	if callFact.IsTop() || callFact.HasOpaqueAlternative() {
		return routePlan{}, false
	}
	// Every fixed actual participating in this selected-read cut must carry a
	// present, owner-authenticated Value observation.  Missing rows are an
	// unavailable planner input, never a request to manufacture all-root
	// Unknown.  Authenticated open Pack tails are widened only by the explicit
	// runtimeTail branch below.
	for _, observation := range observations {
		if !observation.valid || !observation.present || !values.Equal(observation.fact, observation.fact) {
			return routePlan{}, false
		}
	}
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK || !calls.OwnsTarget(target) {
			return routePlan{}, false
		}
		operation, operationOK := target.Operation()
		if !operationOK {
			// Body targets do not denote a Target operation and therefore do
			// not carry formal ownership metadata.
			continue
		}
		if operation == 0 {
			return routePlan{}, false
		}
		declared, declaredOK := targetContract.Operations.OperationAt(int(operation) - 1)
		if !declaredOK || declared != operation {
			return routePlan{}, false
		}
		tail, tailOK := targetContract.Operations.FormalEffectTail(operation)
		if !tailOK || (tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen) {
			return routePlan{}, false
		}
		for effectIndex := 0; effectIndex < targetContract.Operations.FormalEffectCount(operation); effectIndex++ {
			spec, specOK := targetContract.Operations.FormalEffectAt(operation, effectIndex)
			if !specOK {
				return routePlan{}, false
			}
			selection := resolveFormalSelectorRange(spec, actualCount, runtimeTail)
			if !selection.valid {
				// Fixed formal coordinates and malformed selector geometry fail
				// closed.  Only selection.unknown, which is set by an
				// authenticated mounted Pack tail, may widen all roots.
				return routePlan{}, false
			}
			escape, owns := FormalEscape(spec.Kind)
			if !owns {
				continue
			}
			if selection.unknown {
				if !addUnknownAll(schema, demands) {
					return routePlan{}, false
				}
			}
			for actualIndex := selection.start; actualIndex < selection.end; actualIndex++ {
				if actualIndex < 0 || actualIndex >= len(observations) {
					// The selector/observation shape was not jointly
					// redeemable.  This is a planner failure, not an open
					// boundary, so refuse instead of compensating with an
					// all-root Unknown demand.
					return routePlan{}, false
				}
				observation := observations[actualIndex]
				unknown, observationOK := addFactDemand(schema, values, observation.fact, observation.present && observation.valid, escape, demands)
				if !observationOK {
					return routePlan{}, false
				}
				if unknown && !addUnknownAll(schema, demands) {
					return routePlan{}, false
				}
			}
		}
		if tail == vocabulary.RowUnknownOpen {
			// If the mounted call itself has an open actual tail, that tail is
			// not represented by a fixed Value coordinate. A tracked allocation
			// may therefore enter through the runtime suffix, so widen every
			// Placement root before inspecting the fixed prefix.
			if runtimeTail && !addUnknownAll(schema, demands) {
				return routePlan{}, false
			}
			// The unknown formal tail is an unknown ownership effect over all
			// fixed actuals, but it does not fabricate a Value for an
			// unrepresented runtime tail member.
			for _, observation := range observations {
				if !addUnknownOpenTailObservationDemand(schema, values, observation, demands) {
					return routePlan{}, false
				}
			}
		}
	}
	return (&routePlan{}).seal(schema, demands)
}

func routeForTag(plan routePlan, tag routeTag) (route, bool) {
	// routePlan.seal sorts by Heap dense coordinate, and routeTagFor embeds
	// that coordinate above the low escape bits. Use the sorted invariant so
	// checker lookups remain logarithmic for all-root widening plans.
	index := sort.Search(len(plan.routes), func(index int) bool {
		return plan.routes[index].tag >= tag
	})
	if index < len(plan.routes) && plan.routes[index].tag == tag {
		return plan.routes[index], true
	}
	return route{}, false
}
