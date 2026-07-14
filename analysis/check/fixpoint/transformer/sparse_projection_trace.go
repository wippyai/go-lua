package transformer

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// sparseProjectionTrace is transaction-owned symbolic projection evidence.
// It is deliberately private: it is neither the evaluated-root DTO nor a
// production read model. Every slot comes from one sealed requirement and no
// full CFG-row binding map crosses this boundary.
type sparseProjectionTrace struct {
	schema    operationplan.ObservationSchemaID
	inventory operationplan.ObservationConsumerInventoryID
	slots     []sparseProjectionSlot
}

type sparseProjectionSlot struct {
	requirement operationplan.ObservationRequirement
	guard       Guard
	fragments   []sparseProjectionFragment
	observed    []ObservationTerm
	owed        []observationObligation
}

// sparseProjectionFragment is the first exact boundary tranche. Values are
// intentionally absent: the only admitted boundary selector is Return, whose
// symbolic payload is already represented by correlated operations and the
// normalized summary output. Other selectors fail before trace publication.
type sparseProjectionFragment struct {
	guard      Guard
	operations []Operation
	output     summary.Summary
}

type sparseProjectionTraceBuilder struct {
	arena  *Arena
	trace  sparseProjectionTrace
	points map[cfg.Point][]int
	edges  map[sparseProjectionEdge][]int
	failed error
}

type sparseProjectionEdge struct{ from, to cfg.Point }

type sparseProjectionPlan struct {
	schema    operationplan.ObservationSchemaID
	inventory operationplan.ObservationConsumerInventoryID
	slots     []operationplan.ObservationRequirement
	points    map[cfg.Point][]int
	edges     map[sparseProjectionEdge][]int
}

func newSparseProjectionTraceBuilder(arena *Arena, requirements operationplan.ObservationRequirements) (*sparseProjectionTraceBuilder, error) {
	if arena == nil || !requirements.Sealed() {
		return nil, fmt.Errorf("projection trace: unsealed requirements or nil arena")
	}
	plan, err := compileSparseProjectionPlan(requirements)
	if err != nil {
		return nil, err
	}
	return plan.newBuilder(arena), nil
}

func compileSparseProjectionPlan(requirements operationplan.ObservationRequirements) (*sparseProjectionPlan, error) {
	if !requirements.Sealed() {
		return nil, fmt.Errorf("projection trace: observation requirements are not sealed")
	}
	slotCount, edgeCount := 0, 0
	countCursor := requirements.Cursor(false)
	for requirement, ok := countCursor.Next(); ok; requirement, ok = countCursor.Next() {
		slotCount++
		if requirement.Stage() == operationplan.RequirementEdge {
			edgeCount++
		}
	}
	plan := &sparseProjectionPlan{
		schema: requirements.SchemaID(), inventory: requirements.ConsumerInventoryID(),
		slots:  make([]operationplan.ObservationRequirement, 0, slotCount),
		points: make(map[cfg.Point][]int, slotCount-edgeCount), edges: make(map[sparseProjectionEdge][]int, edgeCount),
	}
	cursor := requirements.Cursor(false)
	for requirement, ok := cursor.Next(); ok; requirement, ok = cursor.Next() {
		if err := supportedSparseProjectionRequirement(requirement); err != nil {
			return nil, err
		}
		index := len(plan.slots)
		plan.slots = append(plan.slots, requirement)
		if requirement.Stage() == operationplan.RequirementEdge {
			to, _ := requirement.EdgeTarget()
			edge := sparseProjectionEdge{from: requirement.Point(), to: to}
			plan.edges[edge] = append(plan.edges[edge], index)
		} else {
			plan.points[requirement.Point()] = append(plan.points[requirement.Point()], index)
		}
	}
	if len(plan.slots) == 0 {
		return nil, fmt.Errorf("projection trace: sealed requirements contain no slots")
	}
	return plan, nil
}

func (p *sparseProjectionPlan) newBuilder(arena *Arena) *sparseProjectionTraceBuilder {
	if p == nil || arena == nil {
		return nil
	}
	slots := make([]sparseProjectionSlot, len(p.slots))
	for index, requirement := range p.slots {
		slots[index] = sparseProjectionSlot{requirement: requirement, guard: arena.False()}
	}
	b := &sparseProjectionTraceBuilder{
		arena:  arena,
		trace:  sparseProjectionTrace{schema: p.schema, inventory: p.inventory, slots: slots},
		points: p.points, edges: p.edges,
	}
	return b
}

