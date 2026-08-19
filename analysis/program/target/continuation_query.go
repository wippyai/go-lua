package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// opaqueSuspensionCount is the number of suspensions the reader derives for the
// opaque operation: Yield reentering Normal, Throw and Cancel. They are derived
// rather than stored, so no suspension row backs them.
const opaqueSuspensionCount = 3

func (c *Contract) suspensionCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok {
		return 0
	}
	if c.opaqueOperation(op) {
		return opaqueSuspensionCount
	}
	return row.suspensions.len()
}

// suspensionAt returns an operation-owned relation. For opaque, index 0..2
// derives Yield → Normal/Throw/Cancel in that canonical outcome order; opaque
// Values remain the Contract's existing unknown Values relation.
func (c *Contract) suspensionAt(op vocabulary.Operation, index int) (yield, reentry uint32, source vocabulary.ReentrySource, multiplicity vocabulary.ReentryMultiplicity, ok bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 {
		return 0, 0, 0, 0, false
	}
	if c.opaqueOperation(op) {
		if index >= opaqueSuspensionCount {
			return 0, 0, 0, 0, false
		}
		reentry := uint32(index)
		if index == 2 {
			reentry = 3
		}
		return 2, reentry, vocabulary.ReentryByProvider, vocabulary.ReentryMany, true
	}
	suspensions := row.suspensions
	if index >= suspensions.len() {
		return 0, 0, 0, 0, false
	}
	value := c.suspensions[suspensions.start+uint32(index)]
	return value.yield, value.reentry, value.source, value.multiplicity, true
}

// spawnCount reports the finite detached-spawn authorities owned by op. A
// sealed contract admits at most one such authority globally.
func (c *Contract) spawnCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok || c.opaqueOperation(op) {
		return 0
	}
	return row.spawns.len()
}

// spawnIDAt returns the sealed identity of an operation-owned spawn relation.
func (c *Contract) spawnIDAt(op vocabulary.Operation, index int) (vocabulary.SpawnID, bool) {
	row, ok := c.operation(op)
	if !ok || c.opaqueOperation(op) || index < 0 || index >= row.spawns.len() {
		return 0, false
	}
	spawns := row.spawns
	return vocabulary.SpawnID(spawns.start + uint32(index) + 1), true
}

func (c *Contract) spawn(id vocabulary.SpawnID) (spawnRow, bool) {
	if c == nil || id == 0 || uint64(id) > uint64(len(c.spawns)) {
		return spawnRow{}, false
	}
	return c.spawns[uint32(id)-1], true
}

// spawnRelation exposes the one typed detached application correspondence. Function
// is the exact parent input authority and Child is its existing callback
// activation relation. ParentYield/ParentResume are canonical owner outcome
// ordinals. ChildEntry and ResumeValues are existing closed empty Packs.
func (c *Contract) spawnRelation(id vocabulary.SpawnID) (owner vocabulary.Operation, function vocabulary.InputSource, child vocabulary.CallbackID, parentYield, parentResume uint32, childEntry, resumeValues vocabulary.Values, ok bool) {
	row, ok := c.spawn(id)
	if !ok {
		return 0, vocabulary.InputSource{}, 0, 0, 0, 0, 0, false
	}
	return row.owner, row.function, row.child, row.yield, row.parentResume, row.childEntry, row.resumeValues, true
}

// spawnSiblingCount is always two for a valid spawn relation: both concrete
// enabled-event orders are explicit and neither is inferred by a scheduler.
func (c *Contract) spawnSiblingCount(id vocabulary.SpawnID) int {
	if _, ok := c.spawn(id); !ok {
		return 0
	}
	return spawnSiblingAlternatives
}

// spawnSiblingAt returns one concrete legal sibling ordering.
func (c *Contract) spawnSiblingAt(id vocabulary.SpawnID, index int) (vocabulary.SpawnSiblingAlternative, bool) {
	if index < 0 || index >= spawnSiblingAlternatives {
		return vocabulary.SpawnSiblingInvalid, false
	}
	row, ok := c.spawn(id)
	if !ok {
		return vocabulary.SpawnSiblingInvalid, false
	}
	return row.alternatives[index], true
}

func (c *Contract) ResumeCount(op vocabulary.Operation) int {
	row, ok := c.operation(op)
	if !ok || c.opaqueOperation(op) {
		return 0
	}
	return row.resumes.len()
}

// ResumeIDAt returns the sealed identity for one exact operation-local resume
// correspondence. The authored ordinal is consumed at this boundary and must
// not be retained as a Link or runtime identity.
func (c *Contract) ResumeIDAt(op vocabulary.Operation, index int) (vocabulary.ResumeID, bool) {
	row, ok := c.operation(op)
	if !ok || c.opaqueOperation(op) || index < 0 || index >= row.resumes.len() {
		return 0, false
	}
	resumes := row.resumes
	return vocabulary.ResumeID(resumes.start + uint32(index) + 1), true
}

