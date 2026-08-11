package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// topologyOperands is the cold typed payload authority for one sealed source
// Batch. Rule declarations are reusable schemas, not mutable instance
// registries. Once frozen, the plan is detached from the disposable Assembly;
// Solver retains only that standalone immutable plan for activation revisions.
type topologyOperands struct {
	rows   []topologyOperandRows
	sealed bool
}

type topologyOperandRows interface {
	schema() *ruleSchema
	freeze() bool
	newRevision() topologyOperandRevisionRows
}

// typedTopologyOperands owns one monomorphic O table for one Rule schema in
// one source topology. The table is sorted by immutable base origin and
// searched only during cold compilation; no payload lookup remains in the
// solve path.
type typedTopologyOperands[V, O any] struct {
	rule *Rule[V, O]
	rows []typedTopologyOperand[O]
}

type typedTopologyOperand[O any] struct {
	occurrence equation.Occurrence
	operand    equation.Operand
	value      O
}

func (rows *typedTopologyOperands[V, O]) schema() *ruleSchema {
	if rows == nil || rows.rule == nil {
		return nil
	}
	return rows.rule.schema
}

func (rows *typedTopologyOperands[V, O]) add(occurrence equation.Occurrence, operand equation.Operand, value O) bool {
	if rows == nil || rows.rule == nil || rows.rule.schema == nil || !rows.rule.available() ||
		!occurrence.Available() || !operand.Available() || !operand.Occurrence().Same(occurrence) {
		return false
	}
	rows.rows = append(rows.rows, typedTopologyOperand[O]{occurrence: occurrence, operand: operand, value: value})
	return true
}

func (rows *typedTopologyOperands[V, O]) freeze() bool {
	if rows == nil || rows.rule == nil || rows.rule.schema == nil || len(rows.rows) == 0 {
		return false
	}
	sort.Slice(rows.rows, func(left, right int) bool {
		return lessOperandOrigin(rows.rows[left].occurrence, rows.rows[left].operand, rows.rows[right].occurrence, rows.rows[right].operand)
	})
	for index, row := range rows.rows {
		if !row.occurrence.Available() || !row.operand.Available() || !row.operand.Occurrence().Same(row.occurrence) ||
			index > 0 && sameOperandOrigin(rows.rows[index-1].occurrence, rows.rows[index-1].operand, row.occurrence, row.operand) {
			return false
		}
		frozen, digest, ok := rows.rule.operandContent(row.value)
		if !ok || !(OperandEntity{key: row.operand.Entity()}).MatchesContentDigest(digest) {
			return false
		}
		rows.rows[index].value = frozen
	}
	return true
}

func (rows *typedTopologyOperands[V, O]) newRevision() topologyOperandRevisionRows {
	if rows == nil || rows.rule == nil {
		return nil
	}
	return &typedTopologyOperandRevision[V, O]{plan: rows}
}

func addTopologyOperand[V, O any](set *topologyOperands, rule *Rule[V, O], occurrence equation.Occurrence, operand equation.Operand, value O) bool {
	if set == nil || set.sealed || rule == nil || rule.schema == nil || !rule.available() ||
		rule.schema.outputKind != ruleFactorOutput || rule.schema.output == nil {
		return false
	}
	for _, candidate := range set.rows {
		if candidate == nil || candidate.schema() != rule.schema {
			continue
		}
		typed, ok := candidate.(*typedTopologyOperands[V, O])
		return ok && typed.rule == rule && typed.add(occurrence, operand, value)
	}
	rows := &typedTopologyOperands[V, O]{rule: rule}
	if !rows.add(occurrence, operand, value) {
		return false
	}
	set.rows = append(set.rows, rows)
	return true
}

func (set *topologyOperands) freeze() bool {
	if set == nil || set.sealed {
		return false
	}
	for _, rows := range set.rows {
		if rows == nil || !rows.freeze() {
			return false
		}
	}
	sort.Slice(set.rows, func(left, right int) bool {
		return lessRuntimeKey(set.rows[left].schema().semantic.compositionKey(), set.rows[right].schema().semantic.compositionKey())
	})
	for index, rows := range set.rows {
		if rows.schema() == nil || index > 0 && set.rows[index-1].schema().semantic == rows.schema().semantic {
			return false
		}
	}
	set.sealed = true
	return true
}

// detach moves one frozen operand plan out of its disposable Assembly owner.
// The returned allocation may survive for activation revisions without
// retaining the Assembly's gate, wires, counters, or local capabilities.
func (set *topologyOperands) detach() (*topologyOperands, bool) {
	if set == nil || !set.sealed {
		return nil, false
	}
	detached := &topologyOperands{rows: set.rows, sealed: true}
	set.rows = nil
	set.sealed = false
	return detached, true
}

func (set *topologyOperands) revision(graph *equation.Graph) (*topologyOperandRevision, bool) {
	if set == nil || !set.sealed || graph == nil {
		return nil, false
	}
	revision := &topologyOperandRevision{rows: make([]topologyOperandRevisionRows, len(set.rows))}
	for index, rows := range set.rows {
		revision.rows[index] = rows.newRevision()
		if revision.rows[index] == nil {
			return nil, false
		}
	}
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok {
			return nil, false
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, ok := group.MemberAt(memberIndex)
			if !ok {
				return nil, false
			}
			index := sort.Search(len(revision.rows), func(index int) bool {
				return !lessRuntimeKey(revision.rows[index].schema().semantic.compositionKey(), member.Rule())
			})
			if index >= len(revision.rows) || revision.rows[index].schema().semantic.compositionKey() != member.Rule() {
				continue
			}
			if !revision.rows[index].add(member) {
				return nil, false
			}
		}
	}
	for _, rows := range revision.rows {
		if !rows.finish() {
			return nil, false
		}
	}
	return revision, true
}

