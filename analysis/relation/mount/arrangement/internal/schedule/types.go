// Package schedule owns the immutable, mount-time dependency schedule.
//
// It deliberately knows only certificate recurrence identities and physical
// node digests. The enclosing arrangement package retains the opaque Node
// redemption capability, which prevents this cold index from becoming a
// second physical-plan or evaluator authority.
package schedule

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// RecurrenceKind is the checked recurrence policy redeemed by the solver.
type RecurrenceKind uint8

const (
	RecurrenceInvalid RecurrenceKind = iota
	RecurrenceAcyclic
	RecurrencePositive
)

// Available reports whether the closed recurrence vocabulary is known.
func (kind RecurrenceKind) Available() bool {
	return kind == RecurrenceAcyclic || kind == RecurrencePositive
}

// Edge is one canonical dependency edge inside a sealed component.
type Edge struct {
	from model.DependencyID
	to   model.DependencyID
}

func (edge Edge) Available() bool          { return edge.from.Available() && edge.to.Available() }
func (edge Edge) From() model.DependencyID { return edge.from }
func (edge Edge) To() model.DependencyID   { return edge.to }

// Head is one certificate-issued widening permission association.
type Head struct {
	dependency model.DependencyID
	relation   model.RelationID
}

func (head Head) Available() bool {
	return head.dependency.Available() && head.relation.Available()
}
func (head Head) Dependency() model.DependencyID { return head.dependency }
func (head Head) Relation() model.RelationID     { return head.relation }

// Component is one immutable SCC in deterministic solve order.
type Component struct {
	order      uint32
	members    []model.DependencyID
	edges      []Edge
	recurrence RecurrenceKind
	heads      []Head
}

func (component Component) Available() bool {
	if !component.recurrence.Available() || component.members == nil {
		return false
	}
	seen := make(map[model.DependencyID]struct{}, len(component.members))
	for _, member := range component.members {
		if !member.Available() {
			return false
		}
		if _, duplicate := seen[member]; duplicate {
			return false
		}
		seen[member] = struct{}{}
	}
	for _, edge := range component.edges {
		if !edge.Available() {
			return false
		}
		if _, ok := seen[edge.from]; !ok {
			return false
		}
		if _, ok := seen[edge.to]; !ok {
			return false
		}
	}
	for _, head := range component.heads {
		if !head.Available() {
			return false
		}
		if _, ok := seen[head.dependency]; !ok {
			return false
		}
	}
	return component.recurrence != RecurrenceAcyclic || len(component.heads) == 0
}

func (component Component) Order() uint32 { return component.order }
func (component Component) Members() []model.DependencyID {
	return append([]model.DependencyID(nil), component.members...)
}
func (component Component) Edges() []Edge { return append([]Edge(nil), component.edges...) }
func (component Component) Recurrence() RecurrenceKind {
	return component.recurrence
}
func (component Component) Heads() []Head { return append([]Head(nil), component.heads...) }

// Binding is the arrangement-owned physical evidence supplied to Build. It
// contains no Node pointer: the node stays at its owning arrangement layer;
// this child package retains only the stable digest needed by the schedule
// certificate. All vectors are copied at birth.
type Binding struct {
	dependency model.DependencyID
	expression model.ExpressionID
	node       identity.ContentID
	reads      []model.RelationID
	columns    []model.ColumnID
	writes     []model.RelationID
}

// NewBinding seals the physical evidence already selected by mount. Build
// checks it against the certificate recurrence projection before admitting it
// to the schedule.
func NewBinding(dependency model.DependencyID, expression model.ExpressionID, node identity.ContentID, reads []model.RelationID, columns []model.ColumnID, writes []model.RelationID) (Binding, bool) {
	binding := Binding{
		dependency: dependency,
		expression: expression,
		node:       node,
		reads:      append([]model.RelationID{}, reads...),
		columns:    append([]model.ColumnID{}, columns...),
		writes:     append([]model.RelationID{}, writes...),
	}
	return binding, binding.Available()
}

func (binding Binding) Available() bool {
	if !binding.dependency.Available() || !binding.expression.Available() || !binding.node.Available() || binding.reads == nil || binding.columns == nil || binding.writes == nil {
		return false
	}
	for _, relation := range append(append([]model.RelationID{}, binding.reads...), binding.writes...) {
		if !relation.Available() {
			return false
		}
	}
	for _, column := range binding.columns {
		if !column.Available() {
			return false
		}
	}
	return true
}

// Entry is the immutable work record returned by a Schedule. Work identity
// is Dependency, not Node: many dependencies may intentionally share a
// physical expression root.
type Entry struct {
	id        model.DependencyID
	root      model.ExpressionID
	node      identity.ContentID
	reads     []model.RelationID
	columns   []model.ColumnID
	writes    []model.RelationID
	heads     []model.RelationID
	component uint32
}