func (c *Contract) resume(id vocabulary.ResumeID) (resumeRow, bool) {
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
func (c *Contract) Resume(id vocabulary.ResumeID) (owner vocabulary.Operation, source vocabulary.ResumeSource, carrier vocabulary.ValueFormal, arguments vocabulary.Values, ok bool) {
	row, ok := c.resume(id)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.owner, row.source, row.carrier, row.arguments, true
}

// resumeOutcomeCount is always five for a valid resume: Normal, Return,
// Throw, Yield, and Cancel in that canonical Kind order.
func (c *Contract) resumeOutcomeCount(resume vocabulary.ResumeID) int {
	if _, ok := c.resume(resume); !ok {
		return 0
	}
	return crossActivationOutcomes
}

// resumeOutcomeAt returns one canonical cross-activation outcome mapping.
// Outcome is the canonical ordinal of the owning operation outcome.
func (c *Contract) resumeOutcomeAt(resume vocabulary.ResumeID, index int) (kind flowkind.OutcomeKind, outcome uint32, ok bool) {
	if index < 0 || index >= crossActivationOutcomes {
		return 0, 0, false
	}
	value, found := c.resume(resume)
	if !found {
		return 0, 0, false
	}
	return [...]flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel}[index], value.outcomes[index], true
}

func (c *Contract) callbackResultCount(op vocabulary.Operation, outcome int) int {
	outcomes, ok := c.operationOutcomeRange(op)
	if !ok || outcome < 0 || outcome >= outcomes.len() {
		return 0
	}
	return c.outcomes[outcomes.start+uint32(outcome)].callbackResults.len()
}

func (c *Contract) callbackResultRowAt(op vocabulary.Operation, outcome, index int) (callbackResultRow, bool) {
	outcomes, ok := c.operationOutcomeRange(op)
	if !ok || outcome < 0 || outcome >= outcomes.len() {
		return callbackResultRow{}, false
	}
	results := c.outcomes[outcomes.start+uint32(outcome)].callbackResults
	if index < 0 || index >= results.len() {
		return callbackResultRow{}, false
	}
	return c.callbackResults[results.start+uint32(index)], true
}

func (c *Contract) callbackResultAt(op vocabulary.Operation, outcome, index int) (uint32, vocabulary.CallbackID, bool) {
	row, ok := c.callbackResultRowAt(op, outcome, index)
	if !ok {
		return 0, 0, false
	}
	return row.result, row.callback, true
}

func (c *Contract) callbackForResult(op vocabulary.Operation, outcome int, result uint32) (vocabulary.CallbackID, int, bool) {
	count := c.callbackResultCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, ok := c.callbackResultAt(op, outcome, mid)
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
	current, callback, ok := c.callbackResultAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, false
	}
	return callback, left, true
}

// resultAliasCount returns the static result-to-input correspondences owned
// by one outcome.
func (c *Contract) resultAliasCount(op vocabulary.Operation, outcome int) int {
	outcomes, ok := c.operationOutcomeRange(op)
	if !ok || outcome < 0 || outcome >= outcomes.len() {
		return 0
	}
	return c.outcomes[outcomes.start+uint32(outcome)].resultAliases.len()
}

func (c *Contract) resultAliasRowAt(op vocabulary.Operation, outcome, index int) (resultAliasRow, bool) {
	outcomes, ok := c.operationOutcomeRange(op)
	if !ok || outcome < 0 || outcome >= outcomes.len() {
		return resultAliasRow{}, false
	}
	aliases := c.outcomes[outcomes.start+uint32(outcome)].resultAliases
	if index < 0 || index >= aliases.len() {
		return resultAliasRow{}, false
	}
	return c.resultAliases[aliases.start+uint32(index)], true
}

// resultAliasAt returns one canonical result-to-ValueFormal correspondence.
func (c *Contract) resultAliasAt(op vocabulary.Operation, outcome, index int) (uint32, vocabulary.InputSourceKind, uint32, bool) {
	row, ok := c.resultAliasRowAt(op, outcome, index)
	if !ok {
		return 0, 0, 0, false
	}
	return row.result, row.source.Kind, row.source.Ordinal, true
}

// resultAliasForResult finds the correspondence for one fixed result slot.
func (c *Contract) resultAliasForResult(op vocabulary.Operation, outcome int, result uint32) (vocabulary.InputSourceKind, uint32, int, bool) {
	count := c.resultAliasCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, _, ok := c.resultAliasAt(op, outcome, mid)
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
	current, kind, source, ok := c.resultAliasAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, 0, false
	}
	return kind, source, left, true
}

func (c *Contract) producedCount(op vocabulary.Operation, outcome int) int {
	outcomes, ok := c.operationOutcomeRange(op)
	if !ok || outcome < 0 || outcome >= outcomes.len() {
		return 0
	}
	return c.outcomes[outcomes.start+uint32(outcome)].produced.len()
}

