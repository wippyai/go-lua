package arrangement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	internalschedule "github.com/wippyai/go-lua/analysis/relation/mount/arrangement/internal/schedule"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// The schedule's pure identity/index vocabulary belongs in the internal
// child subsystem. Arrangement deliberately exposes aliases rather than a
// second copy: runtime consumers retain the same public names while this
// altitude remains the sole owner of opaque physical Nodes.
type RecurrenceKind = internalschedule.RecurrenceKind

const (
	RecurrenceInvalid  = internalschedule.RecurrenceInvalid
	RecurrenceAcyclic  = internalschedule.RecurrenceAcyclic
	RecurrencePositive = internalschedule.RecurrencePositive
)

type DependencyEdge = internalschedule.Edge
type WideningHead = internalschedule.Head
type ScheduleComponent = internalschedule.Component

// ScheduleEntry joins an internal immutable dependency record to the exact
// arrangement-owned physical Node. Node pointers never cross into the child
// schedule package, so the schedule cannot become a second plan authority.
type ScheduleEntry struct {
	record internalschedule.Entry
	node   Node
}

func (entry ScheduleEntry) Available() bool {
	return entry.record.Available() && entry.node.Available() && entry.node.Digest() == entry.record.NodeDigest()
}

// Dependency returns the owner-issued work identity.
func (entry ScheduleEntry) Dependency() model.DependencyID {
	if !entry.Available() {
		return model.DependencyID{}
	}
	return entry.record.Dependency()
}

// ID is the historical public spelling for Dependency.
func (entry ScheduleEntry) ID() model.DependencyID { return entry.Dependency() }

// Expression returns the compiler-issued root expression identity.
func (entry ScheduleEntry) Expression() model.ExpressionID {
	if !entry.Available() {
		return model.ExpressionID{}
	}
	return entry.record.Expression()
}

// Root is the historical public spelling for Expression.
func (entry ScheduleEntry) Root() model.ExpressionID { return entry.Expression() }

// Node returns the sealed physical root. Runtime never resolves Expression
// again; this is only an immutable redemption of the mount-issued root.
func (entry ScheduleEntry) Node() Node {
	if !entry.Available() {
		return Node{}
	}
	return entry.node
}

func (entry ScheduleEntry) RelationReads() []model.RelationID {
	if !entry.Available() {
		return nil
	}
	return entry.record.RelationReads()
}

// Reads is the historical public spelling for RelationReads.
func (entry ScheduleEntry) Reads() []model.RelationID { return entry.RelationReads() }

func (entry ScheduleEntry) ColumnReads() []model.ColumnID {
	if !entry.Available() {
		return nil
	}
	return entry.record.ColumnReads()
}

func (entry ScheduleEntry) Writes() []model.RelationID {
	if !entry.Available() {
		return nil
	}
	return entry.record.Writes()
}

func (entry ScheduleEntry) WideningRelations() []model.RelationID {
	if !entry.Available() {
		return nil
	}
	return entry.record.WideningRelations()
}

// WideningFor reports the certificate-issued dependency+destination pair.
func (entry ScheduleEntry) WideningFor(relation model.RelationID) bool {
	return entry.Available() && entry.record.WideningFor(relation)
}

// Component is the sealed SCC solve position.
func (entry ScheduleEntry) Component() uint32 {
	if !entry.Available() {
		return 0
	}
	return entry.record.Component()
}

// DependencySchedule is the arrangement facade over the pure schedule table.
// It retains only the O(1) physical-root projection necessary to turn a
// schedule record back into an opaque Node.
type DependencySchedule struct{ data *dependencyScheduleData }

type dependencyScheduleData struct {
	schedule internalschedule.Schedule
	roots    map[model.ExpressionID]*executionNode
	sealed   bool
}

func (schedule DependencySchedule) Available() bool {
	return schedule.data != nil && schedule.data.sealed && schedule.data.schedule.Available() && schedule.data.roots != nil
}

func (schedule DependencySchedule) Digest() identity.ContentID {
	if !schedule.Available() {
		return identity.ContentID{}
	}
	return schedule.data.schedule.Digest()
}

func (schedule DependencySchedule) LogicalDigest() identity.ContentID {
	if !schedule.Available() {
		return identity.ContentID{}
	}
	return schedule.data.schedule.LogicalDigest()
}

func (schedule DependencySchedule) Dependency(id model.DependencyID) (ScheduleEntry, bool) {
	if !schedule.Available() {
		return ScheduleEntry{}, false
	}
	record, ok := schedule.data.schedule.Entry(id)
	if !ok {
		return ScheduleEntry{}, false
	}
	return schedule.redeem(record)
}

