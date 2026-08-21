package engine

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

// Read is the typed, positional capability issued by SchemaBinding.
// Its compiled row is shared with the owning hot implementation; no
// construction state or cold execution carrier is retained here.
type Read[S any] struct {
	row     *schemaRuleReadRow
	index   int
	resolve func(*productSession, int, uint64) (S, bool)
}

func (read Read[S]) matchesRuntimeOwner(owner anyRule) bool {
	if owner == nil || read.index < 0 || read.resolve == nil || read.row == nil {
		return false
	}
	cell := owner.runtimeRuleCell()
	return cell != nil && read.row.sealed() && read.index == int(read.row.readOrdinal) && read.row.owner == cell && read.row.ownerOrdinal == owner.runtimeRuleOrdinal()
}

// matchesActivationOwner is the activation-only Read fence. Activation
// compilation has already rejected the complete sealed Schema/read geometry;
// Fold-time compares the retained owner row and exact read ordinal directly.
// Ordinary Rule reads use the same canonical cell/ordinal address.
func (read Read[S]) matchesActivationOwner(owner *compiledActivationRule) bool {
	if owner == nil || owner.cell == nil || owner.cell.state == nil || owner.cell.ordinal != owner.ordinal || read.index != 0 || read.resolve == nil || read.row == nil || owner.readCount != 1 {
		return false
	}
	return read.row.sealed() && read.row.owner == owner.cell && read.row.ownerOrdinal == owner.ordinal && read.row.readOrdinal == 0
}

// Frame is one opaque, synchronous Product row issued by the engine to a
// Rule Fold. It grants typed reads and operand access for that row only; it
// carries no Patch, target, Work, or publication capability.
type Frame[V, O any] struct {
	execution *ruleExecution
	owner     *boundRuleMember[V, O]
	epoch     identity.Generation
	row       int
}

type ruleResultKind uint8

const (
	ruleResultInvalid ruleResultKind = iota
	ruleResultNoCandidate
	ruleResultStaged
	ruleResultRouted
)

// RuleResult is an opaque row-local publication intent. Domain folds can
// construct it only through the engine constructors; the engine alone owns
// target authentication, carry application, and Patch mutation.
type RuleResult[V any] struct {
	execution *ruleExecution
	epoch     identity.Generation
	row       int
	kind      ruleResultKind
	value     V
	route     routeOutputBatch[V]
}
