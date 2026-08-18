// Package demand owns one selected Point closure's reverse dependencies.
// Structural routes are sealed CSR; each epoch owns only the current exact
// dynamic read inverse relation. It has no activation candidate or secondary
// work queue.
package demand

import (
	"math/bits"
	"slices"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// Observation is one input-qualified typed dependency.
type Observation struct {
	Input uint64
	Unit  carrier.Unit
}

// DynamicRead is one sealed permission for a staged exact read. It identifies
// the source input and target Factor slot, but intentionally names no Unit:
// the active Unit set is discovered only from a completed Product execution
// and kept in Epoch's sparse inverse relation.
type DynamicRead struct {
	Input uint64
	Slot  shape.Slot
}

func lessDynamicRead(left, right DynamicRead) bool {
	if left.Input != right.Input {
		return left.Input < right.Input
	}
	return left.Slot < right.Slot
}

// Carry is one input-qualified whole-Factor dependency.
type Carry struct {
	Input uint64
	Slot  shape.Slot
}

// Family is the presealed runtime metadata for one dense graph Group index.
// Indexes are graph-issued and never semantic-key lookups on the hot path.
type Family struct {
	Group        int
	Inputs       []int
	InitialReads []Observation
	DynamicReads []DynamicRead
	Carries      []Carry
}

type carryCSR struct {
	point int
	slot  shape.Slot
	begin int
	end   int
}

// Plan is dense in Graph group order. Static exact reads are sealed into CSR
// once before an Epoch exists. Dynamic reads contribute only sealed
// (input, Factor-slot) permissions; their concrete exact Units and sparse
// inverse routes belong exclusively to an Epoch.
type Plan struct {
	mu          sync.RWMutex
	graph       *equation.Graph
	runtime     *carrier.Composition
	families    []Family
	declared    []bool
	selected    []bool
	carryRows   []carryCSR
	carryGroups []int
	readGroups  []readGroup
	readDecls   []readDecl
	sourceAt    map[unitKey]int
	sources     []sourceCSR
	edges       []inverseEdge
	maxWakes    int
	sealed      bool
}

func NewPlan(graph *equation.Graph, runtime *carrier.Composition) *Plan {
	if graph == nil || runtime == nil || graph.GroupCount() < 0 || graph.PointCount() < 0 {
		return nil
	}
	return &Plan{graph: graph, runtime: runtime, families: make([]Family, graph.GroupCount()), declared: make([]bool, graph.GroupCount()), selected: make([]bool, graph.GroupCount())}
}

func (plan *Plan) ownsUnit(unit carrier.Unit) bool {
	if plan == nil || plan.runtime == nil {
		return false
	}
	slot, ok := unit.Slot()
	return ok && plan.runtime.OwnsUnit(slot, unit)
}
func (plan *Plan) ownsSlot(slot shape.Slot) bool {
	return plan != nil && plan.runtime != nil && slot >= 0 && int(slot) < plan.runtime.Count()
}

// Declare copies one Group family's static read surface into its graph-dense
// slot. The Group and each Input index are validated against Graph once.
func (plan *Plan) Declare(family Family) bool {
	if plan == nil || family.Group < 0 || family.Group >= len(plan.families) {
		return false
	}
	plan.mu.Lock()
	defer plan.mu.Unlock()
	if plan.sealed || plan.declared[family.Group] {
		return false
	}
	group, ok := plan.graph.HyperedgeAt(family.Group)
	if !ok || !plan.graph.OwnsGroup(group) || len(family.Inputs) != group.InputCount() {
		return false
	}
	inputs := append([]int(nil), family.Inputs...)
	for index, pointIndex := range inputs {
		input, inputOK := group.InputAt(index)
		actual, pointOK := plan.graph.PointIndex(input.Point())
		if !inputOK || !pointOK || pointIndex != actual || pointIndex < 0 || pointIndex >= plan.graph.PointCount() {
			return false
		}
	}
	reads := make([]Observation, 0, len(family.InitialReads))
	for _, observation := range family.InitialReads {
		if observation.Input >= uint64(len(inputs)) || !plan.ownsUnit(observation.Unit) {
			return false
		}
		reads = append(reads, observation)
	}
	sort.Slice(reads, func(left, right int) bool { return lessObservation(reads[left], reads[right]) })
	reads = uniqueObservations(reads)
	dynamic := make([]DynamicRead, 0, len(family.DynamicReads))
	for _, read := range family.DynamicReads {
		if read.Input >= uint64(len(inputs)) || !plan.ownsSlot(read.Slot) {
			return false
		}
		dynamic = append(dynamic, read)
	}
	sort.Slice(dynamic, func(left, right int) bool { return lessDynamicRead(dynamic[left], dynamic[right]) })
	dynamic = uniqueDynamicReads(dynamic)
	carries := make([]Carry, 0, len(family.Carries))
	for _, carry := range family.Carries {
		if carry.Input >= uint64(len(inputs)) || !plan.ownsSlot(carry.Slot) {
			return false
		}
		duplicate := false
		for _, known := range carries {
			if carry == known {
				duplicate = true
				break
			}
		}
		if !duplicate {
			carries = append(carries, carry)
		}
	}
	family.Inputs, family.InitialReads, family.DynamicReads, family.Carries = inputs, reads, dynamic, carries
	plan.families[family.Group] = family
	plan.declared[family.Group] = true
	return true
}

func uniqueObservations(rows []Observation) []Observation {
	result := rows[:0]
	for _, row := range rows {
		if len(result) != 0 && result[len(result)-1].Input == row.Input && result[len(result)-1].Unit.Same(row.Unit) {
			continue
		}
		result = append(result, row)
	}
	return result
}

func uniqueDynamicReads(rows []DynamicRead) []DynamicRead {
	result := rows[:0]
	for _, row := range rows {
		if len(result) != 0 && result[len(result)-1] == row {
			continue
		}
		result = append(result, row)
	}
	return result
}

type carryEntry struct {
	point int
	slot  shape.Slot
	group int
}

func lessCarryEntry(left, right carryEntry) bool {
	if left.point != right.point {
		return left.point < right.point
	}
	if left.slot != right.slot {
		return left.slot < right.slot
	}
	return left.group < right.group
}

func (plan *Plan) buildCSR() bool {
	carries := make([]carryEntry, 0)
	for groupIndex := range plan.families {
		family := plan.families[groupIndex]
		for _, carry := range family.Carries {
			carries = append(carries, carryEntry{point: family.Inputs[carry.Input], slot: carry.Slot, group: groupIndex})
		}
	}
	sort.Slice(carries, func(left, right int) bool { return lessCarryEntry(carries[left], carries[right]) })
	plan.carryRows = make([]carryCSR, 0, len(carries))
	plan.carryGroups = make([]int, 0, len(carries))
	for begin := 0; begin < len(carries); {
		end := begin + 1
		for end < len(carries) && carries[begin].point == carries[end].point && carries[begin].slot == carries[end].slot {
			end++
		}
		row := carryCSR{point: carries[begin].point, slot: carries[begin].slot, begin: len(plan.carryGroups)}
		last := -1
		for index := begin; index < end; index++ {
			if carries[index].group != last {
				plan.carryGroups = append(plan.carryGroups, carries[index].group)
				last = carries[index].group
			}
		}
		row.end = len(plan.carryGroups)
		plan.carryRows = append(plan.carryRows, row)
		begin = end
	}
	plan.maxWakes = 2*len(plan.selected) + len(plan.carryGroups)
	return true
}

// selectedForPoints derives the dense Group activation mask for one already
// sealed graph view. It never mutates Plan, which lets a later epoch widen its
// selection while the immutable carry/read CSR remains shared.
func (plan *Plan) selectedForPoints(points []int) ([]bool, bool) {
	if plan == nil || plan.graph == nil || len(points) == 0 {
		return nil, false
	}
	expected := make([]bool, len(plan.families))
	seenPoints := make([]bool, plan.graph.PointCount())
	for _, pointIndex := range points {
		if pointIndex < 0 || pointIndex >= len(seenPoints) {
			return nil, false
		}
		if seenPoints[pointIndex] {
			continue
		}
		seenPoints[pointIndex] = true
		point, ok := plan.graph.PointAt(schedule.Node(pointIndex))
		if !ok || !plan.graph.OwnsPoint(point) {
			return nil, false
		}
		for index := 0; index < plan.graph.ProducerCount(point); index++ {
			group, ok := plan.graph.ProducerAt(point, index)
			groupIndex, indexed := plan.graph.GroupIndex(group)
			if !ok || !indexed || groupIndex < 0 || groupIndex >= len(expected) || !plan.declared[groupIndex] {
				return nil, false
			}
			expected[groupIndex] = true
		}
	}
	return expected, true
}

// Seal selects dense Point roots and proves every selected producer has an
// attached Family. All declared families are indexed once here; the initial
// selected mask only controls the first Epoch and is never used to rebuild
// the immutable carry/read indexes.
func (plan *Plan) Seal(points []int) bool {
	if plan == nil || len(points) == 0 {
		return false
	}
	plan.mu.Lock()
	defer plan.mu.Unlock()
	if plan.sealed {
		return false
	}
	expected, ok := plan.selectedForPoints(points)
	if !ok {
		return false
	}
	for index := range expected {
		if !plan.declared[index] {
			return false
		}
	}
	copy(plan.selected, expected)
	if !plan.buildCSR() || !plan.buildReadIndex() {
		return false
	}
	plan.sealed = true
	return true
}

type unitKey struct {
	point int
	unit  carrier.Unit
}

// readGroup is one sealed Group-local static observation index. The member
// value is an ordinal in initial, not a mutable active declaration ID: a
// normal replacement activates every declared static read as one immutable
// relation, while Replace(nil) deactivates that entire relation.
type readGroup struct {
	members     map[Observation]int
	initial     []int
	staticEdges []staticEdge
}

// staticEdge folds the declaration multiplicity for one sealed inverse edge.
// It lets an Epoch toggle a whole static group without rebuilding its static
// relation or allocating a transient edge-delta map.
type staticEdge struct {
	edge  int
	count uint64
}

type readDecl struct {
	observation Observation
	source      int
	group       int
	edge        int
}

type sourceCSR struct {
	begin int
	end   int
}

// inverseEdge is one possible source -> Group subscription.  Edges are
// sorted by source then Group at seal time, so the Epoch bitset emits a
// deterministic Group order without maintaining mutable subscriber vectors.
type inverseEdge struct{ group int }

type edgeKey struct {
	source int
	group  int
}

func (plan *Plan) buildReadIndex() bool {
	if plan == nil || len(plan.readGroups) != 0 || len(plan.readDecls) != 0 || len(plan.sourceAt) != 0 || len(plan.sources) != 0 || len(plan.edges) != 0 {
		return false
	}
	plan.readGroups = make([]readGroup, len(plan.families))
	plan.sourceAt = make(map[unitKey]int)
	edges := make(map[edgeKey]struct{})
	for group := range plan.families {
		family := plan.families[group]
		index := readGroup{members: make(map[Observation]int, len(family.InitialReads)), initial: make([]int, 0, len(family.InitialReads))}
		for _, observation := range family.InitialReads {
			if observation.Input >= uint64(len(family.Inputs)) || !plan.ownsUnit(observation.Unit) {
				return false
			}
			if _, duplicate := index.members[observation]; duplicate {
				return false
			}
			key := unitKey{point: family.Inputs[observation.Input], unit: observation.Unit}
			source, known := plan.sourceAt[key]
			if !known {
				source = len(plan.sourceAt)
				plan.sourceAt[key] = source
			}
			declaration := len(plan.readDecls)
			index.members[observation] = len(index.initial)
			index.initial = append(index.initial, declaration)
			plan.readDecls = append(plan.readDecls, readDecl{observation: observation, source: source, group: group, edge: -1})
			edges[edgeKey{source: source, group: group}] = struct{}{}
		}
		plan.readGroups[group] = index
	}
	ordered := make([]edgeKey, 0, len(edges))
	for key := range edges {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].source != ordered[right].source {
			return ordered[left].source < ordered[right].source
		}
		return ordered[left].group < ordered[right].group
	})
	plan.sources = make([]sourceCSR, len(plan.sourceAt))
	edgeAt := make(map[edgeKey]int, len(ordered))
	for source, begin := 0, 0; source < len(plan.sources); source++ {
		end := begin
		for end < len(ordered) && ordered[end].source == source {
			key := ordered[end]
			edgeAt[key] = end
			plan.edges = append(plan.edges, inverseEdge{group: key.group})
			end++
		}
		if begin == end {
			return false
		}
		plan.sources[source] = sourceCSR{begin: begin, end: end}
		begin = end
	}
	for declaration := range plan.readDecls {
		row := &plan.readDecls[declaration]
		edge, found := edgeAt[edgeKey{source: row.source, group: row.group}]
		if !found {
			return false
		}
		row.edge = edge
	}
	for group := range plan.readGroups {
		index := &plan.readGroups[group]
		if len(index.initial) == 0 {
			continue
		}
		counts := make(map[int]uint64, len(index.initial))
		for _, declaration := range index.initial {
			if declaration < 0 || declaration >= len(plan.readDecls) {
				return false
			}
			edge := plan.readDecls[declaration].edge
			if edge < 0 || edge >= len(plan.edges) || counts[edge] == ^uint64(0) {
				return false
			}
			counts[edge]++
		}
		index.staticEdges = make([]staticEdge, 0, len(counts))
		for edge, count := range counts {
			index.staticEdges = append(index.staticEdges, staticEdge{edge: edge, count: count})
		}
		sort.Slice(index.staticEdges, func(left, right int) bool { return index.staticEdges[left].edge < index.staticEdges[right].edge })
	}
	return true
}

