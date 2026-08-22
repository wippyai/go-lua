package contextfiber

import (
	"encoding/binary"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

const layoutOwnerDomain = "analysis/engine/internal/contextfiber/layout/v1"

// StateOrdinal is a compact state row in Layout. Global rows occupy the
// prefix; mounted rows then occupy one module-local block per canonical
// execution context. It is not the Cartesian FiberOrdinal from Index.
type StateOrdinal uint64

type modulePlan struct {
	points []PointOrdinal
	local  map[PointOrdinal]uint64
}

// Layout is the compact executable-state projection over one Index address
// identity. It retains one point-owner vector and module-local maps, plus a
// context prefix; it never allocates a ContextOrdinal×PointOrdinal table.
//
// A global point has one state row for the whole Link. A mounted point has a
// row only in contexts whose sealed ModuleKey equals the point's owner module.
// Consequently Layout is an eligibility/storage projection, not a claim that
// every dense Index fiber is executable.
type Layout struct {
	index      Index
	owner      identity.ContentID
	generation identity.Generation
	// graph is the exact immutable equation graph whose dense Point shape was
	// admitted into this layout.  A nil value is retained for contextfiber-only
	// layouts; Link execution plans require the graph-bound constructor and
	// therefore never infer graph authority from shape alone.
	graph *equation.Graph

	pointOwners   []PointOwner
	contextModule []identity.ContentID
	modules       map[identity.ContentID]modulePlan
	globals       []PointOrdinal
	globalLocal   map[PointOrdinal]uint64
	prefix        []uint64
	total         uint64
	// available is the projection verdict newLayout reached.  The value is
	// immutable, so accessor guards read a settled fact instead of re-proving
	// the construction; the zero Layout carries the false verdict.
	available bool
}

// StateCell is the inverse lookup result for one compact state row. Global
// cells intentionally have no ContextOrdinal: one global row is shared by all
// directory contexts, and returning a made-up context would turn storage
// identity into a false execution claim.
type StateCell struct {
	layoutOwner identity.ContentID
	generation  identity.Generation
	context     ContextOrdinal
	contextOK   bool
	point       PointOrdinal
	owner       PointOwner
	available   bool
}

// NewLayout validates and builds the compact executable-state layout for one
// already-issued Index. The point-owner vector must be aligned exactly to
// Index.PointCount, and generation must be the Index's exact revision fence.
//
// Admission is closed under two totality laws: every directory context module
// must own at least one mounted point, and every mounted point module must be
// represented by a directory context. Link-global owners must name exactly
// directory.LinkID. No default context or fallback owner is introduced.
func NewLayout(index Index, directory executioncontext.Directory, pointOwners []PointOwner, generation identity.Generation) (Layout, bool) {
	return newLayout(index, directory, pointOwners, generation, nil)
}

// NewLayoutForGraph constructs a compact layout fenced to one exact immutable
// equation Graph.  The graph pointer is an authority fence, not a derived
// digest: equal-shaped graphs and revision views from another topology are not
// interchangeable with this layout.
func NewLayoutForGraph(index Index, directory executioncontext.Directory, pointOwners []PointOwner, generation identity.Generation, graph *equation.Graph) (Layout, bool) {
	if graph == nil || graph.PointCount() == 0 || graph.PointCount() != index.PointCount() {
		return Layout{}, false
	}
	return newLayout(index, directory, pointOwners, generation, graph)
}

func newLayout(index Index, directory executioncontext.Directory, pointOwners []PointOwner, generation identity.Generation, graph *equation.Graph) (Layout, bool) {
	if !index.Available() || !directory.Available() || !generation.Available() || len(pointOwners) != index.PointCount() || !index.OwnedBy(directory, index.PointCount(), generation) {
		return Layout{}, false
	}
	if graph != nil && (graph.PointCount() == 0 || graph.PointCount() != index.PointCount()) {
		return Layout{}, false
	}
	if len(pointOwners) == 0 {
		return Layout{}, false
	}
	owners := append([]PointOwner(nil), pointOwners...)
	for _, owner := range owners {
		if !owner.Available() {
			return Layout{}, false
		}
		if owner.LinkGlobal() && owner.LinkID() != directory.LinkID() {
			return Layout{}, false
		}
	}

	contextModule := make([]identity.ContentID, index.ContextCount())
	contextModules := make(map[identity.ContentID]struct{}, len(contextModule))
	for ordinal := range contextModule {
		contextID, contextIDOK := index.ContextID(ContextOrdinal(ordinal))
		if !contextIDOK {
			return Layout{}, false
		}
		context, contextOK := directory.Context(contextID)
		if !contextOK || !context.Available() || context.LinkID() != directory.LinkID() || !context.ModuleKey().Available() {
			return Layout{}, false
		}
		contextModule[ordinal] = context.ModuleKey()
		contextModules[context.ModuleKey()] = struct{}{}
	}

	modules := make(map[identity.ContentID]modulePlan)
	globals := make([]PointOrdinal, 0)
	globalLocal := make(map[PointOrdinal]uint64)
	for ordinal, owner := range owners {
		point := PointOrdinal(ordinal)
		if owner.LinkGlobal() {
			globalLocal[point] = uint64(len(globals))
			globals = append(globals, point)
			continue
		}
		module := owner.ModuleKey()
		plan := modules[module]
		if plan.local == nil {
			plan.local = make(map[PointOrdinal]uint64)
		}
		plan.local[point] = uint64(len(plan.points))
		plan.points = append(plan.points, point)
		modules[module] = plan
	}

	// Every context module must have mounted state, and no mounted owner may
	// name a module absent from the sealed directory.
	for module := range contextModules {
		plan, ok := modules[module]
		if !ok || len(plan.points) == 0 {
			return Layout{}, false
		}
	}
	for module, plan := range modules {
		if len(plan.points) == 0 {
			return Layout{}, false
		}
		if _, ok := contextModules[module]; !ok {
			return Layout{}, false
		}
	}

	prefix := make([]uint64, len(contextModule)+1)
	prefix[0] = uint64(len(globals))
	for ordinal, module := range contextModule {
		plan := modules[module]
		next, ok := checkedAdd(prefix[ordinal], uint64(len(plan.points)))
		if !ok {
			return Layout{}, false
		}
		prefix[ordinal+1] = next
	}
	if prefix[len(contextModule)] == 0 {
		return Layout{}, false
	}
	owner, ok := deriveLayoutOwner(index, owners, generation)
	if !ok {
		return Layout{}, false
	}
	layout := Layout{
		index:         index,
		owner:         owner,
		generation:    generation,
		graph:         graph,
		pointOwners:   owners,
		contextModule: contextModule,
		modules:       modules,
		globals:       globals,
		globalLocal:   globalLocal,
		prefix:        prefix,
		total:         prefix[len(contextModule)],
	}
	layout.available = layout.completeProjection()
	return layout, layout.available
}

// Available reports whether layout is a complete compact state projection. The
// verdict is sealed by the constructor.
func (layout Layout) Available() bool { return layout.available }

func (layout Layout) completeProjection() bool {
	if !layout.index.Available() || !layout.owner.Available() || !layout.generation.Available() || len(layout.pointOwners) == 0 || len(layout.pointOwners) != layout.index.PointCount() || len(layout.contextModule) != layout.index.ContextCount() || len(layout.prefix) != len(layout.contextModule)+1 || layout.total == 0 {
		return false
	}
	if layout.graph != nil && (layout.graph.PointCount() == 0 || layout.graph.PointCount() != len(layout.pointOwners)) {
		return false
	}
	return layout.prefix[len(layout.prefix)-1] == layout.total
}

// Graph returns the exact immutable equation Graph fenced into this layout.
// Contextfiber-only layouts return nil; consumers that construct executable
// Link plans must reject that absence rather than infer ownership from shape.
func (layout Layout) Graph() *equation.Graph {
	if !layout.Available() {
		return nil
	}
	return layout.graph
}

// Generation returns the exact revision fenced into layout.
func (layout Layout) Generation() identity.Generation {
	if !layout.Available() {
		return 0
	}
	return layout.generation
}

// ContextCount reports the number of contexts in the backing Index.
func (layout Layout) ContextCount() int {
	if !layout.Available() {
		return 0
	}
	return len(layout.contextModule)
}

// ContextID resolves one canonical context ordinal back to the exact
// execution-context identity that owns its mounted block.  Layout keeps the
// Index address identity private, but later state-indexed adapters still need
// this inverse without reopening or reconstructing the Index.
func (layout Layout) ContextID(context ContextOrdinal) (identity.ContentID, bool) {
	if !layout.Available() {
		return identity.ContentID{}, false
	}
	return layout.index.ContextID(context)
}

// ContextModuleKey returns the sealed module owner of one context ordinal.
// It is a projection of the exact Directory/Index identity retained by the
// Layout; no default module or fallback owner is introduced.
func (layout Layout) ContextModuleKey(context ContextOrdinal) (identity.ContentID, bool) {
	if !layout.Available() || uint64(context) >= uint64(len(layout.contextModule)) {
		return identity.ContentID{}, false
	}
	module := layout.contextModule[context]
	if !module.Available() {
		return identity.ContentID{}, false
	}
	return module, true
}

// PointCount reports the dense point shape inherited from Index.
func (layout Layout) PointCount() int {
	if !layout.Available() {
		return 0
	}
	return len(layout.pointOwners)
}

// StateCount reports compact executable state cardinality. It is globals plus
// mounted points for each canonical context, never ContextCount×PointCount.
func (layout Layout) StateCount() StateOrdinal {
	if !layout.Available() {
		return 0
	}
	return StateOrdinal(layout.total)
}

// PointOwnerAt returns the authenticated owner for one dense point ordinal.
func (layout Layout) PointOwnerAt(point PointOrdinal) (PointOwner, bool) {
	if !layout.Available() || uint64(point) >= uint64(len(layout.pointOwners)) {
		return PointOwner{}, false
	}
	return layout.pointOwners[point], true
}

// Lookup maps an executable (ContextOrdinal, PointOrdinal) pair to one
// compact state row. Cross-module mounted pairs and all out-of-bounds pairs
// are refused; Link-global points resolve to the same row for every context.
func (layout Layout) Lookup(context ContextOrdinal, point PointOrdinal) (StateOrdinal, bool) {
	if !layout.Available() || uint64(context) >= uint64(len(layout.contextModule)) || uint64(point) >= uint64(len(layout.pointOwners)) {
		return 0, false
	}
	owner := layout.pointOwners[point]
	if owner.LinkGlobal() {
		local, ok := layout.globalLocal[point]
		return StateOrdinal(local), ok
	}
	if !owner.Mounted() || owner.ModuleKey() != layout.contextModule[context] {
		return 0, false
	}
	plan, ok := layout.modules[owner.ModuleKey()]
	if !ok {
		return 0, false
	}
	local, ok := plan.local[point]
	if !ok || uint64(context) >= uint64(len(layout.prefix)-1) {
		return 0, false
	}
	state, ok := checkedAdd(layout.prefix[context], local)
	if !ok || state >= layout.total {
		return 0, false
	}
	return StateOrdinal(state), true
}

// StateAt performs the inverse compact lookup. A global StateCell has no
// context ordinal because its row is shared by every context; mounted cells
// carry the exact canonical context that owns their module block.
func (layout Layout) StateAt(state StateOrdinal) (StateCell, bool) {
	if !layout.Available() || uint64(state) >= layout.total {
		return StateCell{}, false
	}
	flat := uint64(state)
	if flat < uint64(len(layout.globals)) {
		point := layout.globals[flat]
		return layout.sealCell(StateCell{layoutOwner: layout.owner, generation: layout.generation, point: point, owner: layout.pointOwners[point]}), true
	}
	context := sort.Search(len(layout.contextModule), func(index int) bool {
		return layout.prefix[index+1] > flat
	})
	if context < 0 || context >= len(layout.contextModule) || flat < layout.prefix[context] {
		return StateCell{}, false
	}
	local := flat - layout.prefix[context]
	plan, ok := layout.modules[layout.contextModule[context]]
	if !ok || local >= uint64(len(plan.points)) {
		return StateCell{}, false
	}
	point := plan.points[local]
	return layout.sealCell(StateCell{
		layoutOwner: layout.owner,
		generation:  layout.generation,
		context:     ContextOrdinal(context),
		contextOK:   true,
		point:       point,
		owner:       layout.pointOwners[point],
	}), true
}

// sealCell decides one inverse result's completeness where it is issued, so a
// cell accessor reads a settled fact rather than re-proving the layout's
// identity fence on every read.
func (layout Layout) sealCell(cell StateCell) StateCell {
	cell.available = cell.layoutOwner.Available() && cell.generation.Available() && cell.owner.Available()
	return cell
}

// OwnedBy proves that layout belongs to the exact Index owner, sealed
// Directory, point-owner vector, shape, and generation supplied by the caller.
// The Index remains the address identity; Layout adds only the compact
// eligibility/storage projection.
func (layout Layout) OwnedBy(index Index, directory executioncontext.Directory, pointOwners []PointOwner, generation identity.Generation) bool {
	if !layout.Available() || !index.Available() || !directory.Available() || !generation.Available() || len(pointOwners) != index.PointCount() || !index.OwnedBy(directory, index.PointCount(), generation) {
		return false
	}
	owner, ok := deriveLayoutOwner(index, pointOwners, generation)
	return ok && owner == layout.owner
}

// Available reports whether cell is a complete inverse result. A global cell
// is available even though ContextOrdinal is intentionally unavailable. The
// verdict is sealed where the cell is issued.
func (cell StateCell) Available() bool { return cell.available }

// ContextOrdinal returns the mounted context for this cell. It returns false
// for a Link-global row because globals are singular and context-independent.
func (cell StateCell) ContextOrdinal() (ContextOrdinal, bool) {
	if !cell.Available() || !cell.contextOK {
		return 0, false
	}
	return cell.context, true
}

// PointOrdinal returns the dense point ordinal represented by this cell.
func (cell StateCell) PointOrdinal() (PointOrdinal, bool) {
	if !cell.Available() {
		return 0, false
	}
	return cell.point, true
}

// Owner returns the authenticated owner represented by this cell.
func (cell StateCell) Owner() PointOwner {
	if !cell.Available() {
		return PointOwner{}
	}
	return cell.owner
}

// OwnedBy proves that a cell was issued by this exact Layout revision.
func (cell StateCell) OwnedBy(layout Layout) bool {
	return cell.Available() && layout.Available() && cell.layoutOwner == layout.owner && cell.generation == layout.generation
}

func checkedAdd(left, right uint64) (uint64, bool) {
	if left > math.MaxUint64-right {
		return 0, false
	}
	return left + right, true
}

func deriveLayoutOwner(index Index, pointOwners []PointOwner, generation identity.Generation) (identity.ContentID, bool) {
	if !index.Available() || !generation.Available() || len(pointOwners) != index.PointCount() {
		return identity.ContentID{}, false
	}
	parts := make([][]byte, 0, len(pointOwners)+3)
	parts = append(parts, index.owner[:])
	var generationBytes [8]byte
	binary.BigEndian.PutUint64(generationBytes[:], uint64(generation))
	parts = append(parts, generationBytes[:])
	var pointCountBytes [8]byte
	binary.BigEndian.PutUint64(pointCountBytes[:], uint64(len(pointOwners)))
	parts = append(parts, pointCountBytes[:])
	for _, owner := range pointOwners {
		if !owner.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, owner.id[:])
	}
	return identity.DeriveContentID(layoutOwnerDomain, parts...)
}
