package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func (c *Contract) SuspensionCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	if op == c.opaque {
		return 3
	}
	return row.suspensions.len()
}

// SuspensionAt returns an operation-owned relation. For opaque, index 0..2
// derives Yield → Normal/Throw/Cancel in that canonical outcome order; opaque
// Values remain the Contract's existing unknown Values relation.
func (c *Contract) SuspensionAt(op Operation, index int) (yield, reentry uint32, source ReentrySource, multiplicity ReentryMultiplicity, ok bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 {
		return 0, 0, 0, 0, false
	}
	if op == c.opaque {
		if index >= 3 {
			return 0, 0, 0, 0, false
		}
		reentry := uint32(index)
		if index == 2 {
			reentry = 3
		}
		return 2, reentry, ReentryByProvider, ReentryMany, true
	}
	if index >= row.suspensions.len() {
		return 0, 0, 0, 0, false
	}
	value := c.suspensions[row.suspensions.start+uint32(index)]
	return value.yield, value.reentry, value.source, value.multiplicity, true
}

// SpawnCount reports the finite detached-spawn authorities owned by op. A
// sealed contract admits at most one such authority globally.
func (c *Contract) SpawnCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok || op == c.opaque {
		return 0
	}
	return row.spawns.len()
}

// SpawnIDAt returns the sealed identity of an operation-owned spawn relation.
func (c *Contract) SpawnIDAt(op Operation, index int) (SpawnID, bool) {
	row, ok := c.operation(op)
	if !ok || op == c.opaque || index < 0 || index >= row.spawns.len() {
		return 0, false
	}
	return SpawnID(row.spawns.start + uint32(index) + 1), true
}

func (c *Contract) spawn(id SpawnID) (spawnRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.spawns)) {
		return spawnRow{}, false
	}
	return c.spawns[uint32(id)-1], true
}

// Spawn exposes the one typed detached application correspondence. Function
// is the exact parent input authority and Child is its existing callback
// activation relation. ParentYield/ParentResume are canonical owner outcome
// ordinals. ChildEntry and ResumeValues are existing closed empty Packs.
func (c *Contract) Spawn(id SpawnID) (owner Operation, function InputSource, child CallbackID, parentYield, parentResume uint32, childEntry, resumeValues Values, ok bool) {
	row, ok := c.spawn(id)
	if !ok {
		return 0, InputSource{}, 0, 0, 0, 0, 0, false
	}
	return row.owner, row.function, row.child, row.yield, row.parentResume, row.childEntry, row.resumeValues, true
}

// SpawnSiblingCount is always two for a valid spawn relation: both concrete
// enabled-event orders are explicit and neither is inferred by a scheduler.
func (c *Contract) SpawnSiblingCount(id SpawnID) int {
	if _, ok := c.spawn(id); !ok {
		return 0
	}
	return 2
}

// SpawnSiblingAt returns one concrete legal sibling ordering.
func (c *Contract) SpawnSiblingAt(id SpawnID, index int) (SpawnSiblingAlternative, bool) {
	if index < 0 || index >= 2 {
		return SpawnSiblingInvalid, false
	}
	row, ok := c.spawn(id)
	if !ok {
		return SpawnSiblingInvalid, false
	}
	return row.alternatives[index], true
}

func (c *Contract) ResumeCount(op Operation) int {
	row, ok := c.operation(op)
	if !ok || op == c.opaque {
		return 0
	}
	return row.resumes.len()
}

// ResumeIDAt returns the sealed identity for one exact operation-local resume
// correspondence. The authored ordinal is consumed at this boundary and must
// not be retained as a Link or runtime identity.
func (c *Contract) ResumeIDAt(op Operation, index int) (ResumeID, bool) {
	row, ok := c.operation(op)
	if !ok || op == c.opaque || index < 0 || index >= row.resumes.len() {
		return 0, false
	}
	return ResumeID(row.resumes.start + uint32(index) + 1), true
}

func (c *Contract) resume(id ResumeID) (resumeRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.resumes)) {
		return resumeRow{}, false
	}
	return c.resumes[uint32(id)-1], true
}

// Resume returns the owning operation and exact activation operand declaration
// for a sealed resumption correspondence. A Produced source has no
// ValueFormal carrier; its carrier is the existing Produced result origin and
// any retained CallbackID is queried through ProducedCaptureAt. Arguments is
// the existing operation Values coordinate supplied to the restored activation.
func (c *Contract) Resume(id ResumeID) (owner Operation, source ResumeSource, carrier ValueFormal, arguments Values, ok bool) {
	row, ok := c.resume(id)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.owner, row.source, row.carrier, row.arguments, true
}