type groupReads struct {
	static      bool
	staticSeen  []uint64
	staticEpoch uint64

	// dynamicScratch is never the group’s live dynamic route slice. A commit
	// swaps it with the old live backing slice only after every validation
	// succeeds, so rejected replacements cannot alter the active relation.
	dynamicScratch []Observation
	oldKeyScratch  []unitKey
	newKeyScratch  []unitKey
	routeScratch   []dynamicRouteUpdate
}

// Epoch owns the one current exact dynamic inverse. The Plan holds only
// potential, sealed edges. edgeBits says exactly which source -> Group edges
// are active; edgeCounts accounts for aliased declared observations that share
// one physical edge. No mutable subscriber slice exists.
//
// Each Group owns detached staging buffers. Replace validates into those
// buffers, proves its sparse inverse update, then commits in one critical
// section. Stable warmed routes neither rebuild the immutable static relation
// nor allocate; rejected replacements never write a live backing slice.
type Epoch struct {
	mu   sync.RWMutex
	plan *Plan
	// selected is epoch-local activation over the Plan's immutable indexes.
	// A widened revision opens a new Epoch with a larger mask without mutating
	// the sealed Plan or rebuilding its CSR.
	selected   []bool
	groups     []groupReads
	edgeCounts []uint64
	edgeBits   []uint64
	wakeSeen   []uint64
	wakeEpoch  uint64
	wakes      []Wake
	// Coverage routing has a distinct caller-visible result lifetime. Sharing
	// wakes here would let RouteCoverage overwrite a still-live RoutePoint
	// slice before the executor finishes cross-reason de-duplication.
	coverageWakes []Wake
	// dynamicByGroup is the current canonical active route set for each
	// Group. dynamicBySource is its sparse inverse keyed by the actual source
	// Point and selected exact Unit. Neither exists in Plan because a staged
	// locator must not create a cold candidate×root relation.
	dynamicByGroup  [][]Observation
	dynamicBySource map[unitKey][]int
	groupOwned      []bool
	dynamicShared   bool
	live            bool
}

