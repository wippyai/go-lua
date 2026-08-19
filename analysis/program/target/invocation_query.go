package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func (c *Contract) CallbackCount(op vocabulary.Operation) int {
	if c == nil {
		return 0
	}
	return c.operationCore.CallbackCount(op)
}

func (c *Contract) CallbackAt(op vocabulary.Operation, index int) (vocabulary.CallbackID, bool) {
	if c == nil {
		return 0, false
	}
	return c.operationCore.CallbackAt(op, index)
}

func (c *Contract) callback(id vocabulary.CallbackID) (callbackRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.callbacks)) {
		return callbackRow{}, false
	}
	return c.callbacks[uint32(id)-1], true
}

// CallbackOwner returns the exact sealed operation that owns a callback
// correspondence. The range validation keeps a malformed callback row from
// being accepted merely because its stored owner is an otherwise valid
// operation.
func (c *Contract) CallbackOwner(id vocabulary.CallbackID) (vocabulary.Operation, bool) {
	if c == nil {
		return 0, false
	}
	return c.operationCore.CallbackOwner(id)
}

// callbackFunction returns the exact input authority that supplies a callback
// function. Authored callbacks use ValueFormal; the opaque callback uses the
// sole maximal AllInputs authority.
func (c *Contract) callbackFunction(id vocabulary.CallbackID) (vocabulary.InputSource, bool) {
	row, ok := c.callback(id)
	if !ok {
		return vocabulary.InputSource{}, false
	}
	return row.function, true
}

// CallbackArguments returns the full Values schema at the callback argument
// role. Equal Values handles across roles are structural deduplication only,
// never a flow claim.
func (c *Contract) CallbackArguments(id vocabulary.CallbackID) (vocabulary.Values, bool) {
	row, ok := c.callback(id)
	return row.arguments, ok
}

// CallbackOutcome returns the exact Values relation carried by one callback
// activation outcome. It prescribes no provider response to that outcome.
func (c *Contract) CallbackOutcome(id vocabulary.CallbackID, kind flowkind.OutcomeKind) (vocabulary.Values, bool) {
	index, valid := vocabulary.CrossActivationOutcomeIndex(kind)
	if !valid {
		return 0, false
	}
	row, ok := c.callback(id)
	if !ok {
		return 0, false
	}
	return row.outcomes[index], true
}

// callbackAdmission returns the callback's sole callable convention. A
// callback-backed Subedge projects this value; it never carries a duplicate.
func (c *Contract) callbackAdmission(id vocabulary.CallbackID) (schematype.CallableAdmission, bool) {
	row, ok := c.callback(id)
	return row.admission, ok
}

// callbackOpaque exposes the one explicit maximally conservative callback
// owned by the synthesized opaque operation. Missing authored Subedge rows do
// not imply a callback is closed, successful, or non-reentrant.
func (c *Contract) callbackOpaque(id vocabulary.CallbackID) bool {
	owner, ok := c.CallbackOwner(id)
	opaque, opaqueOK := c.Opaque()
	return ok && opaqueOK && owner == opaque
}

// callbackSubedge returns the sole immediate typed application of callback.
// It is a derived sealed reverse index over SubedgeCallback, not a second
// execution relation or semantic identity coordinate. Retained and opaque
// callbacks intentionally have no immediate Subedge.
func (c *Contract) callbackSubedge(id vocabulary.CallbackID) (vocabulary.SubedgeID, bool) {
	callback, ok := c.callback(id)
	if !ok || callback.subedge == 0 {
		return 0, false
	}
	edge, ok := c.subedge(callback.subedge)
	if !ok || edge.callee != vocabulary.SubedgeCalleeCallback || edge.callback != id {
		return 0, false
	}
	return callback.subedge, true
}

// SubedgeCount returns every sealed typed internal application owned by op.
func (c *Contract) SubedgeCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.subedges.len()
}

func (c *Contract) SubedgeAt(op vocabulary.Operation, index int) (vocabulary.SubedgeID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.subedges.len() {
		return 0, false
	}
	subedges := row.subedges
	return vocabulary.SubedgeID(subedges.start + uint32(index) + 1), true
}

func (c *Contract) subedge(id vocabulary.SubedgeID) (subedgeRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.subedges)) {
		return subedgeRow{}, false
	}
	return c.subedges[id-1], true
}