func supportedSparseProjectionRequirement(requirement operationplan.ObservationRequirement) error {
	switch requirement.Stage() {
	case operationplan.RequirementPoint, operationplan.RequirementEdge,
		operationplan.RequirementObservation, operationplan.RequirementRoute:
		return nil
	case operationplan.RequirementBoundary:
		fact, ok := requirement.FactKind()
		if ok && fact == operationplan.Return {
			return nil
		}
	}
	return fmt.Errorf("projection trace: unsupported selector %q", requirement.Projection())
}

func (b *sparseProjectionTraceBuilder) pointInput(point cfg.Point, row SymbolicCFGRow) {
	if b == nil || b.failed != nil {
		return
	}
	for _, index := range b.points[point] {
		slot := &b.trace.slots[index]
		if slot.requirement.Stage() == operationplan.RequirementPoint {
			slot.guard = unionSparseProjectionGuard(b.arena, slot.guard, row.Guard)
		}
	}
}

func (b *sparseProjectionTraceBuilder) pointOutput(point cfg.Point, row SymbolicCFGRow) {
	if b == nil || b.failed != nil {
		return
	}
	for _, index := range b.points[point] {
		slot := &b.trace.slots[index]
		switch slot.requirement.Stage() {
		case operationplan.RequirementBoundary:
			var operations []Operation
			for _, operation := range row.Operations {
				if operation.Descriptor == DescriptorReturn {
					operations = append(operations, operation)
				}
			}
			fragment := sparseProjectionFragment{guard: row.Guard, operations: operations, output: summary.Normalize(b.arena.reg, row.Output)}
			slot.fragments = recordSparseProjectionFragment(b.arena, slot.fragments, fragment)
		case operationplan.RequirementObservation:
			anchor, ok := slot.requirement.Anchor()
			if !ok {
				b.failed = fmt.Errorf("projection trace: observation selector %q has no anchor", slot.requirement.Projection())
				return
			}
			for _, term := range row.Observations {
				if observationProjectionMatches(slot.requirement, term) {
					slot.observed = recordObservationTerm(slot.observed, term)
				}
			}
			for _, obligation := range row.observationObligations {
				if obligation.Anchor == anchor {
					slot.owed = recordobservationObligation(slot.owed, obligation)
				}
			}
		case operationplan.RequirementRoute:
			// Executing the call point is the local invocation fact. Composed
			// descendant routes remain carried by their observation terms.
			slot.guard = unionSparseProjectionGuard(b.arena, slot.guard, row.Guard)
		}
	}
}

func (b *sparseProjectionTraceBuilder) normalEdge(from, to cfg.Point, guard Guard) {
	if b == nil || b.failed != nil {
		return
	}
	for _, index := range b.edges[sparseProjectionEdge{from: from, to: to}] {
		slot := &b.trace.slots[index]
		slot.guard = unionSparseProjectionGuard(b.arena, slot.guard, guard)
	}
}

func (b *sparseProjectionTraceBuilder) freeze() (*sparseProjectionTrace, error) {
	if b == nil {
		return nil, nil
	}
	if b.failed != nil {
		return nil, b.failed
	}
	for index := range b.trace.slots {
		slot := &b.trace.slots[index]
		slot.fragments = canonicalSparseProjectionFragments(b.arena, slot.fragments)
		slot.observed = unionObservationTerms(b.arena, slot.observed)
		slot.owed = canonicalSparseProjectionObligations(slot.owed)
	}
	return &b.trace, nil
}

func canonicalSparseProjectionFragments(arena *Arena, in []sparseProjectionFragment) []sparseProjectionFragment {
	for index := 1; index < len(in); index++ {
		for current := index; current > 0 && lessSparseProjectionFragment(arena, in[current], in[current-1]); current-- {
			in[current], in[current-1] = in[current-1], in[current]
		}
	}
	out := in[:0]
	for _, fragment := range in {
		if len(out) != 0 && equalSparseProjectionFragment(arena, out[len(out)-1], fragment) {
			continue
		}
		out = append(out, fragment)
	}
	return out
}

func recordSparseProjectionFragment(arena *Arena, in []sparseProjectionFragment, next sparseProjectionFragment) []sparseProjectionFragment {
	for index, prior := range in {
		if equalSparseProjectionFragment(arena, prior, next) {
			return in
		}
		if lessSparseProjectionFragment(arena, next, prior) {
			in = append(in, sparseProjectionFragment{})
			copy(in[index+1:], in[index:])
			in[index] = next
			return in
		}
	}
	return append(in, next)
}

func equalSparseProjectionFragment(arena *Arena, left, right sparseProjectionFragment) bool {
	if left.guard != right.guard || len(left.operations) != len(right.operations) || !summary.EqualNormalized(arena.reg, left.output, right.output) {
		return false
	}
	for index := range left.operations {
		if left.operations[index] != right.operations[index] {
			return false
		}
	}
	return true
}