func Open(plan *Plan) (*Epoch, bool) {
	if plan == nil {
		return nil, false
	}
	plan.mu.RLock()
	defer plan.mu.RUnlock()
	if !plan.sealed || len(plan.readGroups) != len(plan.families) {
		return nil, false
	}
	return openSelected(plan, append([]bool(nil), plan.selected...))
}

// OpenSelected opens an Epoch over the same sealed Plan indexes with a
// widened Point-root selection. The caller supplies graph-dense Point
// indexes; all producer families must already have been declared before the
// one-time Seal.
func OpenSelected(plan *Plan, points []int) (*Epoch, bool) {
	if plan == nil {
		return nil, false
	}
	plan.mu.RLock()
	defer plan.mu.RUnlock()
	if !plan.sealed || len(plan.readGroups) != len(plan.families) {
		return nil, false
	}
	selected, ok := plan.selectedForPoints(points)
	if !ok {
		return nil, false
	}
	return openSelected(plan, selected)
}

// Widen opens a new epoch over the same sealed Plan indexes while preserving
// the current exact relation. Only newly selected Group static rows are
// admitted at this cut; existing dynamic routes remain shared until the new
// epoch first writes them through Replace's copy-on-write boundary.
func (epoch *Epoch) Widen(points []int) (*Epoch, bool) {
	if epoch == nil {
		return nil, false
	}
	epoch.mu.Lock()
	defer epoch.mu.Unlock()
	if !epoch.live || epoch.plan == nil || len(epoch.selected) != len(epoch.plan.families) {
		return nil, false
	}
	epoch.plan.mu.RLock()
	selected, ok := epoch.plan.selectedForPoints(points)
	epoch.plan.mu.RUnlock()
	if !ok || len(selected) != len(epoch.selected) {
		return nil, false
	}
	for group, active := range epoch.selected {
		if active && !selected[group] {
			return nil, false
		}
	}
	next := &Epoch{
		plan:            epoch.plan,
		selected:        append([]bool(nil), selected...),
		groups:          append([]groupReads(nil), epoch.groups...),
		edgeCounts:      append([]uint64(nil), epoch.edgeCounts...),
		edgeBits:        append([]uint64(nil), epoch.edgeBits...),
		wakeSeen:        make([]uint64, len(epoch.wakeSeen)),
		wakes:           make([]Wake, 0, epoch.plan.maxWakes),
		coverageWakes:   make([]Wake, 0, epoch.plan.maxWakes),
		dynamicByGroup:  append([][]Observation(nil), epoch.dynamicByGroup...),
		dynamicBySource: epoch.dynamicBySource,
		groupOwned:      make([]bool, len(epoch.groups)),
		dynamicShared:   true,
		live:            true,
	}
	for group, active := range selected {
		if !active {
			continue
		}
		if epoch.selected[group] {
			continue
		}
		index := epoch.plan.readGroups[group]
		state := &next.groups[group]
		state.static = true
		state.staticSeen = make([]uint64, len(index.initial))
		for _, contribution := range index.staticEdges {
			if contribution.edge < 0 || contribution.edge >= len(next.edgeCounts) || contribution.count == 0 || next.edgeCounts[contribution.edge] > ^uint64(0)-contribution.count {
				return nil, false
			}
			next.edgeCounts[contribution.edge] += contribution.count
		}
		next.groupOwned[group] = true
	}
	for edge, count := range next.edgeCounts {
		if count != 0 {
			next.edgeBits[edge>>6] |= uint64(1) << uint(edge&63)
		} else {
			next.edgeBits[edge>>6] &^= uint64(1) << uint(edge&63)
		}
	}
	return next, true
}

