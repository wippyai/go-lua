package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func (c *Contract) CallbackCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.callbacks.len()
}

func (c *Contract) CallbackAt(op Operation, index int) (CallbackID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.callbacks.len() {
		return 0, false
	}
	return CallbackID(row.callbacks.start + uint32(index) + 1), true
}

func (c *Contract) callback(id CallbackID) (callbackRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.callbacks)) {
		return callbackRow{}, false
	}
	return c.callbacks[uint32(id)-1], true
}

// CallbackOwner returns the exact sealed operation that owns a callback
// correspondence. The range validation keeps a malformed callback row from
// being accepted merely because its stored owner is an otherwise valid
// operation.
func (c *Contract) CallbackOwner(id CallbackID) (Operation, bool) {
	row, ok := c.callback(id)
	if !ok || row.owner == 0 {
		return 0, false
	}
	owner, ok := c.operation(row.owner)
	if !ok {
		return 0, false
	}
	index := uint32(id) - 1
	if index < owner.callbacks.start || index >= owner.callbacks.end {
		return 0, false
	}
	return row.owner, true
}

// CallbackFunction returns the exact input authority that supplies a callback
// function. Authored callbacks use ValueFormal; the opaque callback uses the
// sole maximal AllInputs authority.
func (c *Contract) CallbackFunction(id CallbackID) (InputSource, bool) {
	row, ok := c.callback(id)
	if !ok {
		return InputSource{}, false
	}
	return row.function, true
}

// CallbackArguments returns the full Values schema at the callback argument
// role. Equal Values handles across roles are structural deduplication only,
// never a flow claim.
func (c *Contract) CallbackArguments(id CallbackID) (Values, bool) {
	row, ok := c.callback(id)
	return row.arguments, ok
}

// CallbackOutcome returns the exact Values relation carried by one callback
// activation outcome. It prescribes no provider response to that outcome.
func (c *Contract) CallbackOutcome(id CallbackID, kind flowkind.OutcomeKind) (Values, bool) {
	index, valid := crossActivationOutcomeIndex(kind)
	if !valid {
		return 0, false
	}
	row, ok := c.callback(id)
	if !ok {
		return 0, false
	}
	return row.outcomes[index], true
}

// CallbackAdmission returns the callback's sole callable convention. A
// callback-backed Subedge projects this value; it never carries a duplicate.
func (c *Contract) CallbackAdmission(id CallbackID) (schematype.CallableAdmission, bool) {
	row, ok := c.callback(id)
	return row.admission, ok
}

// CallbackOpaque exposes the one explicit maximally conservative callback
// owned by the synthesized opaque operation. Missing authored Subedge rows do
// not imply a callback is closed, successful, or non-reentrant.
func (c *Contract) CallbackOpaque(id CallbackID) bool {
	owner, ok := c.CallbackOwner(id)
	return ok && owner == c.opaque
}

// CallbackSubedge returns the sole immediate typed application of callback.
// It is a derived sealed reverse index over SubedgeCallback, not a second
// execution relation or semantic identity coordinate. Retained and opaque
// callbacks intentionally have no immediate Subedge.
func (c *Contract) CallbackSubedge(id CallbackID) (SubedgeID, bool) {
	callback, ok := c.callback(id)
	if !ok || callback.subedge == 0 {
		return 0, false
	}
	edge, ok := c.subedge(callback.subedge)
	if !ok || edge.callee != SubedgeCalleeCallback || edge.callback != id {
		return 0, false
	}
	return callback.subedge, true
}

// SubedgeCount returns every sealed typed internal application owned by op.
func (c *Contract) SubedgeCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.subedges.len()
}

func (c *Contract) SubedgeAt(op Operation, index int) (SubedgeID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.subedges.len() {
		return 0, false
	}
	return SubedgeID(row.subedges.start + uint32(index) + 1), true
}

func (c *Contract) subedge(id SubedgeID) (subedgeRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.subedges)) {
		return subedgeRow{}, false
	}
	return c.subedges[id-1], true
}

// SubedgeOwner is the only Target ownership projection needed by Link. It
// deliberately does not create a Candidate, Application, or Program Term.
func (c *Contract) SubedgeOwner(id SubedgeID) (Operation, bool) {
	row, ok := c.subedge(id)
	if !ok || row.owner == 0 {
		return 0, false
	}
	owner, ok := c.operation(row.owner)
	if !ok {
		return 0, false
	}
	index := uint32(id) - 1
	if index < owner.subedges.start || index >= owner.subedges.end {
		return 0, false
	}
	return row.owner, true
}

func (c *Contract) SubedgeRole(id SubedgeID) (uint32, bool) {
	row, ok := c.subedge(id)
	return row.role, ok
}

func (c *Contract) SubedgeFamily(id SubedgeID) (SubedgeFamily, bool) {
	row, ok := c.subedge(id)
	return row.family, ok
}

func (c *Contract) SubedgeCallee(id SubedgeID) (SubedgeCalleeKind, bool) {
	row, ok := c.subedge(id)
	return row.callee, ok
}