// subedgeOwner is the only Target ownership projection needed by Link. It
// deliberately does not create a Candidate, Application, or Program Term.
func (c *Contract) subedgeOwner(id vocabulary.SubedgeID) (vocabulary.Operation, bool) {
	row, ok := c.subedge(id)
	if !ok || row.owner == 0 {
		return 0, false
	}
	ownerRow, ok := c.operation(row.owner)
	if !ok {
		return 0, false
	}
	index := uint32(id) - 1
	subedges := ownerRow.subedges
	if index < subedges.start || index >= subedges.end {
		return 0, false
	}
	return row.owner, true
}

func (c *Contract) subedgeRole(id vocabulary.SubedgeID) (uint32, bool) {
	row, ok := c.subedge(id)
	return row.role, ok
}

func (c *Contract) SubedgeFamily(id vocabulary.SubedgeID) (vocabulary.SubedgeFamily, bool) {
	row, ok := c.subedge(id)
	return row.family, ok
}

func (c *Contract) subedgeCallee(id vocabulary.SubedgeID) (vocabulary.SubedgeCalleeKind, bool) {
	row, ok := c.subedge(id)
	return row.callee, ok
}

func (c *Contract) subedgeAdmission(id vocabulary.SubedgeID) (schematype.CallableAdmission, bool) {
	row, ok := c.subedge(id)
	return row.admission, ok
}

func (c *Contract) subedgeCallback(id vocabulary.SubedgeID) (vocabulary.CallbackID, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != vocabulary.SubedgeCalleeCallback || row.callback == 0 {
		return 0, false
	}
	return row.callback, true
}

// subedgeCapturedInitialRead reports the capture-once boot source owned by
// this edge. The SubedgeID itself is its operation-local capture identity.
func (c *Contract) subedgeCapturedInitialRead(id vocabulary.SubedgeID) (vocabulary.InitialRoot, vocabulary.ExactKey, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != vocabulary.SubedgeCalleeCapturedInitialRead || row.readRoot == 0 || row.readKey == 0 {
		return 0, 0, false
	}
	return row.readRoot, row.readKey, true
}

func (c *Contract) subedgeMetaKey(id vocabulary.SubedgeID) (vocabulary.ExactKey, bool) {
	row, ok := c.subedge(id)
	if !ok || row.callee != vocabulary.SubedgeCalleeMetaKey || row.metaKey == 0 {
		return 0, false
	}
	return row.metaKey, true
}

// SubedgeArguments returns the contextual callee-argument Values endpoint.
// Equal Values handles in another role are never implicit dataflow.
func (c *Contract) SubedgeArguments(id vocabulary.SubedgeID) (vocabulary.Values, bool) {
	row, ok := c.subedge(id)
	return row.arguments, ok
}

// subedgeRuleEntry reports whether an argument-free Subedge has explicit
// owner-Rule entry authority. Nonempty direct arguments use ArgumentOrigins.
func (c *Contract) subedgeRuleEntry(id vocabulary.SubedgeID) (bool, bool) {
	row, ok := c.subedge(id)
	return row.ruleEntry, ok
}

// argumentOriginCount reports the authored complete source set for this
// contextual argument endpoint. A zero count is either the explicit nullary
// Rule entry reported by SubedgeRuleEntry or a complete sibling/admission
// route; it never implies an entry by itself.
func (c *Contract) argumentOriginCount(id vocabulary.SubedgeID) int {
	row, ok := c.subedge(id)
	if !ok {
		return 0
	}
	return row.argumentOrigins.len()
}

// ArgumentOriginAt returns one direct owner-input or owner-Rule source
// for a single argument Values segment. The source is zero for Rule entries.
func (c *Contract) ArgumentOriginAt(id vocabulary.SubedgeID, index int) (segment vocabulary.ArgumentSegment, ordinal uint32, source vocabulary.ArgumentSource, input vocabulary.InputSource, ok bool) {
	row, found := c.subedge(id)
	if !found || index < 0 || index >= row.argumentOrigins.len() {
		return vocabulary.ArgumentSegmentInvalid, 0, vocabulary.ArgumentSourceInvalid, vocabulary.InputSource{}, false
	}
	item := c.subedgeOrigins[row.argumentOrigins.start+uint32(index)]
	return item.segment, item.index, item.kind, item.source, true
}