func (epoch *Epoch) prepareGroupWrite(group int) bool {
	if epoch == nil || group < 0 || group >= len(epoch.groups) {
		return false
	}
	if !epoch.dynamicShared || group >= len(epoch.groupOwned) || epoch.groupOwned[group] {
		return true
	}
	state := &epoch.groups[group]
	state.staticSeen = append([]uint64(nil), state.staticSeen...)
	state.dynamicScratch = nil
	state.oldKeyScratch = nil
	state.newKeyScratch = nil
	state.routeScratch = nil
	epoch.groupOwned[group] = true
	return true
}

func (epoch *Epoch) prepareDynamicMapWrite() bool {
	if epoch == nil || !epoch.dynamicShared {
		return epoch != nil
	}
	next := make(map[unitKey][]int, len(epoch.dynamicBySource))
	for key, groups := range epoch.dynamicBySource {
		next[key] = groups
	}
	epoch.dynamicBySource = next
	epoch.dynamicShared = false
	return true
}

func openSelected(plan *Plan, selected []bool) (*Epoch, bool) {
	if plan == nil || len(selected) != len(plan.families) {
		return nil, false
	}
	epoch := &Epoch{
		plan:            plan,
		selected:        append([]bool(nil), selected...),
		groups:          make([]groupReads, len(plan.families)),
		edgeCounts:      make([]uint64, len(plan.edges)),
		edgeBits:        make([]uint64, (len(plan.edges)+63)/64),
		wakeSeen:        make([]uint64, len(plan.families)),
		wakes:           make([]Wake, 0, plan.maxWakes),
		coverageWakes:   make([]Wake, 0, plan.maxWakes),
		dynamicByGroup:  make([][]Observation, len(plan.families)),
		dynamicBySource: make(map[unitKey][]int),
		groupOwned:      make([]bool, len(plan.families)),
		live:            true,
	}
	for group, selected := range selected {
		if !selected {
			continue
		}
		if group < 0 || group >= len(plan.families) {
			return nil, false
		}
		index := plan.readGroups[group]
		state := &epoch.groups[group]
		state.static = true
		state.staticSeen = make([]uint64, len(index.initial))
		for _, contribution := range index.staticEdges {
			if contribution.edge < 0 || contribution.edge >= len(epoch.edgeCounts) || contribution.count == 0 || epoch.edgeCounts[contribution.edge] > ^uint64(0)-contribution.count {
				return nil, false
			}
			epoch.edgeCounts[contribution.edge] += contribution.count
		}
	}
	for edge, count := range epoch.edgeCounts {
		if count != 0 {
			epoch.setEdgeBit(edge, true)
		}
	}
	return epoch, true
}

func lessObservation(left, right Observation) bool {
	if left.Input != right.Input {
		return left.Input < right.Input
	}
	if left.Unit.Same(right.Unit) {
		return false
	}
	return left.Unit.Less(right.Unit)
}

// nextWakeEpoch starts one Point-publication-local Group set.  A Group needs
// only one dirty notification for a published ChangeSet, even when several
// exact Units, a Factor carry, and a support transition all justify it.
// Stamps retain the first canonical witness without a per-publication map or
// allocation.
func (epoch *Epoch) nextWakeEpoch() uint64 {
	if epoch.wakeEpoch == ^uint64(0) {
		clear(epoch.wakeSeen)
		epoch.wakeEpoch = 1
		return epoch.wakeEpoch
	}
	epoch.wakeEpoch++
	if epoch.wakeEpoch == 0 {
		epoch.wakeEpoch = 1
	}
	return epoch.wakeEpoch
}

func (epoch *Epoch) setEdgeBit(edge int, active bool) {
	word, bit := edge>>6, uint(edge&63)
	if active {
		epoch.edgeBits[word] |= uint64(1) << bit
		return
	}
	epoch.edgeBits[word] &^= uint64(1) << bit
}

func (epoch *Epoch) edgeBit(edge int) bool {
	return epoch.edgeBits[edge>>6]&(uint64(1)<<uint(edge&63)) != 0
}

// nextStaticEpoch starts one detached static-input validation attempt. Its
// stamps are staging metadata only; a rejected replacement cannot affect the
// active static relation, inverse bits, or dynamic routes.
func (state *groupReads) nextStaticEpoch() uint64 {
	if state == nil {
		return 0
	}
	if state.staticEpoch == ^uint64(0) {
		clear(state.staticSeen)
		state.staticEpoch = 1
		return state.staticEpoch
	}
	state.staticEpoch++
	if state.staticEpoch == 0 {
		state.staticEpoch = 1
	}
	return state.staticEpoch
}