func (c *Contract) SubedgeAdmission(id SubedgeID) (schematype.CallableAdmission, bool) {
	row, ok := c.subedge(id)
	return row.admission, ok
}

func (c *Contract) SubedgeCallback(id SubedgeID) (CallbackID, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != SubedgeCalleeCallback || row.callback == 0 {
		return 0, false
	}
	return row.callback, true
}

// SubedgeCapturedInitialRead reports the capture-once boot source owned by
// this edge. The SubedgeID itself is its operation-local capture identity.
func (c *Contract) SubedgeCapturedInitialRead(id SubedgeID) (InitialRoot, ExactKey, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != SubedgeCalleeCapturedInitialRead || row.readRoot == 0 || row.readKey == 0 {
		return 0, 0, false
	}
	return row.readRoot, row.readKey, true
}

func (c *Contract) SubedgeMetaKey(id SubedgeID) (ExactKey, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != SubedgeCalleeMetaKey || row.metaKey == 0 {
		return 0, false
	}
	return row.metaKey, true
}

// SubedgeArguments returns the contextual callee-argument Values endpoint.
// Equal Values handles in another role are never implicit dataflow.
func (c *Contract) SubedgeArguments(id SubedgeID) (Values, bool) {
	row, ok := c.subedge(id)
	return row.arguments, ok
}

// SubedgeRuleEntry reports whether an argument-free Subedge has explicit
// owner-Rule entry authority. Nonempty direct arguments use ArgumentOrigins.
func (c *Contract) SubedgeRuleEntry(id SubedgeID) (bool, bool) {
	row, ok := c.subedge(id)
	return row.ruleEntry, ok
}

// ArgumentOriginCount reports the authored complete source set for this
// contextual argument endpoint. A zero count is either the explicit nullary
// Rule entry reported by SubedgeRuleEntry or a complete sibling/admission
// route; it never implies an entry by itself.
func (c *Contract) ArgumentOriginCount(id SubedgeID) int {
	row, ok := c.subedge(id)
	if !ok {
		return 0
	}
	return row.argumentOrigins.len()
}

// ArgumentOriginAt returns one direct owner-input or owner-Rule source
// for a single argument Values segment. The source is zero for Rule entries.
func (c *Contract) ArgumentOriginAt(id SubedgeID, index int) (segment ArgumentSegment, ordinal uint32, source ArgumentSource, input InputSource, ok bool) {
	row, found := c.subedge(id)
	if !found || index < 0 || index >= row.argumentOrigins.len() {
		return ArgumentSegmentInvalid, 0, ArgumentSourceInvalid, InputSource{}, false
	}
	item := c.subedgeOrigins[row.argumentOrigins.start+uint32(index)]
	return item.segment, item.index, item.kind, item.source, true
}

func (c *Contract) SubedgeTerminal(id SubedgeID, kind flowkind.OutcomeKind) (Values, bool) {
	index, valid := crossActivationOutcomeIndex(kind)
	if !valid {
		return 0, false
	}
	row, ok := c.subedge(id)
	if !ok {
		return 0, false
	}
	return row.outcomes[index], true
}

// AdmissionFailure returns the distinct exact Values source produced
// when this edge's callable admission fails. It is neither candidate absence
// nor the callee Throw terminal.
func (c *Contract) AdmissionFailure(id SubedgeID) (Values, bool) {
	row, ok := c.subedge(id)
	if !ok || row.admissionFailure == 0 {
		return 0, false
	}
	return row.admissionFailure, true
}

// AdmissionRoute returns the one explicit transport of a callable
// admission failure. Only Outcome and Subedge routes are representable.
func (c *Contract) AdmissionRoute(id SubedgeID) (route SubedgeRoute, adjustment Adjustment, result Values, placement Placement, offset uint32, outcome uint32, sibling SubedgeID, destination Values, ok bool) {
	row, found := c.subedge(id)
	if !found || row.admissionRoute.route == RouteInvalid {
		return RouteInvalid, AdjustmentInvalid, 0, PlacementInvalid, 0, 0, 0, 0, false
	}
	item := row.admissionRoute
	return item.route, item.adjustment, item.result, item.placement, item.offset, item.outcome, item.subedge, item.destination, true
}

// SubedgeRouteAt returns the contextual projected Result and its one route.
// RejectYield's Result is the canonical C-boundary error Values, not the
// discarded child Yield payload; it may target an owner Throw or sibling edge.
func (c *Contract) SubedgeRouteAt(id SubedgeID, kind flowkind.OutcomeKind) (route SubedgeRoute, adjustment Adjustment, result Values, placement Placement, offset uint32, outcome uint32, sibling SubedgeID, destination Values, ok bool) {
	index, valid := crossActivationOutcomeIndex(kind)
	if !valid {
		return RouteInvalid, AdjustmentInvalid, 0, PlacementInvalid, 0, 0, 0, 0, false
	}
	row, found := c.subedge(id)
	if !found {
		return RouteInvalid, AdjustmentInvalid, 0, PlacementInvalid, 0, 0, 0, 0, false
	}
	item := row.routes[index]
	if item.route == RouteInvalid {
		return RouteInvalid, AdjustmentInvalid, 0, PlacementInvalid, 0, 0, 0, 0, false
	}
	return item.route, item.adjustment, item.result, item.placement, item.offset, item.outcome, item.subedge, item.destination, true
}