// ResumeOutcomeCount is always five for a valid resume: Normal, Return,
// Throw, Yield, and Cancel in that canonical Kind order.
func (c *Contract) ResumeOutcomeCount(resume ResumeID) int {
	if _, ok := c.resume(resume); !ok {
		return 0
	}
	return 5
}

// ResumeOutcomeAt returns one canonical cross-activation outcome mapping.
// Outcome is the canonical ordinal of the owning operation outcome.
func (c *Contract) ResumeOutcomeAt(resume ResumeID, index int) (kind flowkind.OutcomeKind, outcome uint32, ok bool) {
	if index < 0 || index >= 5 {
		return 0, 0, false
	}
	value, found := c.resume(resume)
	if !found {
		return 0, 0, false
	}
	return [...]flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel}[index], value.outcomes[index], true
}

func (c *Contract) CallbackResultCount(op Operation, outcome int) int {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return 0
	}
	return c.outcomes[row.outcomes.start+uint32(outcome)].callbackResults.len()
}

func (c *Contract) callbackResultAt(op Operation, outcome, index int) (callbackResultRow, bool) {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return callbackResultRow{}, false
	}
	results := c.outcomes[row.outcomes.start+uint32(outcome)].callbackResults
	if index < 0 || index >= results.len() {
		return callbackResultRow{}, false
	}
	return c.callbackResults[results.start+uint32(index)], true
}

func (c *Contract) CallbackResultAt(op Operation, outcome, index int) (uint32, CallbackID, bool) {
	row, ok := c.callbackResultAt(op, outcome, index)
	if !ok {
		return 0, 0, false
	}
	return row.result, row.callback, true
}

func (c *Contract) CallbackForResult(op Operation, outcome int, result uint32) (CallbackID, int, bool) {
	count := c.CallbackResultCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, ok := c.CallbackResultAt(op, outcome, mid)
		if !ok {
			return 0, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, 0, false
	}
	current, callback, ok := c.CallbackResultAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, false
	}
	return callback, left, true
}

// ResultAliasCount returns the static result-to-input correspondences owned
// by one outcome.
func (c *Contract) ResultAliasCount(op Operation, outcome int) int {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return 0
	}
	return c.outcomes[row.outcomes.start+uint32(outcome)].resultAliases.len()
}

func (c *Contract) resultAliasAt(op Operation, outcome, index int) (resultAliasRow, bool) {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return resultAliasRow{}, false
	}
	aliases := c.outcomes[row.outcomes.start+uint32(outcome)].resultAliases
	if index < 0 || index >= aliases.len() {
		return resultAliasRow{}, false
	}
	return c.resultAliases[aliases.start+uint32(index)], true
}

// ResultAliasAt returns one canonical result-to-ValueFormal correspondence.
func (c *Contract) ResultAliasAt(op Operation, outcome, index int) (uint32, InputSourceKind, uint32, bool) {
	row, ok := c.resultAliasAt(op, outcome, index)
	if !ok {
		return 0, 0, 0, false
	}
	return row.result, row.source.Kind, row.source.Ordinal, true
}

// ResultAliasForResult finds the correspondence for one fixed result slot.
func (c *Contract) ResultAliasForResult(op Operation, outcome int, result uint32) (InputSourceKind, uint32, int, bool) {
	count := c.ResultAliasCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, _, ok := c.ResultAliasAt(op, outcome, mid)
		if !ok {
			return 0, 0, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, 0, 0, false
	}
	current, kind, source, ok := c.ResultAliasAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, 0, false
	}
	return kind, source, left, true
}

func (c *Contract) ProducedCount(op Operation, outcome int) int {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return 0
	}
	return c.outcomes[row.outcomes.start+uint32(outcome)].produced.len()
}

func (c *Contract) producedAt(op Operation, outcome, index int) (producedRow, bool) {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return producedRow{}, false
	}
	produced := c.outcomes[row.outcomes.start+uint32(outcome)].produced
	if index < 0 || index >= produced.len() {
		return producedRow{}, false
	}
	return c.produced[produced.start+uint32(index)], true
}