// validStaticToggle proves that the immutable sealed static contribution can
// move from active to next without changing an edge count outside its lawful
// range. No scratch map is needed because each Group's contribution was
// canonicalized at Plan.Seal.
func (epoch *Epoch) validStaticToggle(group int, active, next bool) bool {
	if epoch == nil || epoch.plan == nil || group < 0 || group >= len(epoch.plan.readGroups) {
		return false
	}
	if active == next {
		return true
	}
	for _, contribution := range epoch.plan.readGroups[group].staticEdges {
		if contribution.edge < 0 || contribution.edge >= len(epoch.edgeCounts) || contribution.count == 0 || epoch.edgeBit(contribution.edge) != (epoch.edgeCounts[contribution.edge] != 0) {
			return false
		}
		if next {
			if epoch.edgeCounts[contribution.edge] > ^uint64(0)-contribution.count {
				return false
			}
		} else if epoch.edgeCounts[contribution.edge] < contribution.count {
			return false
		}
	}
	return true
}

// applyStaticToggle executes a previously validated sealed static
// contribution. It has no rejection path and therefore cannot split an
// otherwise atomic replacement after dynamic validation succeeded.
func (epoch *Epoch) applyStaticToggle(group int, active, next bool) {
	if active == next {
		return
	}
	for _, contribution := range epoch.plan.readGroups[group].staticEdges {
		if next {
			epoch.edgeCounts[contribution.edge] += contribution.count
		} else {
			epoch.edgeCounts[contribution.edge] -= contribution.count
		}
		epoch.setEdgeBit(contribution.edge, epoch.edgeCounts[contribution.edge] != 0)
	}
}

func sameObservation(left, right Observation) bool {
	return left.Input == right.Input && left.Unit.Same(right.Unit)
}

func sameDynamicKey(left, right unitKey) bool {
	return left.point == right.point && left.unit.Same(right.unit)
}

func lessDynamicKey(left, right unitKey) bool {
	if left.point != right.point {
		return left.point < right.point
	}
	if left.unit.Same(right.unit) {
		return false
	}
	return left.unit.Less(right.unit)
}

// dynamicSourceKey verifies a completed staged route against the one sealed
// DynamicRead declaration that authorizes it. It validates the actual exact
// Unit ownership before deriving the source point; callers cannot route a
// unit merely because it happens to be in the carrier layout.
func (epoch *Epoch) dynamicSourceKey(group int, observation Observation) (unitKey, bool) {
	if epoch == nil || epoch.plan == nil || group < 0 || group >= len(epoch.plan.families) || observation.Input >= uint64(len(epoch.plan.families[group].Inputs)) || observation.Unit.Kind() != carrier.ExactUnit || !epoch.plan.ownsUnit(observation.Unit) {
		return unitKey{}, false
	}
	slot, slotOK := observation.Unit.Slot()
	if !slotOK {
		return unitKey{}, false
	}
	declared := epoch.plan.families[group].DynamicReads
	index := sort.Search(len(declared), func(index int) bool {
		candidate := declared[index]
		return candidate.Input > observation.Input || candidate.Input == observation.Input && candidate.Slot >= slot
	})
	if index >= len(declared) || declared[index].Input != observation.Input || declared[index].Slot != slot {
		return unitKey{}, false
	}
	return unitKey{point: epoch.plan.families[group].Inputs[observation.Input], unit: observation.Unit}, true
}

func (epoch *Epoch) dynamicKeysInto(group int, observations []Observation, keys []unitKey) ([]unitKey, bool) {
	keys = keys[:0]
	for _, observation := range observations {
		key, ok := epoch.dynamicSourceKey(group, observation)
		if !ok {
			return nil, false
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return lessDynamicKey(keys[left], keys[right]) })
	write := 0
	for _, key := range keys {
		if write != 0 && sameDynamicKey(keys[write-1], key) {
			continue
		}
		if write != 0 && !lessDynamicKey(keys[write-1], key) {
			return nil, false
		}
		keys[write] = key
		write++
	}
	return keys[:write], true
}

func groupContains(groups []int, group int) (int, bool) {
	index := sort.SearchInts(groups, group)
	return index, index < len(groups) && groups[index] == group
}

func validSortedGroups(groups []int) bool {
	for index, group := range groups {
		if group < 0 || index > 0 && groups[index-1] >= group {
			return false
		}
	}
	return true
}

func copyWithoutDynamicGroup(groups []int, group int) ([]int, bool) {
	if !validSortedGroups(groups) {
		return nil, false
	}
	index, found := groupContains(groups, group)
	if !found {
		return nil, false
	}
	result := make([]int, 0, len(groups)-1)
	result = append(result, groups[:index]...)
	result = append(result, groups[index+1:]...)
	return result, true
}

func copyWithDynamicGroup(groups []int, group int) ([]int, bool) {
	if !validSortedGroups(groups) {
		return nil, false
	}
	index, duplicate := groupContains(groups, group)
	if duplicate {
		return nil, false
	}
	result := make([]int, 0, len(groups)+1)
	result = append(result, groups[:index]...)
	result = append(result, group)
	result = append(result, groups[index:]...)
	return result, true
}

type dynamicRouteUpdate struct {
	key     unitKey
	groups  []int
	present bool
}