func (c *Contract) SubedgeTerminal(id vocabulary.SubedgeID, kind flowkind.OutcomeKind) (vocabulary.Values, bool) {
	index, valid := vocabulary.CrossActivationOutcomeIndex(kind)
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
func (c *Contract) AdmissionFailure(id vocabulary.SubedgeID) (vocabulary.Values, bool) {
	row, ok := c.subedge(id)
	if !ok || row.admissionFailure == 0 {
		return 0, false
	}
	return row.admissionFailure, true
}

// admissionRoute returns the one explicit transport of a callable
// admission failure. Only Outcome and Subedge routes are representable.
func (c *Contract) admissionRoute(id vocabulary.SubedgeID) (route vocabulary.SubedgeRoute, adjustment vocabulary.Adjustment, result vocabulary.Values, placement vocabulary.Placement, offset uint32, outcome uint32, sibling vocabulary.SubedgeID, destination vocabulary.Values, ok bool) {
	row, found := c.subedge(id)
	if !found || row.admissionRoute.route == vocabulary.RouteInvalid {
		return vocabulary.RouteInvalid, vocabulary.AdjustmentInvalid, 0, vocabulary.PlacementInvalid, 0, 0, 0, 0, false
	}
	item := row.admissionRoute
	return item.route, item.adjustment, item.result, item.placement, item.offset, item.outcome, item.subedge, item.destination, true
}

// subedgeRouteAt returns the contextual projected Result and its one route.
// RejectYield's Result is the canonical C-boundary error Values, not the
// discarded child Yield payload; it may target an owner Throw or sibling edge.
func (c *Contract) subedgeRouteAt(id vocabulary.SubedgeID, kind flowkind.OutcomeKind) (route vocabulary.SubedgeRoute, adjustment vocabulary.Adjustment, result vocabulary.Values, placement vocabulary.Placement, offset uint32, outcome uint32, sibling vocabulary.SubedgeID, destination vocabulary.Values, ok bool) {
	index, valid := vocabulary.CrossActivationOutcomeIndex(kind)
	if !valid {
		return vocabulary.RouteInvalid, vocabulary.AdjustmentInvalid, 0, vocabulary.PlacementInvalid, 0, 0, 0, 0, false
	}
	row, found := c.subedge(id)
	if !found {
		return vocabulary.RouteInvalid, vocabulary.AdjustmentInvalid, 0, vocabulary.PlacementInvalid, 0, 0, 0, 0, false
	}
	item := row.routes[index]
	if item.route == vocabulary.RouteInvalid {
		return vocabulary.RouteInvalid, vocabulary.AdjustmentInvalid, 0, vocabulary.PlacementInvalid, 0, 0, 0, 0, false
	}
	return item.route, item.adjustment, item.result, item.placement, item.offset, item.outcome, item.subedge, item.destination, true
}

// CallbackLifecycle returns the complete sealed callback lifecycle relation.
func (c *Contract) CallbackLifecycle(id vocabulary.CallbackID) (vocabulary.CallbackLifecycle, bool) {
	if c == nil {
		return 0, false
	}
	return c.operationCore.CallbackLifecycle(id)
}

// CallbackEffectCount returns the finite explicit occurrences in the
// callback's expected Koka row.
func (c *Contract) CallbackEffectCount(id vocabulary.CallbackID) int {
	row, ok := c.callback(id)
	if !ok {
		return 0
	}
	return row.effects.len()
}

func (c *Contract) CallbackEffectTarget(id vocabulary.CallbackID, index int) (vocabulary.Operation, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0, false
	}
	return effect.target, true
}

func (c *Contract) CallbackEffectValueArgumentCount(id vocabulary.CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.values.len()
}