// CallbackLifecycle returns the complete sealed callback lifecycle relation.
func (c *Contract) CallbackLifecycle(id CallbackID) (CallbackLifecycle, bool) {
	row, ok := c.callback(id)
	return row.lifecycle, ok
}

// CallbackEffectCount returns the finite explicit occurrences in the
// callback's expected Koka row.
func (c *Contract) CallbackEffectCount(id CallbackID) int {
	row, ok := c.callback(id)
	if !ok {
		return 0
	}
	return row.effects.len()
}

func (c *Contract) CallbackEffectTarget(id CallbackID, index int) (Operation, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0, false
	}
	return effect.target, true
}

func (c *Contract) CallbackEffectValueArgumentCount(id CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.values.len()
}

func (c *Contract) CallbackEffectValueArgumentAt(id CallbackID, index, argument int) (ValueFormal, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.values.len() {
		return 0, false
	}
	return c.effectVals[effect.values.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectTypeArgumentCount(id CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.types.len()
}

func (c *Contract) CallbackEffectTypeArgumentAt(id CallbackID, index, argument int) (TypeFormal, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.types.len() {
		return 0, false
	}
	return c.effectType[effect.types.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectValuesArgumentCount(id CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.valuesVar.len()
}

func (c *Contract) CallbackEffectValuesArgumentAt(id CallbackID, index, argument int) (ValuesVar, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.valuesVar.len() {
		return 0, false
	}
	return c.effectVars[effect.valuesVar.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectRowArgumentCount(id CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.rows.len()
}

func (c *Contract) CallbackEffectRowArgumentAt(id CallbackID, index, argument int) (RowVar, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.rows.len() {
		return 0, false
	}
	return c.effectRows[effect.rows.start+uint32(argument)], true
}

// CallbackEffectTail returns the callback's expected row tail.
func (c *Contract) CallbackEffectTail(id CallbackID) (RowTail, RowVar, bool) {
	row, ok := c.callback(id)
	if !ok {
		return 0, 0, false
	}
	return row.effectTail, row.effectVar, true
}

func (c *Contract) callbackEffect(id CallbackID, index int) (effectRow, bool) {
	row, ok := c.callback(id)
	if !ok || index < 0 || index >= row.effects.len() {
		return effectRow{}, false
	}
	return c.effects[row.effects.start+uint32(index)], true
}

// CallbackRelease reports the optional explicit causal release of a retained
// callback. The release operation owns the reverse range exposed below.
func (c *Contract) CallbackRelease(id CallbackID) (Operation, ValueFormal, uint32, CallbackReleaseMode, bool) {
	row, ok := c.callback(id)
	if !ok || row.release == 0 || uint64(row.release) > uint64(len(c.callbackReleases)) {
		return 0, 0, 0, CallbackReleaseInvalid, false
	}
	release := c.callbackReleases[row.release-1]
	if release.callback != id {
		return 0, 0, 0, CallbackReleaseInvalid, false
	}
	return release.operation, release.input, release.outcome, release.mode, true
}

// CallbackReleaseZero reports the required zero-holder arm of an explicit
// retained callback release. The outcome is meaningful only for Throw and
// Idempotent; Suppress returns zero and creates no terminal successor.
func (c *Contract) CallbackReleaseZero(id CallbackID) (CallbackReleaseZeroBehavior, uint32, bool) {
	row, ok := c.callback(id)
	if !ok || row.release == 0 || uint64(row.release) > uint64(len(c.callbackReleases)) {
		return CallbackReleaseZeroInvalid, 0, false
	}
	release := c.callbackReleases[row.release-1]
	if release.callback != id || !validCallbackReleaseZeroBehavior(release.zeroBehavior) {
		return CallbackReleaseZeroInvalid, 0, false
	}
	return release.zeroBehavior, release.zeroOutcome, true
}

// CallbackReleaseCount returns releases caused by one source-visible operation.
func (c *Contract) CallbackReleaseCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.releases.len()
}

// CallbackReleaseAt returns one release in the operation's dense direct range.
func (c *Contract) CallbackReleaseAt(op Operation, index int) (CallbackID, ValueFormal, uint32, CallbackReleaseMode, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.releases.len() {
		return 0, 0, 0, CallbackReleaseInvalid, false
	}
	release := c.callbackReleases[row.releases.start+uint32(index)]
	if release.operation != op {
		return 0, 0, 0, CallbackReleaseInvalid, false
	}
	return release.callback, release.input, release.outcome, release.mode, true
}

// SuspensionCount reports exact authored suspension relations. The one opaque
// Operation derives its three maximal provider reentries from Contract.Opaque;
// no duplicate opaque flag or authored fallback row exists.