// stageDynamicReplace derives every altered sparse inverse vector before
// either side of the relation mutates. Source keys are canonical and the
// two-pointer merge is O(old + new) after their individual canonicalization.
func (epoch *Epoch) stageDynamicReplace(group int, oldKeys, newKeys []unitKey, updates []dynamicRouteUpdate) ([]dynamicRouteUpdate, bool) {
	if epoch == nil || epoch.dynamicBySource == nil {
		return nil, false
	}
	updates = updates[:0]
	oldIndex, newIndex := 0, 0
	for oldIndex < len(oldKeys) || newIndex < len(newKeys) {
		if oldIndex < len(oldKeys) && (newIndex == len(newKeys) || lessDynamicKey(oldKeys[oldIndex], newKeys[newIndex])) {
			key := oldKeys[oldIndex]
			groups, present := epoch.dynamicBySource[key]
			if !present {
				return nil, false
			}
			next, ok := copyWithoutDynamicGroup(groups, group)
			if !ok {
				return nil, false
			}
			updates = append(updates, dynamicRouteUpdate{key: key, groups: next, present: len(next) != 0})
			oldIndex++
			continue
		}
		if newIndex < len(newKeys) && (oldIndex == len(oldKeys) || lessDynamicKey(newKeys[newIndex], oldKeys[oldIndex])) {
			key := newKeys[newIndex]
			groups := epoch.dynamicBySource[key]
			next, ok := copyWithDynamicGroup(groups, group)
			if !ok {
				return nil, false
			}
			updates = append(updates, dynamicRouteUpdate{key: key, groups: next, present: true})
			newIndex++
			continue
		}
		if oldIndex >= len(oldKeys) || newIndex >= len(newKeys) || !sameDynamicKey(oldKeys[oldIndex], newKeys[newIndex]) {
			return nil, false
		}
		groups, present := epoch.dynamicBySource[oldKeys[oldIndex]]
		if !present || !validSortedGroups(groups) {
			return nil, false
		}
		if _, found := groupContains(groups, group); !found {
			return nil, false
		}
		oldIndex++
		newIndex++
	}
	return updates, true
}

func canonicalDynamicObservations(rows []Observation) []Observation {
	// slices.SortFunc is monomorphized for Observation, unlike sort.Slice's
	// reflection adapter. After the detached epoch scratch reaches capacity,
	// canonicalization remains O(K log K) and allocation-free.
	slices.SortFunc(rows, func(left, right Observation) int {
		if left.Input < right.Input {
			return -1
		}
		if left.Input > right.Input {
			return 1
		}
		if left.Unit.Same(right.Unit) {
			return 0
		}
		if left.Unit.Less(right.Unit) {
			return -1
		}
		return 1
	})
	write := 0
	for _, row := range rows {
		if write != 0 && sameObservation(rows[write-1], row) {
			continue
		}
		rows[write] = row
		write++
	}
	return rows[:write]
}

func validCanonicalDynamicObservations(rows []Observation) bool {
	for index := range rows {
		if rows[index].Unit == (carrier.Unit{}) || index > 0 && !lessObservation(rows[index-1], rows[index]) {
			return false
		}
	}
	return true
}

func sameCanonicalObservations(left, right []Observation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !sameObservation(left[index], right[index]) {
			return false
		}
	}
	return true
}

// Replace atomically exchanges a Group's current exact read relation. A
// normal replacement must carry every declared static read; the remaining
// observations must be authorized dynamic exact routes. Replace(nil) is the
// sole reset spelling and removes both static and sparse dynamic subscriptions.
func (epoch *Epoch) Replace(group int, reads []Observation) bool {
	if epoch == nil || group < 0 {
		return false
	}
	epoch.mu.Lock()
	defer epoch.mu.Unlock()
	if !epoch.live || epoch.plan == nil || group >= len(epoch.groups) || group >= len(epoch.plan.readGroups) || group >= len(epoch.dynamicByGroup) || group >= len(epoch.selected) || !epoch.selected[group] {
		return false
	}
	if !epoch.prepareGroupWrite(group) {
		return false
	}
	index := epoch.plan.readGroups[group]
	state := &epoch.groups[group]
	if index.members == nil {
		return false
	}

	nextStatic := reads != nil
	nextDynamic := state.dynamicScratch[:0]
	if reads != nil {
		stamp := state.nextStaticEpoch()
		if stamp == 0 || len(state.staticSeen) != len(index.initial) {
			return false
		}
		for _, observation := range reads {
			if position, static := index.members[observation]; static {
				if position < 0 || position >= len(index.initial) {
					return false
				}
				state.staticSeen[position] = stamp
				continue
			}
			if _, allowed := epoch.dynamicSourceKey(group, observation); !allowed {
				return false
			}
			nextDynamic = append(nextDynamic, observation)
		}
		for position := range index.initial {
			if state.staticSeen[position] != stamp {
				return false
			}
		}
		nextDynamic = canonicalDynamicObservations(nextDynamic)
	}

	oldDynamic := epoch.dynamicByGroup[group]
	if !validCanonicalDynamicObservations(oldDynamic) || !validCanonicalDynamicObservations(nextDynamic) {
		return false
	}
	// Preserve any grown detached buffer even when the candidate relation is
	// semantically unchanged. It can never alias oldDynamic while that slice is
	// live, and retaining it is what makes a warmed stable Replace allocation
	// free rather than reallocating the same staging capacity every call.
	state.dynamicScratch = nextDynamic[:0]
	dynamicChanged := !sameCanonicalObservations(oldDynamic, nextDynamic)
	if dynamicChanged && !epoch.prepareDynamicMapWrite() {
		return false
	}
	if !dynamicChanged && state.static == nextStatic {
		// The common warmed path: static membership is immutable and every
		// discovered exact route is unchanged. Validation above used only
		// detached epoch-owned scratch, so there is no allocation or commit.
		return true
	}

	var updates []dynamicRouteUpdate
	if dynamicChanged {
		var oldKeys, newKeys []unitKey
		var oldKeysOK, newKeysOK bool
		oldKeys, oldKeysOK = epoch.dynamicKeysInto(group, oldDynamic, state.oldKeyScratch)
		newKeys, newKeysOK = epoch.dynamicKeysInto(group, nextDynamic, state.newKeyScratch)
		if !oldKeysOK || !newKeysOK {
			return false
		}
		state.oldKeyScratch, state.newKeyScratch = oldKeys, newKeys
		var staged bool
		updates, staged = epoch.stageDynamicReplace(group, oldKeys, newKeys, state.routeScratch)
		if !staged {
			return false
		}
		state.routeScratch = updates
	}
	if !epoch.validStaticToggle(group, state.static, nextStatic) {
		return false
	}

	// Every failure-prone condition is now proved. Commit static and dynamic
	// sides as one logical relation replacement; this block has no rejection
	// branch, so a failed call above leaves both active relations unchanged.
	epoch.applyStaticToggle(group, state.static, nextStatic)
	for _, update := range updates {
		if update.present {
			epoch.dynamicBySource[update.key] = update.groups
		} else {
			delete(epoch.dynamicBySource, update.key)
		}
	}
	if dynamicChanged {
		epoch.dynamicByGroup[group] = nextDynamic
		// The previous live slice becomes the next detached stage buffer only
		// after the replacement is committed. It is never written while live.
		state.dynamicScratch = oldDynamic[:0]
	}
	state.static = nextStatic
	return true
}