func (c *Contract) producedRowAt(op vocabulary.Operation, outcome, index int) (producedRow, bool) {
	outcomes, ok := c.operationOutcomeRange(op)
	if !ok || outcome < 0 || outcome >= outcomes.len() {
		return producedRow{}, false
	}
	produced := c.outcomes[outcomes.start+uint32(outcome)].produced
	if index < 0 || index >= produced.len() {
		return producedRow{}, false
	}
	return c.produced[produced.start+uint32(index)], true
}

func (c *Contract) producedAt(op vocabulary.Operation, outcome, index int) (uint32, vocabulary.Operation, bool) {
	row, ok := c.producedRowAt(op, outcome, index)
	if !ok {
		return 0, 0, false
	}
	return row.result, row.target, true
}

func (c *Contract) producedForResult(op vocabulary.Operation, outcome int, result uint32) (vocabulary.Operation, int, bool) {
	count := c.producedCount(op, outcome)
	left, right := 0, count
	for left < right {
		mid := left + (right-left)/2
		current, _, ok := c.producedAt(op, outcome, mid)
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
	current, target, ok := c.producedAt(op, outcome, left)
	if !ok || current != result {
		return 0, 0, false
	}
	return target, left, true
}

// FreshResultCount is the number of nominal result roots owned by one
// canonical outcome.
func (c *Contract) FreshResultCount(op vocabulary.Operation, outcome int) int {
	outcomes, ok := c.operationOutcomeRange(op)
	if !ok || outcome < 0 || outcome >= outcomes.len() {
		return 0
	}
	return c.outcomes[outcomes.start+uint32(outcome)].fresh.len()
}

func (c *Contract) freshResultAt(op vocabulary.Operation, outcome, index int) (freshResultRow, bool) {
	outcomes, ok := c.operationOutcomeRange(op)
	if !ok || outcome < 0 || outcome >= outcomes.len() {
		return freshResultRow{}, false
	}
	fresh := c.outcomes[outcomes.start+uint32(outcome)].fresh
	if index < 0 || index >= fresh.len() {
		return freshResultRow{}, false
	}
	return c.fresh[fresh.start+uint32(index)], true
}

// FreshResultAt returns one canonical fixed-result freshness relation. The
// ordinal is dense within this outcome and independent of authoring order.
func (c *Contract) FreshResultAt(op vocabulary.Operation, outcome, index int) (result, ordinal uint32, kind schematype.FreshClass, ok bool) {
	row, found := c.freshResultAt(op, outcome, index)
	if !found {
		return 0, 0, schematype.FreshClassInvalid, false
	}
	return row.result, row.ordinal, row.kind, true
}

// freshResultForResult finds the nominal freshness relation for one fixed
// outcome result. The returned index is the canonical FreshResultAt index.
func (c *Contract) freshResultForResult(op vocabulary.Operation, outcome int, result uint32) (ordinal uint32, kind schematype.FreshClass, index int, ok bool) {
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

func (c *Contract) producedCaptureCount(op vocabulary.Operation, outcome, produced int) int {
	row, ok := c.producedRowAt(op, outcome, produced)
	if !ok {
		return 0
	}
	return row.captures.len()
}

func (c *Contract) producedCaptureAt(op vocabulary.Operation, outcome, produced, capture int) (vocabulary.CaptureKind, uint32, bool) {
	row, ok := c.producedRowAt(op, outcome, produced)
	if !ok || capture < 0 || capture >= row.captures.len() {
		return 0, 0, false
	}
	value := c.captures[row.captures.start+uint32(capture)]
	return value.kind, value.ordinal, true
}

// producedTypeValueCapture returns the sole fixed input formal whose runtime
// TypeValue is retained by one Produced row. The relation is indexed when the
// contract seals; it neither scans captures nor infers TypeValue identity
// from the value/runtime-type spelling.
func (c *Contract) producedTypeValueCapture(op vocabulary.Operation, outcome, produced int) (vocabulary.ValueFormal, bool) {
	row, ok := c.producedRowAt(op, outcome, produced)
	if !ok || row.typeValueCapture == noTypeValueCapture || row.typeValueCapture >= uint32(row.captures.len()) {
		return 0, false
	}
	capture := c.captures[row.captures.start+row.typeValueCapture]
	if capture.kind != vocabulary.CaptureTypeValueFormal {
		return 0, false
	}
	return vocabulary.ValueFormal(capture.ordinal), true
}

// TypeDeclaration returns the complete neutral declaration retained by the
// sealed target type row. It preserves primitive atoms and formal-scope
// framing; domain consumers pass this value to their explicit
// schema/typecontract semantic adapter.
func (c *Contract) TypeDeclaration(typ vocabulary.Type) (schematype.Type, bool) {
	if c == nil {
		return schematype.Type{}, false
	}
	return c.Core.TypeDeclaration(typ)
}