func (entry Entry) Available() bool {
	if !entry.id.Available() || !entry.root.Available() || !entry.node.Available() || entry.reads == nil || entry.columns == nil || entry.writes == nil || entry.heads == nil {
		return false
	}
	for _, column := range entry.columns {
		if !column.Available() {
			return false
		}
	}
	for _, relation := range append(append([]model.RelationID{}, entry.reads...), append(entry.writes, entry.heads...)...) {
		if !relation.Available() {
			return false
		}
	}
	return true
}

func (entry Entry) Dependency() model.DependencyID { return entry.id }
func (entry Entry) Expression() model.ExpressionID { return entry.root }
func (entry Entry) NodeDigest() identity.ContentID { return entry.node }
func (entry Entry) RelationReads() []model.RelationID {
	return append([]model.RelationID(nil), entry.reads...)
}
func (entry Entry) ColumnReads() []model.ColumnID {
	return append([]model.ColumnID(nil), entry.columns...)
}
func (entry Entry) Writes() []model.RelationID {
	return append([]model.RelationID(nil), entry.writes...)
}
func (entry Entry) WideningRelations() []model.RelationID {
	return append([]model.RelationID(nil), entry.heads...)
}
func (entry Entry) WideningFor(relation model.RelationID) bool {
	if !entry.Available() || !relation.Available() {
		return false
	}
	for _, head := range entry.heads {
		if head == relation {
			return true
		}
	}
	return false
}
func (entry Entry) Component() uint32 { return entry.component }

// Schedule is the closed, immutable dependency/index artifact. Its public
// redemption surface has no resolver, plan callback, or mutable graph.
type Schedule struct{ data *data }

type data struct {
	entries    []Entry
	byID       map[model.DependencyID]int
	byRelation map[model.RelationID][]int
	byColumn   map[model.ColumnID][]int
	components []Component
	digest     identity.ContentID
	logical    identity.ContentID
	sealed     bool
}

func (schedule Schedule) Available() bool {
	return schedule.data != nil && schedule.data.sealed && schedule.data.entries != nil && schedule.data.byID != nil && schedule.data.byRelation != nil && schedule.data.byColumn != nil && schedule.data.components != nil && schedule.data.logical.Available() && schedule.data.digest.Available() && len(schedule.data.entries) == len(schedule.data.byID)
}

func (schedule Schedule) Digest() identity.ContentID {
	if !schedule.Available() {
		return identity.ContentID{}
	}
	return schedule.data.digest
}

func (schedule Schedule) LogicalDigest() identity.ContentID {
	if !schedule.Available() {
		return identity.ContentID{}
	}
	return schedule.data.logical
}

func (schedule Schedule) Entry(id model.DependencyID) (Entry, bool) {
	if !schedule.Available() || !id.Available() {
		return Entry{}, false
	}
	index, ok := schedule.data.byID[id]
	if !ok || index < 0 || index >= len(schedule.data.entries) {
		return Entry{}, false
	}
	entry := cloneEntry(schedule.data.entries[index])
	return entry, entry.Available() && entry.id == id
}

func (schedule Schedule) Entries() []Entry {
	if !schedule.Available() {
		return nil
	}
	result := make([]Entry, len(schedule.data.entries))
	for index, entry := range schedule.data.entries {
		result[index] = cloneEntry(entry)
	}
	return result
}

func (schedule Schedule) Component(order uint32) (Component, bool) {
	if !schedule.Available() || order >= uint32(len(schedule.data.components)) {
		return Component{}, false
	}
	component := cloneComponent(schedule.data.components[order])
	return component, component.Available() && component.order == order
}

func (schedule Schedule) Components() []Component {
	if !schedule.Available() {
		return nil
	}
	result := make([]Component, len(schedule.data.components))
	for index, component := range schedule.data.components {
		result[index] = cloneComponent(component)
	}
	return result
}

func (schedule Schedule) Len() int {
	if !schedule.Available() {
		return 0
	}
	return len(schedule.data.entries)
}

func (schedule Schedule) WakeRelation(relation model.RelationID) []Entry {
	if !schedule.Available() || !relation.Available() {
		return nil
	}
	return schedule.entriesAt(schedule.data.byRelation[relation])
}

func (schedule Schedule) WakeColumn(column model.ColumnID) []Entry {
	if !schedule.Available() || !column.Available() {
		return nil
	}
	return schedule.entriesAt(schedule.data.byColumn[column])
}

func (schedule Schedule) entriesAt(indices []int) []Entry {
	if len(indices) == 0 {
		return nil
	}
	result := make([]Entry, 0, len(indices))
	for _, index := range indices {
		if index >= 0 && index < len(schedule.data.entries) {
			result = append(result, cloneEntry(schedule.data.entries[index]))
		}
	}
	return result
}

func cloneEntry(value Entry) Entry {
	value.reads = append([]model.RelationID{}, value.reads...)
	value.columns = append([]model.ColumnID{}, value.columns...)
	value.writes = append([]model.RelationID{}, value.writes...)
	value.heads = append([]model.RelationID{}, value.heads...)
	return value
}

func cloneComponent(value Component) Component {
	value.members = append([]model.DependencyID{}, value.members...)
	value.edges = append([]Edge{}, value.edges...)
	value.heads = append([]Head{}, value.heads...)
	return value
}