func (epoch *Epoch) activeObservationCount(group int) int {
	if epoch == nil || group < 0 {
		return 0
	}
	epoch.mu.RLock()
	defer epoch.mu.RUnlock()
	if !epoch.live || epoch.plan == nil || group >= len(epoch.groups) || group >= len(epoch.dynamicByGroup) {
		return 0
	}
	static := 0
	if epoch.groups[group].static && group < len(epoch.plan.readGroups) {
		static = len(epoch.plan.readGroups[group].initial)
	}
	return static + len(epoch.dynamicByGroup[group])
}

func (epoch *Epoch) activeObservation(group, index int) (Observation, bool) {
	if epoch == nil || group < 0 || index < 0 {
		return Observation{}, false
	}
	epoch.mu.RLock()
	defer epoch.mu.RUnlock()
	if !epoch.live || epoch.plan == nil || group >= len(epoch.groups) || group >= len(epoch.dynamicByGroup) {
		return Observation{}, false
	}
	static := 0
	if epoch.groups[group].static && group < len(epoch.plan.readGroups) {
		static = len(epoch.plan.readGroups[group].initial)
	}
	if index >= static {
		dynamic := index - static
		if dynamic < 0 || dynamic >= len(epoch.dynamicByGroup[group]) {
			return Observation{}, false
		}
		return epoch.dynamicByGroup[group][dynamic], true
	}
	if group >= len(epoch.plan.readGroups) || index >= len(epoch.plan.readGroups[group].initial) {
		return Observation{}, false
	}
	declaration := epoch.plan.readGroups[group].initial[index]
	if declaration < 0 || declaration >= len(epoch.plan.readDecls) {
		return Observation{}, false
	}
	return epoch.plan.readDecls[declaration].observation, true
}

type Reason uint8

const (
	ChangedUnit Reason = iota + 1
	ChangedFactor
	SupportAdded
	SupportRemoved
	AuthorshipChanged
)

type Wake struct {
	Group  int
	Reason Reason
	Unit   carrier.Unit
	Slot   shape.Slot
	Region support.Mask
}

// appendCurrentUnitWakes reads the epoch's exact active inverse bitset. The
// Plan source CSR gives canonical dense Group order; InitialReads are never
// consulted after Open.
func (epoch *Epoch) visitCurrentUnitWakes(point int, unit carrier.Unit, region support.Mask, visit func(Wake) bool) bool {
	if epoch == nil || epoch.plan == nil || visit == nil {
		return false
	}
	source, found := epoch.plan.sourceAt[unitKey{point: point, unit: unit}]
	if !found || source < 0 || source >= len(epoch.plan.sources) {
		return true
	}
	row := epoch.plan.sources[source]
	if row.begin < 0 || row.end < row.begin || row.end > len(epoch.plan.edges) {
		return true
	}
	firstWord, lastWord := row.begin>>6, (row.end-1)>>6
	for word := firstWord; word <= lastWord; word++ {
		active := epoch.edgeBits[word]
		if word == firstWord {
			active &= ^uint64(0) << uint(row.begin&63)
		}
		if word == lastWord && row.end&63 != 0 {
			active &= (uint64(1) << uint(row.end&63)) - 1
		}
		for active != 0 {
			bit := bits.TrailingZeros64(active)
			edge := word*64 + bit
			if !visit(Wake{Group: epoch.plan.edges[edge].group, Reason: ChangedUnit, Unit: unit, Region: region}) {
				return false
			}
			active &= active - 1
		}
	}
	return true
}

func (epoch *Epoch) appendCurrentUnitWakes(result []Wake, point int, unit carrier.Unit, region support.Mask) []Wake {
	_ = epoch.visitCurrentUnitWakes(point, unit, region, func(wake Wake) bool {
		result = append(result, wake)
		return true
	})
	return result
}

// visitDynamicUnitWakes reaches only the current epoch-local sparse inverse
// for a selected exact Unit. It deliberately carries the publication region
// unchanged: route membership is exact, while guard qualification remains the
// Product/solver's authority rather than a second demand approximation.
func (epoch *Epoch) visitDynamicUnitWakes(point int, unit carrier.Unit, region support.Mask, visit func(Wake) bool) bool {
	if epoch == nil || epoch.dynamicBySource == nil || visit == nil {
		return false
	}
	groups, found := epoch.dynamicBySource[unitKey{point: point, unit: unit}]
	if !found {
		return true
	}
	if !validSortedGroups(groups) {
		return false
	}
	for _, group := range groups {
		if !visit(Wake{Group: group, Reason: ChangedUnit, Unit: unit, Region: region}) {
			return false
		}
	}
	return true
}

func (plan *Plan) findCarry(point int, slot shape.Slot) (carryCSR, bool) {
	index := sort.Search(len(plan.carryRows), func(index int) bool {
		row := plan.carryRows[index]
		return row.point > point || row.point == point && row.slot >= slot
	})
	if index >= len(plan.carryRows) || plan.carryRows[index].point != point || plan.carryRows[index].slot != slot {
		return carryCSR{}, false
	}
	return plan.carryRows[index], true
}