func (c *Contract) CallbackEffectValueArgumentAt(id vocabulary.CallbackID, index, argument int) (vocabulary.ValueFormal, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.values.len() {
		return 0, false
	}
	return c.effectVals[effect.values.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectTypeArgumentCount(id vocabulary.CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.types.len()
}

func (c *Contract) CallbackEffectTypeArgumentAt(id vocabulary.CallbackID, index, argument int) (vocabulary.TypeFormal, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.types.len() {
		return 0, false
	}
	return c.effectType[effect.types.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectValuesArgumentCount(id vocabulary.CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.valuesVar.len()
}

func (c *Contract) CallbackEffectValuesArgumentAt(id vocabulary.CallbackID, index, argument int) (vocabulary.ValuesVar, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.valuesVar.len() {
		return 0, false
	}
	return c.effectVars[effect.valuesVar.start+uint32(argument)], true
}

func (c *Contract) CallbackEffectRowArgumentCount(id vocabulary.CallbackID, index int) int {
	effect, ok := c.callbackEffect(id, index)
	if !ok {
		return 0
	}
	return effect.rows.len()
}

func (c *Contract) callbackEffectRowArgumentAt(id vocabulary.CallbackID, index, argument int) (vocabulary.RowVar, bool) {
	effect, ok := c.callbackEffect(id, index)
	if !ok || argument < 0 || argument >= effect.rows.len() {
		return 0, false
	}
	return c.effectRows[effect.rows.start+uint32(argument)], true
}

// CallbackEffectTail returns the callback's expected row tail.
func (c *Contract) CallbackEffectTail(id vocabulary.CallbackID) (vocabulary.RowTail, vocabulary.RowVar, bool) {
	row, ok := c.callback(id)
	if !ok {
		return 0, 0, false
	}
	return row.effectTail, row.effectVar, true
}

func (c *Contract) callbackEffect(id vocabulary.CallbackID, index int) (effectRow, bool) {
	row, ok := c.callback(id)
	if !ok || index < 0 || index >= row.effects.len() {
		return effectRow{}, false
	}
	return c.effects[row.effects.start+uint32(index)], true
}

// callbackRelease reports the optional explicit causal release of a retained
// callback. The release operation owns the reverse range exposed below.
func (c *Contract) callbackRelease(id vocabulary.CallbackID) (vocabulary.Operation, vocabulary.ValueFormal, uint32, vocabulary.CallbackReleaseMode, bool) {
	row, ok := c.callback(id)
	if !ok || row.release == 0 || uint64(row.release) > uint64(len(c.callbackReleases)) {
		return 0, 0, 0, vocabulary.CallbackReleaseInvalid, false
	}
	release := c.callbackReleases[row.release-1]
	if release.callback != id {
		return 0, 0, 0, vocabulary.CallbackReleaseInvalid, false
	}
	return release.operation, release.input, release.outcome, release.mode, true
}

// callbackReleaseZero reports the required zero-holder arm of an explicit
// retained callback release. The outcome is meaningful only for Throw and
// Idempotent; Suppress returns zero and creates no terminal successor.
func (c *Contract) callbackReleaseZero(id vocabulary.CallbackID) (vocabulary.CallbackReleaseZeroBehavior, uint32, bool) {
	row, ok := c.callback(id)
	if !ok || row.release == 0 || uint64(row.release) > uint64(len(c.callbackReleases)) {
		return vocabulary.CallbackReleaseZeroInvalid, 0, false
	}
	release := c.callbackReleases[row.release-1]
	if release.callback != id || !vocabulary.ValidCallbackReleaseZeroBehavior(release.zeroBehavior) {
		return vocabulary.CallbackReleaseZeroInvalid, 0, false
	}
	return release.zeroBehavior, release.zeroOutcome, true
}

// callbackReleaseCount returns releases caused by one source-visible operation.
func (c *Contract) callbackReleaseCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	return row.releases.len()
}

// callbackReleaseAt returns one release in the operation's dense direct range.
func (c *Contract) callbackReleaseAt(op vocabulary.Operation, index int) (vocabulary.CallbackID, vocabulary.ValueFormal, uint32, vocabulary.CallbackReleaseMode, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.releases.len() {
		return 0, 0, 0, vocabulary.CallbackReleaseInvalid, false
	}
	releases := row.releases
	release := c.callbackReleases[releases.start+uint32(index)]
	if release.operation != op {
		return 0, 0, 0, vocabulary.CallbackReleaseInvalid, false
	}
	return release.callback, release.input, release.outcome, release.mode, true
}

// SuspensionCount reports exact authored suspension relations. The one opaque
// Operation derives its three maximal provider reentries from Contract.Opaque;
// no duplicate opaque flag or authored fallback row exists.

// CallbackPublicationEffectDescriptor returns the immutable Target-owned
// publication semantics for one exact callback effect occurrence.
func (c *Contract) CallbackPublicationEffectDescriptor(callback vocabulary.CallbackID, index int) (PublicationEffectDescriptor, bool) {
	row, ok := c.callback(callback)
	if !ok || !c.sealed || index < 0 || index >= row.effects.len() {
		return PublicationEffectDescriptor{}, false
	}
	effect := c.effects[row.effects.start+uint32(index)]
	if !c.validPublicationEffectRow(effect) {
		return PublicationEffectDescriptor{}, false
	}
	return effect.publication, true
}
