// Package index owns Heap's exact index receiver topology and its narrow
// owner-fenced index transfer judgments. It converts an existing Heap
// projection of a typed candidate and Value/Call facts into a finite set of
// possible Heap-root routes without introducing a second Heap access identity.
package index

import (
	"sync"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// Topology is the one cold, Link-scoped receiver-to-root support authority
// for Heap index operations. static contains the table roots whose runtime
// kind is structurally known. Fresh roots deliberately remain separate: their
// table/function kind is a guarded Call fact, not a property of the root.
//
// Neither slice contains a selected-operation×root relation. Fresh routes are
// conservatively recovered from their existing source Application and current
// Call fact.
type Topology struct {
	heap     heapdomain.Schema
	values   *valuedomain.Schema
	calls    *calldomain.Algebra
	boundary linkboundary.Values
	linkID   keyspace.ContentID

	static      []rootRole
	fresh       []freshRoot
	freshByRoot map[heapdomain.Key]uint32
	freshApps   []freshApplication
	freshByApp  map[linkproject.Application]uint32
	scratch     sync.Pool
}

type rootRole struct {
	key  heapdomain.Key
	role materialization.Role
}

type freshRoot struct {
	key         heapdomain.Key
	application linkproject.Application
	tag         uint32
}

// freshApplication groups only Link's intrinsic root-to-source-application
// relation. It deliberately stores no operation-selection relation. tag is
// the canonical, topology-local grouping coordinate for this one group.
type freshApplication struct {
	application linkproject.Application
	tag         uint32
}

// routeScratch is reusable implementation storage, never semantic State.
// emitted deduplicates widened Root routes. demanded is a generation-marked
// table indexed by topology-local fresh-application tag, so exact Call demand
// neither scans nor clears the complete fresh-application universe. Neither
// collection names a Factor coordinate or survives one observation.
type routeScratch struct {
	emitted     map[rootRole]struct{}
	demanded    []uint64
	demandEpoch uint64
}

func (scratch *routeScratch) nextDemandEpoch() uint64 {
	scratch.demandEpoch++
	if scratch.demandEpoch == 0 {
		clear(scratch.demanded)
		scratch.demandEpoch = 1
	}
	return scratch.demandEpoch
}

// Index is one exact existing Heap candidate projection. It retains only its
// canonical receiver/result/dynamic-key Value coordinates and Heap key
// partition. Root selection is always performed by Topology; no
// candidate×root relation is retained here or elsewhere.
type Index struct {
	topology    *Topology
	indexAccess heapdomain.IndexAccess
	receiver    valuedomain.Coordinate
	result      valuedomain.Coordinate
	dynamicKey  valuedomain.Coordinate
	slot        heapdomain.Slot
	id          keyspace.ContentID
}

// Access is retained as the index package's rule operand name. It is an alias
// of the topology projection, not a second Heap schema authority.
type Access = Index

// RouteKind distinguishes a tracked Heap root from an untracked possible
// table and an ordinary non-table alternative. Unknown is semantically
// different from Other: it retains a possible table operation whose root is
// not represented in Heap. Other proves only that this alternative cannot
// contribute a table root.
type RouteKind uint8

const (
	RouteInvalid RouteKind = iota
	RouteRoot
	RouteUnknown
	RouteOther
)

// Route is one observation result. Root routes carry a single root-role pair;
// unknown and other carry no fabricated Heap key or materialization role.
type Route struct {
	topology *Topology
	kind     RouteKind
	key      heapdomain.Key
	role     materialization.Role
}

func (route Route) Kind() RouteKind { return route.kind }

func (route Route) Root() (heapdomain.Key, materialization.Role, bool) {
	if route.topology == nil || route.kind != RouteRoot || !route.role.Valid() ||
		!route.topology.heap.Admits(route.key, route.topology.heap.Default()) {
		return heapdomain.Key{}, materialization.Invalid, false
	}
	if _, ok := route.topology.heap.Reference(route.key, route.role); !ok {
		return heapdomain.Key{}, materialization.Invalid, false
	}
	return route.key, route.role, true
}

// HeapState preserves the critical distinction between an absent Heap fact
// and Heap.Top. Consumers must not use an empty fact traversal as a Top test.
// Exact includes every non-bottom, non-top normalized Heap relation.
type HeapState uint8

const (
	HeapStateInvalid HeapState = iota
	HeapStateNone
	HeapStateExact
	HeapStateTop
)

func (topology *Topology) HeapState(key heapdomain.Key, value heapdomain.Value) HeapState {
	if topology == nil || !topology.valid() || !topology.heap.Admits(key, value) {
		return HeapStateInvalid
	}
	if value.IsTop() {
		return HeapStateTop
	}
	if value.IsBottom() {
		return HeapStateNone
	}
	return HeapStateExact
}

// Seal creates the sole topology authority from already-sealed Factor
// schemas. It is deliberately a cold O(R) construction where R is the Heap
// root support. Hot exact observation scans receiver atoms and selected fresh
// rows only; it never scans every Heap root or materializes candidate×root.
func Seal(heap heapdomain.Schema, values *valuedomain.Schema, calls *calldomain.Algebra) (*Topology, bool) {
	if values == nil || calls == nil || !values.OwnsHeapSchema(heap) || values.Link() == nil || calls.Link() != values.Link() || !heap.ContentID().Available() ||
		heap.LinkContentID() != values.Link().ContentID() || calls.LinkID() != values.Link().ContentID() {
		return nil, false
	}
	linked := values.Link()
	if linked.Boundary() == nil {
		return nil, false
	}
	topology := &Topology{heap: heap, values: values, calls: calls, boundary: linked.Boundary().Values(), linkID: linked.ContentID(), freshByRoot: make(map[heapdomain.Key]uint32), freshByApp: make(map[linkproject.Application]uint32)}
	if !topology.build() || !topology.valid() {
		return nil, false
	}
	// The support is fixed after Seal. Seed reusable route and exact-demand
	// dedup storage now; sync.Pool may be reclaimed, so this is a steady-state
	// allocation envelope, not a semantic or unconditional allocation guarantee.
	topology.scratch.New = func() any {
		return &routeScratch{
			emitted:  make(map[rootRole]struct{}, len(topology.static)+len(topology.fresh)*2),
			demanded: make([]uint64, len(topology.freshApps)),
		}
	}
	topology.scratch.Put(&routeScratch{
		emitted:  make(map[rootRole]struct{}, len(topology.static)+len(topology.fresh)*2),
		demanded: make([]uint64, len(topology.freshApps)),
	})
	return topology, true
}

func (topology *Topology) valid() bool {
	return topology != nil && topology.values != nil && topology.values.Link() != nil && topology.calls != nil &&
		topology.boundary.Count() == topology.values.CoordinateCount() &&
		topology.values.OwnsHeapSchema(topology.heap) && topology.calls.Link() == topology.values.Link() && topology.linkID.Available() && topology.heap.ContentID().Available() &&
		topology.heap.LinkContentID() == topology.linkID && topology.values.Link().ContentID() == topology.linkID &&
		topology.calls.LinkID() == topology.linkID
}

func (topology *Topology) build() bool {
	for index := 0; index < topology.heap.KeyCount(); index++ {
		key, ok := topology.heap.KeyAt(index)
		if !ok {
			return false
		}
		switch key.Kind() {
		case heapdomain.RootBoot:
			if !topology.appendStatic(key) {
				return false
			}
		case heapdomain.RootAllocation:
			if _, _, kind, source := key.ProgramAllocation(); source {
				if kind == heapdomain.AllocationTable && !topology.appendStatic(key) {
					return false
				}
				continue
			}
			application, _, _, _, _, _, fresh := key.FreshResult()
			if !fresh {
				return false
			}
			// Fresh rows are valid only when their source application is an
			// existing Call key. Their nominal kind is operation-dependent and
			// intentionally not read through a Link selection bridge.
			if _, ok := topology.calls.KeyForApplication(application); !ok {
				return false
			}
			if topology.freshByRoot[key] != 0 {
				return false
			}
			appIndex := topology.freshByApp[application]
			if appIndex == 0 {
				if uint64(len(topology.freshApps)) == uint64(^uint32(0)) {
					return false
				}
				topology.freshApps = append(topology.freshApps, freshApplication{application: application, tag: uint32(len(topology.freshApps) + 1)})
				appIndex = uint32(len(topology.freshApps))
				topology.freshByApp[application] = appIndex
			}
			group := &topology.freshApps[appIndex-1]
			if group.tag == 0 {
				return false
			}
			topology.fresh = append(topology.fresh, freshRoot{key: key, application: application, tag: group.tag})
			topology.freshByRoot[key] = uint32(len(topology.fresh))
		default:
			return false
		}
	}
	return true
}

func (topology *Topology) appendStatic(key heapdomain.Key) bool {
	added := false
	for _, role := range []materialization.Role{materialization.Exact, materialization.Recent, materialization.Summary} {
		if _, ok := topology.heap.Reference(key, role); ok {
			topology.static = append(topology.static, rootRole{key: key, role: role})
			added = true
		}
	}
	return added
}

// Access returns the exact existing Heap candidate row. All geometry comes
// from the sealed Heap row; this path never reopens Flow or reconstructs a
// Lens.
func (topology *Topology) Access(indexAccess heapdomain.IndexAccess) (Access, bool) {
	if topology == nil || !topology.valid() {
		return Access{}, false
	}
	id, idOK := topology.heap.IndexAccessID(indexAccess)
	geometry, geometryOK := topology.heap.IndexAccessGeometry(indexAccess)
	base, baseOK := topology.boundary.Of(geometry.Shard, geometry.Base)
	receiver, receiverOK := topology.values.CoordinateFor(base)
	slot, slotOK := topology.heap.SlotForIndexAccess(indexAccess)
	if !idOK || !id.Available() || !geometryOK || !baseOK || !receiverOK || !slotOK || (geometry.ReadTerm == 0) == (geometry.WriteTerm == 0) {
		return Access{}, false
	}
	resultAccess := Access{topology: topology, indexAccess: indexAccess, receiver: receiver, slot: slot, id: id}
	if geometry.ReadTerm != 0 {
		resultValue, resultValueOK := topology.heap.IndexAccessResult(indexAccess)
		result, resultOK := topology.values.CoordinateFor(resultValue)
		if !resultValueOK || !resultOK {
			return Access{}, false
		}
		resultAccess.result = result
	}
	if keyspace.TermFamily(geometry.Lens) == keyspace.FamilyLensKey {
		dynamic, dynamicOK := topology.boundary.Of(geometry.Shard, geometry.KeyTerm)
		coordinate, coordinateOK := topology.values.CoordinateFor(dynamic)
		if !dynamicOK || !coordinateOK {
			return Access{}, false
		}
		resultAccess.dynamicKey = coordinate
	}
	return resultAccess, true
}

// AccessFor resolves one exact mounted Program index-access occurrence through
// Heap's occurrence inverse and then issues this Topology's canonical Access.
// It never enumerates Heap candidates and never reconstitutes geometry in the
// index package; the supplied Program/Shard/term must first pass Heap's owner
// and executable-geometry fence.
func (topology *Topology) AccessFor(shard linkproject.Shard, owner *program.Program, occurrence keyspace.Term) (Access, bool) {
	if topology == nil || !topology.valid() {
		return Access{}, false
	}
	indexAccess, ok := topology.heap.IndexAccessFor(shard, owner, occurrence)
	if !ok {
		return Access{}, false
	}
	return topology.Access(indexAccess)
}

// OwnsAccess is the topology ownership fence for one retained index operand.
// A valid Access from a duplicate same-content Topology is still foreign:
// the issuing Topology pointer is part of the semantic owner boundary. The
// canonical reissue also verifies every derived coordinate and policy field,
// so a matching portable ID cannot substitute for ownership.
func (topology *Topology) OwnsAccess(access Access) bool {
	if topology == nil || access.topology != topology || !topology.valid() {
		return false
	}
	canonical, ok := topology.Access(access.indexAccess)
	return ok && canonical == access
}

func (access Access) valid() bool {
	if access.topology == nil || !access.topology.valid() || !access.id.Available() {
		return false
	}
	canonical, ok := access.topology.Access(access.indexAccess)
	return ok && canonical.receiver == access.receiver && canonical.result == access.result && canonical.dynamicKey == access.dynamicKey && canonical.slot == access.slot && canonical.id == access.id
}

func (access Access) IndexAccess() (heapdomain.IndexAccess, bool) {
	if !access.valid() {
		return heapdomain.IndexAccess{}, false
	}
	return access.indexAccess, true
}
func (access Access) Receiver() (valuedomain.Coordinate, bool) {
	if !access.valid() {
		return valuedomain.Coordinate{}, false
	}
	return access.receiver, true
}

// Result returns the fixed result Value coordinate for a typed Read candidate.
// Write candidates have no result coordinate and fail closed, so a Rule cannot
// turn an assignment source into an index-read result.
func (access Access) Result() (valuedomain.Coordinate, bool) {
	if !access.valid() || !access.result.Valid() {
		return valuedomain.Coordinate{}, false
	}
	return access.result, true
}

// DynamicKey returns the existing Value coordinate for a dynamic heap key.
// Exact-key lenses return false: their selector is already carried by Slot,
// so no synthetic literal Value coordinate is introduced.
func (access Access) DynamicKey() (valuedomain.Coordinate, bool) {
	if !access.valid() || !access.dynamicKey.Valid() {
		return valuedomain.Coordinate{}, false
	}
	return access.dynamicKey, true
}

// Read reports whether the Heap candidate row is a typed Read. Raw-get
// admission is based solely on this row membership; no operation enum is
// reclassified at query time.
func (access Access) Read() bool {
	if !access.valid() {
		return false
	}
	geometry, ok := access.topology.heap.IndexAccessGeometry(access.indexAccess)
	return ok && geometry.ReadTerm != 0 && geometry.WriteTerm == 0
}

// Write reports whether this existing Heap candidate row is a typed indexed
// write.  Write rows deliberately retain no result coordinate; mutation rules
// must use the sealed RHS Payload rather than treating a write as a read.
func (access Access) Write() bool {
	if !access.valid() {
		return false
	}
	geometry, ok := access.topology.heap.IndexAccessGeometry(access.indexAccess)
	return ok && geometry.ReadTerm == 0 && geometry.WriteTerm != 0
}

func (access Access) Slot() (heapdomain.Slot, bool) {
	if !access.valid() {
		return heapdomain.Slot{}, false
	}
	return access.slot, true
}
func (access Access) ID() (keyspace.ContentID, bool) {
	if !access.valid() {
		return keyspace.ContentID{}, false
	}
	return access.id, true
}

// CallState obtains the currently solved Call fact for one exact source
// application. tag is the nonzero, topology-local fresh-application tag from
// VisitReceiverCallDemand. It lets the caller feed the selected fact back to
// this same observation without reconstructing a Call-to-receiver relation.
// It intentionally has no default: missing state produces an unknown route
// rather than pretending an opaque fresh root is absent.
type CallState func(key calldomain.Key, tag uint64) (calldomain.Value, bool)

// VisitReceiverCallDemand emits the exact Call facts that a receiver needs
// before its guarded fresh-root routes can be observed. Only a rooted opaque
// fresh allocation with an admitted Heap materialization can demand Call
// state; static tables, unsupported materializations, non-reference values,
// and unrooted opaque alternatives do not. The tag is a one-based canonical
// fresh-application group position for this sealed Topology only. It is a
// transient selection correlation, never a Factor coordinate, Program/Link
// identity, or persisted value.
//
// Value.Top demands every fresh application group exactly once. An exact
// Value emits each group at most once even if several fresh roots or roles of
// the same source application occur in its atom relation. Invalid/foreign
// inputs fail closed; Bottom is valid and emits no demand.
func (topology *Topology) VisitReceiverCallDemand(receiver valuedomain.Value, visit func(calldomain.Key, uint64) bool) bool {
	if topology == nil || !topology.valid() || visit == nil {
		return false
	}
	if receiver.IsTop() && !topology.values.Equal(receiver, topology.values.Top()) {
		return false
	}
	if receiver.IsBottom() {
		return topology.values.Equal(receiver, topology.values.Bottom())
	}
	if receiver.IsTop() {
		for _, group := range topology.freshApps {
			if group.tag == 0 {
				return false
			}
			key, ok := topology.calls.KeyForApplication(group.application)
			if !ok {
				return false
			}
			if !visit(key, uint64(group.tag)) {
				return true
			}
		}
		return true
	}

	scratch := topology.scratch.Get().(*routeScratch)
	defer topology.scratch.Put(scratch)
	epoch := scratch.nextDemandEpoch()
	return topology.values.VisitAtoms(receiver, func(atom valuedomain.Atom) bool {
		reference, role, rooted := atom.Reference()
		if !rooted || reference.Kind() != valuedomain.ReferenceOpaque {
			return true
		}
		root, ok := reference.AllocationKey()
		if !ok {
			return true
		}
		fresh, ok := topology.freshFor(root)
		if !ok || fresh.tag == 0 {
			return false
		}
		// This exact materialization has no Heap containment reference, so
		// VisitReceiver will emit Unknown before consulting Call. Do not create
		// a spurious dynamic read for a fact that cannot affect the route.
		if topology.heapReferenceMissing(fresh.key, role) {
			return true
		}
		index := int(fresh.tag - 1)
		if index < 0 || index >= len(scratch.demanded) {
			return false
		}
		if scratch.demanded[index] == epoch {
			return true
		}
		key, ok := topology.calls.KeyForApplication(fresh.application)
		if !ok {
			return false
		}
		scratch.demanded[index] = epoch
		return visit(key, uint64(fresh.tag))
	})
}

// VisitReceiver observes every possible index receiver route. It emits each
// root-role at most once, followed at most once by Unknown and Other. It does
// not inspect Heap facts; HeapState classifies those separately so callers can
// never conflate Heap.Top with an absent root state.
func (topology *Topology) VisitReceiver(receiver valuedomain.Value, callState CallState, visit func(Route) bool) bool {
	if topology == nil || !topology.valid() || visit == nil {
		return false
	}
	// Top has no atom image, so VisitAtoms alone cannot provide its owner
	// fence. Exact and bottom Values are fenced by the corresponding branch.
	if receiver.IsTop() && !topology.values.Equal(receiver, topology.values.Top()) {
		return false
	}
	if receiver.IsBottom() {
		return topology.values.Equal(receiver, topology.values.Bottom())
	}
	if receiver.IsTop() {
		return topology.visitTopReceiver(callState, visit)
	}
	return topology.visitExactReceiver(receiver, callState, visit)
}

// visitExactReceiver is the hot observation path. Value's normalized atom
// image has no duplicate atom, and a rooted atom carries one role, so it does
// not need a root-role set. visitFresh suppresses a selector replay locally.
// Unknown/Other are the only global alternatives that can repeat here.
func (topology *Topology) visitExactReceiver(receiver valuedomain.Value, callState CallState, visit func(Route) bool) bool {
	unknown, other := false, false
	emit := func(route Route) bool {
		switch route.kind {
		case RouteRoot:
			return visit(route)
		case RouteUnknown:
			if unknown {
				return true
			}
			unknown = true
			return visit(route)
		case RouteOther:
			if other {
				return true
			}
			other = true
			return visit(route)
		default:
			return false
		}
	}
	return topology.values.VisitAtoms(receiver, func(atom valuedomain.Atom) bool {
		reference, role, rooted := atom.Reference()
		if !rooted {
			if atom.RuntimeKinds().Contains(runtimekind.Table) {
				return emit(topology.unknownRoute())
			}
			return emit(topology.otherRoute())
		}
		if reference.Kind() == valuedomain.ReferenceTable {
			key, ok := topology.keyForReference(reference)
			if !ok {
				return false
			}
			if _, ok := topology.heap.Reference(key, role); !ok {
				return emit(topology.unknownRoute())
			}
			return emit(topology.rootRoute(key, role))
		}
		if reference.Kind() != valuedomain.ReferenceOpaque {
			return emit(topology.otherRoute())
		}
		root, ok := reference.AllocationKey()
		if !ok {
			return emit(topology.unknownRoute())
		}
		fresh, ok := topology.freshFor(root)
		if !ok {
			return false
		}
		return topology.visitFresh(fresh, role, callState, emit)
	})
}

// visitTopReceiver handles a widened receiver with reusable owner-local
// scratch. Several selected candidates can independently justify one fresh
// root, so exact canonical output requires deduplication. The scratch is not
// semantic State; the pool can replenish it after collection.
func (topology *Topology) visitTopReceiver(callState CallState, visit func(Route) bool) bool {
	scratch := topology.scratch.Get().(*routeScratch)
	clear(scratch.emitted)
	defer topology.scratch.Put(scratch)
	unknown, other := false, false
	emit := func(route Route) bool {
		if route.kind == RouteRoot {
			root, role, ok := route.Root()
			if !ok {
				return false
			}
			identity := rootRole{key: root, role: role}
			if _, found := scratch.emitted[identity]; found {
				return true
			}
			scratch.emitted[identity] = struct{}{}
			return visit(route)
		}
		switch route.kind {
		case RouteUnknown:
			if unknown {
				return true
			}
			unknown = true
			return visit(route)
		case RouteOther:
			if other {
				return true
			}
			other = true
			return visit(route)
		default:
			return false
		}
	}
	for _, supported := range topology.static {
		if !emit(topology.rootRoute(supported.key, supported.role)) {
			return true
		}
	}
	for _, group := range topology.freshApps {
		if !topology.visitFreshApplication(group, callState, emit) {
			return true
		}
	}
	return emit(topology.unknownRoute()) && emit(topology.otherRoute())
}

func (topology *Topology) rootRoute(key heapdomain.Key, role materialization.Role) Route {
	return Route{topology: topology, kind: RouteRoot, key: key, role: role}
}
func (topology *Topology) unknownRoute() Route { return Route{topology: topology, kind: RouteUnknown} }
func (topology *Topology) otherRoute() Route   { return Route{topology: topology, kind: RouteOther} }

func (topology *Topology) keyForReference(reference valuedomain.Reference) (heapdomain.Key, bool) {
	if root, ok := reference.AllocationKey(); ok {
		if topology.heap.OwnsKey(root) {
			return root, true
		}
		return heapdomain.Key{}, false
	}
	if root, ok := reference.BootRoot(); ok {
		return topology.heap.KeyForBootRoot(root)
	}
	return heapdomain.Key{}, false
}

func (topology *Topology) freshFor(root heapdomain.Key) (freshRoot, bool) {
	index := topology.freshByRoot[root]
	if index == 0 || int(index) > len(topology.fresh) {
		return freshRoot{}, false
	}
	return topology.fresh[index-1], true
}

// visitFresh preserves only the Call fact's empty/non-empty distinction. Heap
// does not replay a selected target through Link, so any nonempty fresh source
// is conservatively an unknown table route.
func (topology *Topology) visitFresh(fresh freshRoot, role materialization.Role, callState CallState, emit func(Route) bool) bool {
	if !role.Valid() || topology.heapReferenceMissing(fresh.key, role) {
		return emit(topology.unknownRoute())
	}
	if fresh.tag == 0 {
		return false
	}
	key, ok := topology.calls.KeyForApplication(fresh.application)
	if !ok || callState == nil {
		return emit(topology.unknownRoute())
	}
	state, available := callState(key, uint64(fresh.tag))
	if !available || !topology.calls.Admits(key, state) {
		return emit(topology.unknownRoute())
	}
	if state.IsEmpty() {
		return true
	}
	return emit(topology.unknownRoute())
}

// visitFreshApplication is the Value.Top path. It retains only Link's direct
// root-to-Application edge; selected-operation reconstruction is forbidden.
func (topology *Topology) visitFreshApplication(group freshApplication, callState CallState, emit func(Route) bool) bool {
	key, ok := topology.calls.KeyForApplication(group.application)
	if !ok || callState == nil {
		return emit(topology.unknownRoute())
	}
	if group.tag == 0 {
		return false
	}
	state, available := callState(key, uint64(group.tag))
	if !available || !topology.calls.Admits(key, state) {
		return emit(topology.unknownRoute())
	}
	if state.IsEmpty() {
		return true
	}
	return emit(topology.unknownRoute())
}

func (topology *Topology) heapReferenceMissing(key heapdomain.Key, role materialization.Role) bool {
	_, ok := topology.heap.Reference(key, role)
	return !ok
}