func lessSparseProjectionFragment(arena *Arena, left, right sparseProjectionFragment) bool {
	if left.guard != right.guard {
		return left.guard < right.guard
	}
	limit := len(left.operations)
	if len(right.operations) < limit {
		limit = len(right.operations)
	}
	for index := 0; index < limit; index++ {
		a, b := left.operations[index], right.operations[index]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Descriptor != b.Descriptor {
			return a.Descriptor < b.Descriptor
		}
		if a.Slot != b.Slot {
			return a.Slot < b.Slot
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
	}
	if len(left.operations) != len(right.operations) {
		return len(left.operations) < len(right.operations)
	}
	return summary.NormalizedPayloadDigest(arena.reg, left.output) < summary.NormalizedPayloadDigest(arena.reg, right.output)
}

func sparseProjectionFragmentKey(arena *Arena, fragment sparseProjectionFragment) string {
	key := arena.canonicalGuard(fragment.guard)
	for _, operation := range fragment.operations {
		key += fmt.Sprintf("/%d:%s:%d:%s", operation.Kind, operation.Descriptor, operation.Slot, arena.canonicalValue(operation.Value))
	}
	return key
}

func cloneSparseProjectionTrace(in *sparseProjectionTrace) *sparseProjectionTrace {
	if in == nil {
		return nil
	}
	out := &sparseProjectionTrace{schema: in.schema, inventory: in.inventory, slots: make([]sparseProjectionSlot, len(in.slots))}
	for index, slot := range in.slots {
		out.slots[index] = sparseProjectionSlot{
			requirement: slot.requirement, guard: slot.guard,
			observed:  append([]ObservationTerm(nil), slot.observed...),
			owed:      append([]observationObligation(nil), slot.owed...),
			fragments: make([]sparseProjectionFragment, len(slot.fragments)),
		}
		for fragmentIndex, fragment := range slot.fragments {
			out.slots[index].fragments[fragmentIndex] = sparseProjectionFragment{
				guard: fragment.guard, operations: append([]Operation(nil), fragment.operations...), output: fragment.output.Clone(),
			}
		}
	}
	return out
}

func joinSparseProjectionTrace(arena *Arena, left, right *sparseProjectionTrace) (*sparseProjectionTrace, bool) {
	if left == nil || right == nil {
		return nil, true
	}
	if left.schema != right.schema || left.inventory != right.inventory || len(left.slots) != len(right.slots) {
		return nil, false
	}
	out := cloneSparseProjectionTrace(left)
	for index := range out.slots {
		if out.slots[index].requirement != right.slots[index].requirement {
			return nil, false
		}
		out.slots[index].guard = unionSparseProjectionGuard(arena, out.slots[index].guard, right.slots[index].guard)
		for _, fragment := range right.slots[index].fragments {
			out.slots[index].fragments = recordSparseProjectionFragment(arena, out.slots[index].fragments, fragment)
		}
		out.slots[index].observed = unionObservationTerms(arena, out.slots[index].observed, right.slots[index].observed)
		out.slots[index].owed = canonicalSparseProjectionObligations(unionobservationObligations(out.slots[index].owed, right.slots[index].owed))
	}
	return out, true
}

func canonicalSparseProjectionObligations(in []observationObligation) []observationObligation {
	if len(in) < 2 {
		return in
	}
	sort.Slice(in, func(i, j int) bool {
		left, right := in[i], in[j]
		if order := bytes.Compare(left.BodyOwner[:], right.BodyOwner[:]); order != 0 {
			return order < 0
		}
		if order := bytes.Compare(left.Route[:], right.Route[:]); order != 0 {
			return order < 0
		}
		if left.Anchor != right.Anchor {
			return left.Anchor.Less(right.Anchor)
		}
		return left.Guard < right.Guard
	})
	out := in[:0]
	for _, obligation := range in {
		if len(out) != 0 && out[len(out)-1] == obligation {
			continue
		}
		out = append(out, obligation)
	}
	return out
}

func unionSparseProjectionGuard(arena *Arena, left, right Guard) Guard {
	if left == right || right == arena.False() {
		return left
	}
	if left == arena.False() {
		return right
	}
	return arena.Or(left, right)
}

func observationProjectionMatches(requirement operationplan.ObservationRequirement, term ObservationTerm) bool {
	anchor, ok := requirement.Anchor()
	if !ok || anchor != term.Anchor {
		return false
	}
	kind, ok := requirement.ObservationKind()
	return ok && observation.Kind(term.Kind) == kind
}