// RoutePoint uses Graph consumer CSR for structural changes and immutable
// typed/carry CSR slices for value changes. Every Wake names a dense Group.
func (epoch *Epoch) RoutePoint(pointIndex int, set carrier.ChangeSet) ([]Wake, bool) {
	if epoch == nil || pointIndex < 0 {
		return nil, false
	}
	epoch.mu.Lock()
	defer epoch.mu.Unlock()
	if !epoch.live || epoch.plan == nil || epoch.plan.runtime == nil || pointIndex >= epoch.plan.graph.PointCount() || !epoch.plan.runtime.OwnsChangeSet(set) {
		return nil, false
	}
	point, ok := epoch.plan.graph.PointAt(schedule.Node(pointIndex))
	if !ok || !epoch.plan.graph.OwnsPoint(point) {
		return nil, false
	}
	epoch.wakes = epoch.wakes[:0]
	result := epoch.wakes
	marker := epoch.nextWakeEpoch()
	appendWake := func(wake Wake) bool {
		if wake.Group < 0 || wake.Group >= len(epoch.wakeSeen) {
			return false
		}
		if epoch.wakeSeen[wake.Group] == marker {
			return true
		}
		epoch.wakeSeen[wake.Group] = marker
		result = append(result, wake)
		return true
	}
	appendConsumers := func(reason Reason, region support.Mask) bool {
		if !region.Valid() || support.Empty(region) {
			return true
		}
		for index := 0; index < epoch.plan.graph.ConsumerCount(point); index++ {
			group, ok := epoch.plan.graph.ConsumerAt(point, index)
			groupIndex, indexed := epoch.plan.graph.GroupIndex(group)
			if !ok || !indexed || groupIndex < 0 || groupIndex >= len(epoch.selected) {
				return false
			}
			if epoch.selected[groupIndex] {
				if !appendWake(Wake{Group: groupIndex, Reason: reason, Region: region}) {
					return false
				}
			}
		}
		return true
	}
	if !appendConsumers(SupportAdded, set.Added()) || !appendConsumers(SupportRemoved, set.Removed()) {
		return nil, false
	}
	for index := 0; index < set.Count(); index++ {
		row, ok := set.At(index)
		if !ok || !row.Region().Valid() || support.Empty(row.Region()) {
			return nil, false
		}
		if !epoch.visitCurrentUnitWakes(pointIndex, row.Unit(), row.Region(), appendWake) {
			return nil, false
		}
		if !epoch.visitDynamicUnitWakes(pointIndex, row.Unit(), row.Region(), appendWake) {
			return nil, false
		}
	}
	for index := 0; index < set.FactorCount(); index++ {
		row, ok := set.FactorAt(index)
		if !ok || !epoch.plan.ownsSlot(row.Slot()) || !row.Region().Valid() || support.Empty(row.Region()) {
			return nil, false
		}
		if bucket, found := epoch.plan.findCarry(pointIndex, row.Slot()); found {
			for _, group := range epoch.plan.carryGroups[bucket.begin:bucket.end] {
				if group < 0 || group >= len(epoch.selected) || !epoch.selected[group] {
					continue
				}
				if !appendWake(Wake{Group: group, Reason: ChangedFactor, Slot: row.Slot(), Region: row.Region()}) {
					return nil, false
				}
			}
		}
	}
	epoch.wakes = result
	return result, true
}

// RouteCoverage wakes only declared whole-Factor carries whose exact
// authorship relation changed. It is deliberately separate from RoutePoint:
// coverage-only changes are structural fold evidence and must not fabricate a
// semantic FactorRegion or wake typed value reads.
func (epoch *Epoch) RouteCoverage(pointIndex int, set carrier.CoverageChangeSet) ([]Wake, bool) {
	if epoch == nil || pointIndex < 0 {
		return nil, false
	}
	epoch.mu.Lock()
	defer epoch.mu.Unlock()
	if !epoch.live || epoch.plan == nil || epoch.plan.runtime == nil || pointIndex >= epoch.plan.graph.PointCount() || !epoch.plan.runtime.OwnsCoverageChangeSet(set) {
		return nil, false
	}
	point, ok := epoch.plan.graph.PointAt(schedule.Node(pointIndex))
	if !ok || !epoch.plan.graph.OwnsPoint(point) {
		return nil, false
	}
	epoch.coverageWakes = epoch.coverageWakes[:0]
	result := epoch.coverageWakes
	marker := epoch.nextWakeEpoch()
	for index := 0; index < set.Count(); index++ {
		row, rowOK := set.At(index)
		if !rowOK || !epoch.plan.ownsSlot(row.Slot()) || !row.Region().Valid() || support.Empty(row.Region()) {
			return nil, false
		}
		bucket, found := epoch.plan.findCarry(pointIndex, row.Slot())
		if !found {
			continue
		}
		for _, group := range epoch.plan.carryGroups[bucket.begin:bucket.end] {
			if group < 0 || group >= len(epoch.wakeSeen) {
				return nil, false
			}
			if group >= len(epoch.selected) || !epoch.selected[group] {
				continue
			}
			if epoch.wakeSeen[group] == marker {
				continue
			}
			epoch.wakeSeen[group] = marker
			result = append(result, Wake{Group: group, Reason: AuthorshipChanged, Slot: row.Slot(), Region: row.Region()})
		}
	}
	epoch.coverageWakes = result
	return result, true
}

func (epoch *Epoch) Discard() bool {
	if epoch == nil {
		return false
	}
	epoch.mu.Lock()
	defer epoch.mu.Unlock()
	if !epoch.live {
		return true
	}
	// Widened epochs may share immutable live-route slices until their first
	// copy-on-write Replace. Drop only this epoch's outer authority; clearing a
	// shared backing slice would corrupt the successor being installed.
	epoch.dynamicByGroup = nil
	epoch.dynamicBySource = nil
	clear(epoch.wakes)
	epoch.wakes = nil
	clear(epoch.coverageWakes)
	epoch.coverageWakes = nil
	epoch.live = false
	return true
}
func (epoch *Epoch) Live() bool {
	if epoch == nil {
		return false
	}
	epoch.mu.RLock()
	defer epoch.mu.RUnlock()
	return epoch.live
}
