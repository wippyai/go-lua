package call

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
)

// BehaviorResultCount projects the number of neutral result correspondences
// declared for this known operation target. The existing Target capability is
// the owner fence; no Target Contract, binding spelling, or provider object
// crosses this Call boundary.
func (target Target) BehaviorResultCount() int {
	operation, ok := target.behaviorOperation()
	if !ok {
		return 0
	}
	return target.owner.contract.BehaviorResultCount(operation)
}

// BehaviorResultAt projects one neutral result correspondence. Relation is an
// opaque schema identity and Source is only the operation-local input
// coordinate needed by the downstream Value-owned cross-axis consumer.
func (target Target) BehaviorResultAt(index int) (outcome, result uint32, source vocabulary.InputSource, relation schema.EntryID, ok bool) {
	operation, operationOK := target.behaviorOperation()
	if !operationOK || index < 0 || index >= target.BehaviorResultCount() {
		return 0, 0, vocabulary.InputSource{}, schema.EntryID{}, false
	}
	outcome, result, source, relation, ok = target.owner.contract.BehaviorResultAt(operation, index)
	if !ok || !relation.Available() {
		return 0, 0, vocabulary.InputSource{}, schema.EntryID{}, false
	}
	return outcome, result, source, relation, true
}

// BehaviorPredicateCount projects the number of neutral predicate
// correspondences declared for this known operation target.
func (target Target) BehaviorPredicateCount() int {
	operation, ok := target.behaviorOperation()
	if !ok {
		return 0
	}
	return target.owner.contract.BehaviorPredicateCount(operation)
}

// BehaviorPredicateAt projects one neutral predicate correspondence. Branch
// polarity remains outside this declaration and is owned by the consumer.
func (target Target) BehaviorPredicateAt(index int) (outcome, result uint32, subject vocabulary.InputSource, relation schema.EntryID, ok bool) {
	operation, operationOK := target.behaviorOperation()
	if !operationOK || index < 0 || index >= target.BehaviorPredicateCount() {
		return 0, 0, vocabulary.InputSource{}, schema.EntryID{}, false
	}
	outcome, result, subject, relation, ok = target.owner.contract.BehaviorPredicateAt(operation, index)
	if !ok || !relation.Available() {
		return 0, 0, vocabulary.InputSource{}, schema.EntryID{}, false
	}
	return outcome, result, subject, relation, true
}

// behaviorOperation is the only Target-to-behavior conversion. Operation()
// already authenticates the existing target row and rejects body targets;
// this helper adds no second capability or receipt vocabulary.
func (target Target) behaviorOperation() (vocabulary.Operation, bool) {
	if !target.Valid() || target.owner.contract == nil {
		return 0, false
	}
	return target.Operation()
}
