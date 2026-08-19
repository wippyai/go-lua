package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func (c *Contract) callback(id vocabulary.CallbackID) (callbackRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.callbacks)) {
		return callbackRow{}, false
	}
	return c.callbacks[uint32(id)-1], true
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
	owner, ok := c.Operations.CallbackOwner(id)
	opaque, opaqueOK := c.Operations.Opaque()
	return ok && opaqueOK && owner == opaque
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
