package fixpoint

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Work is one deterministic evaluator input. Dependency is the owner-issued
// scheduling identity; physical execution is recovered only through the
// mount-sealed schedule. Root is the exact immutable database input.
type Work struct {
	dependency model.DependencyID
	root       Root
}

func (work Work) Available() bool { return work.dependency.Available() && work.root.Available() }

// Dependency returns the compiler-issued scheduling identity.
func (work Work) Dependency() model.DependencyID {
	if !work.Available() {
		return model.DependencyID{}
	}
	return work.dependency
}

// ID is the short spelling retained for schedule consumers.
func (work Work) ID() model.DependencyID { return work.Dependency() }

// Root returns the exact full or delta root this work must observe.
func (work Work) Root() Root {
	if !work.Available() {
		return Root{}
	}
	return work.root
}

// Queue is the deterministic, semi-naive solve queue for one exact mounted
// execution. It owns no graph, reverse map, evaluator callback, relation
// state, or widening permit. Execution owns all schedule and wake indexes.
type Queue struct {
	execution arrangement.Execution
	mounted   witness.Mounted
	items     []Work
	seen      map[workKey]struct{}
	head      database.Version
}

// workKey deliberately uses the semantic dependency identity rather than a
// physical node identity. Exact root identity is separately proved on every
// admission; revision makes duplicate wakeups within that live root O(1).
type workKey struct {
	dependency model.DependencyID
	revision   uint64
}

// New binds Queue to an Execution only when it is the exact execution sealed
// in Mounted. A matching fence alone is insufficient: a foreign mount can
// share fence coordinates while carrying different layouts and wake proof.
func New(execution arrangement.Execution, mounted witness.Mounted) (Queue, bool) {
	if !execution.Available() || !mounted.Available() || !execution.Fence().Same(mounted.Fence()) {
		return Queue{}, false
	}
	plan := mounted.Arrangement()
	installed := plan.Execution()
	if !plan.Available() || !installed.Available() || installed.Digest() != execution.Digest() || installed.LogicalDigest() != execution.LogicalDigest() || !installed.Fence().Same(execution.Fence()) || !execution.DependencySchedule().Available() {
		return Queue{}, false
	}
	for _, entry := range execution.Schedules() {
		canonical, ok := execution.Dependency(entry.Dependency())
		if !ok || !sameScheduleEntry(canonical, entry) {
			return Queue{}, false
		}
	}
	return Queue{execution: execution, mounted: mounted, seen: make(map[workKey]struct{})}, true
}

// Execution returns the exact immutable schedule authority redeemed by Queue.
func (queue Queue) Execution() arrangement.Execution {
	if queue.execution.Available() {
		return queue.execution
	}
	return arrangement.Execution{}
}

// Entry redeems the exact sealed entry for current work. It cannot resolve a
// logical expression or traverse a runtime graph.
func (queue Queue) Entry(work Work) (arrangement.ScheduleEntry, bool) {
	if !queue.validWork(work) {
		return arrangement.ScheduleEntry{}, false
	}
	entry, ok := queue.execution.Dependency(work.Dependency())
	if !ok || !entry.Available() {
		return arrangement.ScheduleEntry{}, false
	}
	return entry, true
}

// Node returns the already-mounted physical root for current work.
func (queue Queue) Node(work Work) (arrangement.Node, bool) {
	entry, ok := queue.Entry(work)
	if !ok {
		return arrangement.Node{}, false
	}
	node := entry.Node()
	return node, node.Available()
}

// SeedFull is the sole full traversal: it enqueues each sealed dependency
// once for the initial root. Every successor is delta-driven.
func (queue *Queue) SeedFull(root Root) bool {
	if queue == nil || !queue.ready() || !root.Available() || root.Mode() != FullRoot || queue.head.Available() || !queue.rootMatches(root) {
		return false
	}
	version, ok := root.FullVersion()
	if !ok || !queue.ownsVersion(version) {
		return false
	}
	items, seen, ok := queue.itemsFor(queue.execution.Schedules(), root)
	if !ok {
		return false
	}
	queue.items, queue.seen, queue.head = items, seen, version
	return true
}