func (schedule DependencySchedule) Schedules() []ScheduleEntry {
	if !schedule.Available() {
		return nil
	}
	records := schedule.data.schedule.Entries()
	// A valid empty schedule remains an authenticated empty vector. Wake
	// lookups intentionally return nil for no match, but initial seeding must
	// retain the distinction from an unavailable schedule.
	result := make([]ScheduleEntry, 0, len(records))
	for _, record := range records {
		entry, ok := schedule.redeem(record)
		if !ok {
			return nil
		}
		result = append(result, entry)
	}
	return result
}

func (schedule DependencySchedule) Component(order uint32) (ScheduleComponent, bool) {
	if !schedule.Available() {
		return ScheduleComponent{}, false
	}
	return schedule.data.schedule.Component(order)
}

func (schedule DependencySchedule) Components() []ScheduleComponent {
	if !schedule.Available() {
		return nil
	}
	return schedule.data.schedule.Components()
}

func (schedule DependencySchedule) DependencyCount() int {
	if !schedule.Available() {
		return 0
	}
	return schedule.data.schedule.Len()
}

func (schedule DependencySchedule) WakeRelation(relation model.RelationID) []ScheduleEntry {
	if !schedule.Available() {
		return nil
	}
	return schedule.redeemAll(schedule.data.schedule.WakeRelation(relation))
}

func (schedule DependencySchedule) WakeColumn(column model.ColumnID) []ScheduleEntry {
	if !schedule.Available() {
		return nil
	}
	return schedule.redeemAll(schedule.data.schedule.WakeColumn(column))
}

func (schedule DependencySchedule) redeemAll(records []internalschedule.Entry) []ScheduleEntry {
	if len(records) == 0 {
		return nil
	}
	result := make([]ScheduleEntry, 0, len(records))
	for _, record := range records {
		entry, ok := schedule.redeem(record)
		if !ok {
			return nil
		}
		result = append(result, entry)
	}
	return result
}

func (schedule DependencySchedule) redeem(record internalschedule.Entry) (ScheduleEntry, bool) {
	if !schedule.Available() || !record.Available() {
		return ScheduleEntry{}, false
	}
	root, ok := schedule.data.roots[record.Expression()]
	if !ok || root == nil {
		return ScheduleEntry{}, false
	}
	entry := ScheduleEntry{record: record, node: Node{value: root}}
	return entry, entry.Available()
}

// buildDependencySchedule is the mount-side bridge into the child schedule
// subsystem. It traverses the already-lowered physical Node tree exactly once
// to collect sealed column wake evidence; internal/schedule then owns all
// recurrence construction, indexing, validation, and digesting.
func buildDependencySchedule(recurrence certificate.RecurrenceData, entries []executionEntry) (DependencySchedule, bool) {
	if !recurrence.Available() || entries == nil {
		return DependencySchedule{}, false
	}
	roots := make(map[model.ExpressionID]*executionNode, len(entries))
	for _, entry := range entries {
		if !entry.id.Available() || !entry.digest.Available() || entry.root == nil || !entry.root.digest.Available() {
			return DependencySchedule{}, false
		}
		if _, duplicate := roots[entry.id]; duplicate {
			return DependencySchedule{}, false
		}
		roots[entry.id] = entry.root
	}
	projections := recurrence.Projections()
	bindings := make([]internalschedule.Binding, 0, len(projections))
	for _, projection := range projections {
		dependency, expression := projection.Dependency(), projection.Expression()
		root, found := roots[expression]
		if !dependency.Available() || !expression.Available() || !found || root == nil || !root.digest.Available() {
			return DependencySchedule{}, false
		}
		columns, columnsOK := scheduleColumns(Node{value: root}, projection.Reads())
		if !columnsOK {
			return DependencySchedule{}, false
		}
		binding, bindingOK := internalschedule.NewBinding(dependency, expression, root.digest, projection.Reads(), columns, projection.Writes())
		if !bindingOK {
			return DependencySchedule{}, false
		}
		bindings = append(bindings, binding)
	}
	value, ok := internalschedule.Build(recurrence, bindings)
	if !ok || !value.Available() {
		return DependencySchedule{}, false
	}
	data := &dependencyScheduleData{schedule: value, roots: roots, sealed: true}
	result := DependencySchedule{data: data}
	return result, result.Available()
}