// topologyOperandRevision is one accepted Graph's short-lived direct
// RuleMember.Key-to-O binder.  It is consumed while boundRule values are
// created and never retained by Solver or solverRuntime.
type topologyOperandRevision struct{ rows []topologyOperandRevisionRows }

func (revision *topologyOperandRevision) bind(member equation.RuleMember, factors map[composition.Key]runtimeFactor) (runtimeMember, bool) {
	if revision == nil || !member.Rule().Available() {
		return nil, false
	}
	index := sort.Search(len(revision.rows), func(index int) bool {
		return !lessRuntimeKey(revision.rows[index].schema().semantic.compositionKey(), member.Rule())
	})
	if index >= len(revision.rows) || revision.rows[index].schema().semantic.compositionKey() != member.Rule() {
		return nil, false
	}
	return revision.rows[index].bind(member, factors)
}

type topologyOperandRevisionRows interface {
	schema() *ruleSchema
	add(equation.RuleMember) bool
	finish() bool
	bind(equation.RuleMember, map[composition.Key]runtimeFactor) (runtimeMember, bool)
}

type typedTopologyMember[O any] struct {
	member composition.Key
	value  O
}

type typedTopologyOperandRevision[V, O any] struct {
	plan    *typedTopologyOperands[V, O]
	members []typedTopologyMember[O]
}

func (rows *typedTopologyOperandRevision[V, O]) schema() *ruleSchema {
	if rows == nil || rows.plan == nil {
		return nil
	}
	return rows.plan.schema()
}

func (rows *typedTopologyOperandRevision[V, O]) add(member equation.RuleMember) bool {
	if rows == nil || rows.plan == nil || rows.plan.rule == nil || member.Rule() != rows.plan.rule.schema.semantic.compositionKey() ||
		member.OperandFamily() != rows.plan.rule.schema.operandFamily.compositionKey() || !member.Operand().Available() || !member.Occurrence().Available() {
		return false
	}
	index := sort.Search(len(rows.plan.rows), func(index int) bool {
		source := rows.plan.rows[index]
		return !lessOperandOrigin(source.occurrence, source.operand, member.Occurrence(), member.Operand())
	})
	if index >= len(rows.plan.rows) {
		return false
	}
	source := rows.plan.rows[index]
	if !sameOperandOrigin(source.occurrence, source.operand, member.Occurrence(), member.Operand()) {
		return false
	}
	rows.members = append(rows.members, typedTopologyMember[O]{member: member.Key(), value: source.value})
	return true
}

func (rows *typedTopologyOperandRevision[V, O]) finish() bool {
	if rows == nil {
		return false
	}
	sort.Slice(rows.members, func(left, right int) bool {
		return lessRuntimeKey(rows.members[left].member, rows.members[right].member)
	})
	for index, row := range rows.members {
		if !row.member.Available() || index > 0 && rows.members[index-1].member == row.member {
			return false
		}
	}
	return true
}

func (rows *typedTopologyOperandRevision[V, O]) bind(member equation.RuleMember, factors map[composition.Key]runtimeFactor) (runtimeMember, bool) {
	if rows == nil || rows.plan == nil || rows.plan.rule == nil || factors == nil || !member.Key().Available() {
		return nil, false
	}
	index := sort.Search(len(rows.members), func(index int) bool {
		return !lessRuntimeKey(rows.members[index].member, member.Key())
	})
	if index >= len(rows.members) || rows.members[index].member != member.Key() {
		return nil, false
	}
	output, present := factors[rows.plan.rule.schema.output.semantic.compositionKey()]
	if !present || output == nil {
		return nil, false
	}
	result, ok := bindRuleMember(member, rows.plan.rule, rows.members[index].value, output)
	if !ok || result == nil || !bindRuntimeRuleReads(rows.plan.rule.schema, result.rule, member, factors) {
		return nil, false
	}
	return result, true
}

func sameOperandOrigin(leftOccurrence equation.Occurrence, leftOperand equation.Operand, rightOccurrence equation.Occurrence, rightOperand equation.Operand) bool {
	if !leftOccurrence.Available() || !leftOperand.Available() || !rightOccurrence.Available() || !rightOperand.Available() {
		return false
	}
	return !lessOperandOrigin(leftOccurrence, leftOperand, rightOccurrence, rightOperand) &&
		!lessOperandOrigin(rightOccurrence, rightOperand, leftOccurrence, leftOperand)
}

// lessOperandOrigin orders the immutable base coordinates retained by both
// ordinary and activation-derived rows. Dynamic overlays deliberately expose
// their base source, kind, and entities here, so cancellation/rebuild needs
// no closure or runtime source lookup to recover O.
func lessOperandOrigin(leftOccurrence equation.Occurrence, leftOperand equation.Operand, rightOccurrence equation.Occurrence, rightOperand equation.Operand) bool {
	leftSource, rightSource := leftOccurrence.Site().Source(), rightOccurrence.Site().Source()
	if leftSource != rightSource {
		return lessRuntimeKey(leftSource, rightSource)
	}
	if leftOccurrence.Kind() != rightOccurrence.Kind() {
		return leftOccurrence.Kind() < rightOccurrence.Kind()
	}
	leftEntity, rightEntity := leftOccurrence.Entity(), rightOccurrence.Entity()
	if leftEntity != rightEntity {
		return lessRuntimeKey(leftEntity, rightEntity)
	}
	return lessRuntimeKey(leftOperand.Entity(), rightOperand.Entity())
}