// SeedLater admits only a direct committed successor. It carries every
// still-pending dependency identity forward to that successor root and unions
// it with the exact entries selected by sealed column/relation wake indexes.
// It never retains an old Root object or falls back to a full rescan.
func (queue *Queue) SeedLater(root Root) bool {
	if queue == nil || !queue.ready() || !root.Available() || root.Mode() != LaterRoot || !queue.head.Available() || !queue.rootMatches(root) {
		return false
	}
	delta, ok := root.Delta()
	if !ok || !queue.ownsDelta(delta) || !delta.Base().Same(queue.head) {
		return false
	}

	// A pending item has not yet been evaluated. Its old Root is stale after
	// publication, but its sealed DependencyID is still owed one evaluation on
	// the successor. Preserve that identity only; itemsFor issues new Work
	// values carrying root below.
	selected := make(map[model.DependencyID]arrangement.ScheduleEntry)
	for _, pending := range queue.items {
		if !queue.current(pending) {
			continue
		}
		entry, entryOK := queue.execution.Dependency(pending.Dependency())
		if !entryOK || !queue.selectEntry(selected, entry) {
			return false
		}
	}
	for _, column := range delta.ChangedColumnIDs() {
		if !column.Available() {
			return false
		}
		for _, entry := range queue.execution.WakeColumn(column) {
			if !queue.selectEntry(selected, entry) {
				return false
			}
		}
		for _, entry := range queue.execution.WakeRelation(column.Relation()) {
			if !queue.selectEntry(selected, entry) {
				return false
			}
		}
	}
	wake := make([]arrangement.ScheduleEntry, 0, len(selected))
	for _, entry := range selected {
		wake = append(wake, entry)
	}
	items, seen, ok := queue.itemsFor(wake, root)
	if !ok {
		return false
	}
	// Replacement is intentionally last. If a delta or wake entry is
	// malformed, prior work remains untouched; if it is valid, every carried
	// item is recreated only against the exact successor root.
	queue.items, queue.seen, queue.head = items, seen, delta.Next()
	return true
}

// Next removes the first current work item. Removing before evaluation makes
// a later publication boundary explicit: only unconsumed dependency IDs are
// carried forward by SeedLater.
func (queue *Queue) Next() (Work, bool) {
	if queue == nil {
		return Work{}, false
	}
	for len(queue.items) != 0 {
		work := queue.items[0]
		queue.items = queue.items[1:]
		if queue.current(work) {
			return work, true
		}
	}
	return Work{}, false
}

func (queue *Queue) Empty() bool {
	if queue == nil {
		return true
	}
	queue.discardStale()
	return len(queue.items) == 0
}

func (queue *Queue) Len() int {
	if queue == nil {
		return 0
	}
	queue.discardStale()
	return len(queue.items)
}

func (queue *Queue) ready() bool {
	if queue == nil || !queue.execution.Available() || !queue.mounted.Available() || !queue.execution.Fence().Same(queue.mounted.Fence()) || !queue.execution.DependencySchedule().Available() || queue.seen == nil {
		return false
	}
	plan := queue.mounted.Arrangement()
	installed := plan.Execution()
	return plan.Available() && installed.Available() && installed.Digest() == queue.execution.Digest() && installed.LogicalDigest() == queue.execution.LogicalDigest() && installed.Fence().Same(queue.execution.Fence())
}

func (queue *Queue) rootMatches(root Root) bool {
	if queue == nil || !root.Available() || !queue.ready() {
		return false
	}
	switch root.Mode() {
	case FullRoot:
		version, ok := root.FullVersion()
		return ok && queue.ownsVersion(version)
	case LaterRoot:
		delta, ok := root.Delta()
		return ok && queue.ownsDelta(delta)
	default:
		return false
	}
}

func (queue *Queue) ownsVersion(version database.Version) bool {
	if queue == nil || !version.Available() || !version.Mounted().Same(queue.mounted) || !version.Fence().Same(queue.mounted.RuntimeFence()) {
		return false
	}
	plan := version.Arrangement()
	execution := plan.Execution()
	return plan.Available() && execution.Available() && execution.Digest() == queue.execution.Digest() && execution.LogicalDigest() == queue.execution.LogicalDigest() && execution.Fence().Same(queue.execution.Fence())
}