func (c *Contract) ProducedAt(op Operation, outcome, index int) (uint32, Operation, bool) {
	row, ok := c.producedAt(op, outcome, index)
	if !ok {
		return 0, 0, false
	}
	return row.result, row.target, true
}

func (c *Contract) ProducedForResult(op Operation, outcome int, result uint32) (Operation, int, bool) {
	count := c.ProducedCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, ok := c.ProducedAt(op, outcome, mid)
		if !ok {
			return 0, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, 0, false
	}
	current, target, ok := c.ProducedAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, false
	}
	return target, left, true
}

// FreshResultCount is the number of nominal result roots owned by one
// canonical outcome.
func (c *Contract) FreshResultCount(op Operation, outcome int) int {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return 0
	}
	return c.outcomes[row.outcomes.start+uint32(outcome)].fresh.len()
}

func (c *Contract) freshResultAt(op Operation, outcome, index int) (freshResultRow, bool) {
	row, ok := c.operation(op)
	if !ok || outcome < 0 || outcome >= row.outcomes.len() {
		return freshResultRow{}, false
	}
	fresh := c.outcomes[row.outcomes.start+uint32(outcome)].fresh
	if index < 0 || index >= fresh.len() {
		return freshResultRow{}, false
	}
	return c.fresh[fresh.start+uint32(index)], true
}

// FreshResultAt returns one canonical fixed-result freshness relation. The
// ordinal is dense within this outcome and independent of authoring order.
func (c *Contract) FreshResultAt(op Operation, outcome, index int) (result, ordinal uint32, kind schematype.FreshClass, ok bool) {
	row, found := c.freshResultAt(op, outcome, index)
	if !found {
		return 0, 0, schematype.FreshClassInvalid, false
	}
	return row.result, row.ordinal, row.kind, true
}

// FreshResultForResult finds the nominal freshness relation for one fixed
// outcome result. The returned index is the canonical FreshResultAt index.
func (c *Contract) FreshResultForResult(op Operation, outcome int, result uint32) (ordinal uint32, kind schematype.FreshClass, index int, ok bool) {
	count := c.FreshResultCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, _, found := c.FreshResultAt(op, outcome, mid)
		if !found {
			return 0, schematype.FreshClassInvalid, 0, false
		}
		if current < result {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if left >= count {
		return 0, schematype.FreshClassInvalid, 0, false
	}
	current, ordinal, kind, found := c.FreshResultAt(op, outcome, left)
	if !found || current != result {
		return 0, schematype.FreshClassInvalid, 0, false
	}
	return ordinal, kind, left, true
}

func (c *Contract) ProducedCaptureCount(op Operation, outcome, produced int) int {
	row, ok := c.producedAt(op, outcome, produced)
	if !ok {
		return 0
	}
	return row.captures.len()
}

func (c *Contract) ProducedCaptureAt(op Operation, outcome, produced, capture int) (CaptureKind, uint32, bool) {
	row, ok := c.producedAt(op, outcome, produced)
	if !ok || capture < 0 || capture >= row.captures.len() {
		return 0, 0, false
	}
	value := c.captures[row.captures.start+uint32(capture)]
	return value.kind, value.ordinal, true
}

// ProducedTypeValueCapture returns the sole fixed input formal whose runtime
// TypeValue is retained by one Produced row. The relation is indexed when the
// contract seals; it neither scans captures nor infers TypeValue identity
// from the value/runtime-type spelling.
func (c *Contract) ProducedTypeValueCapture(op Operation, outcome, produced int) (ValueFormal, bool) {
	row, ok := c.producedAt(op, outcome, produced)
	if !ok || row.typeValueCapture == noTypeValueCapture || row.typeValueCapture >= uint32(row.captures.len()) {
		return 0, false
	}
	capture := c.captures[row.captures.start+row.typeValueCapture]
	if capture.kind != CaptureTypeValueFormal {
		return 0, false
	}
	return ValueFormal(capture.ordinal), true
}

// TypeDeclaration returns the complete neutral declaration retained by the
// sealed target type row. It preserves primitive atoms and formal-scope
// framing; domain consumers pass this value to their explicit
// schema/typecontract semantic adapter.
func (c *Contract) TypeDeclaration(typ Type) (schematype.Type, bool) {
	if c == nil || typ == 0 || uint64(typ) > uint64(len(c.types)) {
		return schematype.Type{}, false
	}
	return c.types[uint32(typ)-1].declaration, true
}

func (r indexRange) len() int { return int(r.end - r.start) }