func (queue *Queue) ownsDelta(delta database.Delta) bool {
	return queue != nil && delta.Available() && queue.ownsVersion(delta.Base()) && queue.ownsVersion(delta.Next()) && delta.Next().SuccessorOf(delta.Base())
}

func (queue *Queue) validWork(work Work) bool {
	return queue != nil && queue.ready() && work.Available() && queue.current(work)
}

func (queue *Queue) current(work Work) bool {
	if queue == nil || !work.Available() || !queue.head.Available() || !queue.entryMatches(work.Dependency()) || !queue.rootMatches(work.Root()) {
		return false
	}
	root := work.Root()
	switch root.Mode() {
	case FullRoot:
		version, ok := root.FullVersion()
		return ok && version.Same(queue.head)
	case LaterRoot:
		delta, ok := root.Delta()
		return ok && delta.Next().Same(queue.head)
	default:
		return false
	}
}

func (queue *Queue) discardStale() {
	if queue == nil || len(queue.items) == 0 {
		return
	}
	kept := queue.items[:0]
	for _, work := range queue.items {
		if queue.current(work) {
			kept = append(kept, work)
		}
	}
	queue.items = kept
}

// itemsFor validates the complete replacement vector before Queue state is
// changed. This is the atomic root-boundary law.
func (queue *Queue) itemsFor(entries []arrangement.ScheduleEntry, root Root) ([]Work, map[workKey]struct{}, bool) {
	if queue == nil || !root.Available() || !queue.rootMatches(root) || root.Revision() == 0 {
		return nil, nil, false
	}
	ordered := append([]arrangement.ScheduleEntry(nil), entries...)
	sort.SliceStable(ordered, func(left, right int) bool { return scheduleEntryLess(ordered[left], ordered[right]) })
	items := make([]Work, 0, len(ordered))
	seen := make(map[workKey]struct{}, len(ordered))
	for _, entry := range ordered {
		if !entry.Available() || !queue.entryMatches(entry.Dependency()) {
			return nil, nil, false
		}
		key := workKey{dependency: entry.Dependency(), revision: root.Revision()}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, Work{dependency: entry.Dependency(), root: root})
	}
	return items, seen, true
}

// selectEntry unions only exact schedule entries from sealed wake vectors.
func (queue *Queue) selectEntry(selected map[model.DependencyID]arrangement.ScheduleEntry, entry arrangement.ScheduleEntry) bool {
	if queue == nil || selected == nil || !entry.Available() || !queue.entryMatches(entry.Dependency()) {
		return false
	}
	prior, exists := selected[entry.Dependency()]
	if exists && !sameScheduleEntry(prior, entry) {
		return false
	}
	selected[entry.Dependency()] = entry
	return true
}

func (queue *Queue) entryMatches(dependency model.DependencyID) bool {
	if queue == nil || !dependency.Available() || !queue.execution.Available() {
		return false
	}
	entry, ok := queue.execution.Dependency(dependency)
	return ok && entry.Available()
}

func sameScheduleEntry(left, right arrangement.ScheduleEntry) bool {
	return left.Available() && right.Available() && left.Dependency() == right.Dependency() && left.Expression() == right.Expression() && left.Component() == right.Component() && left.Node().Digest() == right.Node().Digest()
}

func scheduleEntryLess(left, right arrangement.ScheduleEntry) bool {
	if left.Component() != right.Component() {
		return left.Component() < right.Component()
	}
	return dependencyLess(left.Dependency(), right.Dependency())
}

func dependencyLess(left, right model.DependencyID) bool {
	leftOwner, rightOwner := left.Owner().Content(), right.Owner().Content()
	if identityLess(leftOwner, rightOwner) {
		return true
	}
	if identityLess(rightOwner, leftOwner) {
		return false
	}
	return identityLess(left.Content(), right.Content())
}

func identityLess(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}
