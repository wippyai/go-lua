// Package index owns Heap's exact index receiver topology and its narrow
// owner-fenced index transfer judgments. It converts an existing Heap
// projection of a typed candidate and Value/Call facts into a finite set of
// possible Heap-root routes without introducing a second Heap access identity.
package index

import (
	"fmt"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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
	heap        heapdomain.Schema
	values      *valuedomain.Schema
	calls       *calldomain.Algebra
	packs       *pack.Schema
	catalog     *rawCatalog
	selectors   *keymatch.SelectorProjection
	static      []rawSetRouteFact
	fresh       []freshRoot
	freshByRoot map[heapdomain.Key]uint32
	freshApps   []freshApplication
	freshByApp  map[identity.ContentID]uint32
	// moduleExports is keyed only by the existing fresh-root Heap key. Each
	// row is an owner-issued Value proof for one composed require result; it
	// is not a selected-operation×root relation.
	moduleExports map[heapdomain.Key]moduleExportRoute
	indexRows     map[heapdomain.IndexAccess]*indexRow
	staticByTag   map[heapdomain.RawRouteTag]uint32
	scratch       sync.Pool
}

// SealFailure is the closed phase at which Heap index topology admission
// failed. It is diagnostic-only: no failure exposes a partial Topology or
// changes the ordinary Seal contract.
type SealFailure uint8

const (
	SealFailureNone SealFailure = iota
	SealFailureInputs
	SealFailureBoundary
	SealFailureRoots
	SealFailureValidity
)

func (failure SealFailure) String() string {
	switch failure {
	case SealFailureNone:
		return "none"
	case SealFailureInputs:
		return "inputs"
	case SealFailureBoundary:
		return "boundary"
	case SealFailureRoots:
		return "roots"
	case SealFailureValidity:
		return "validity"
	default:
		return "unknown"
	}
}

// SealDiagnostic names one closed failure and, where applicable, its mount
// and row. Negative coordinates mean the failure is not row-specific.
type SealDiagnostic struct {
	failure SealFailure
	mount   int
	row     int
}

func (diagnostic SealDiagnostic) Failure() SealFailure { return diagnostic.failure }
func (diagnostic SealDiagnostic) Mount() int           { return diagnostic.mount }
func (diagnostic SealDiagnostic) Row() int             { return diagnostic.row }
func (diagnostic SealDiagnostic) String() string {
	if diagnostic.failure == SealFailureNone {
		return SealFailureNone.String()
	}
	if diagnostic.mount < 0 {
		return diagnostic.failure.String()
	}
	if diagnostic.row < 0 {
		return fmt.Sprintf("%s:mount=%d", diagnostic.failure, diagnostic.mount)
	}
	return fmt.Sprintf("%s:mount=%d:row=%d", diagnostic.failure, diagnostic.mount, diagnostic.row)
}

func sealDiagnostic(failure SealFailure, mount, row int) SealDiagnostic {
	return SealDiagnostic{failure: failure, mount: mount, row: row}
}

type freshRoot struct {
	key           heapdomain.Key
	applicationID identity.ContentID
	tag           uint32
}

// freshApplication groups only Link's intrinsic root-to-source-application
// relation. It deliberately stores no operation-selection relation. tag is
// the canonical, topology-local grouping coordinate for this one group.
type freshApplication struct {
	applicationID identity.ContentID
	tag           uint32
}

type moduleExportRoute struct {
	operation vocabulary.Operation
	roots     []heapdomain.Key
}

// routeScratch is reusable implementation storage, never semantic State.
// emitted deduplicates widened Root routes. demanded is a generation-marked
// table indexed by topology-local fresh-application tag, so exact Call demand
// neither scans nor clears the complete fresh-application universe. Neither
// collection names a Factor coordinate or survives one observation.
type routeScratch struct {
	emitted     map[rawSetRouteFact]struct{}
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

// indexRow is the one immutable projection retained by Topology for a Heap
// candidate. Index carries only a pointer to this row; callers cannot forge a
// copied coordinate bundle.
type indexRow struct {
	topology    *Topology
	indexAccess heapdomain.IndexAccess
	receiver    valuedomain.Coordinate
	result      valuedomain.Coordinate
	dynamicKey  valuedomain.Coordinate
	slot        heapdomain.Slot
	id          identity.ContentID
}

// Index is one exact existing Heap candidate projection. Its embedded row is
// private but keeps the historical field access internal to this package;
// pointer identity is the owner fence and preserves comparability for engine
// operand content.
type Index struct {
	*indexRow
}

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
func Seal(heap heapdomain.Schema, values *valuedomain.Schema, calls *calldomain.Algebra, packs *pack.Schema, selectors *keymatch.SelectorProjection) (*Topology, bool) {
	topology, diagnostic := SealWithFailure(heap, values, calls, packs, selectors)
	return topology, diagnostic.failure == SealFailureNone
}

// SealWithFailure is Seal with a permanent closed diagnostic. It never
// returns a partially admitted Topology.
func SealWithFailure(heap heapdomain.Schema, values *valuedomain.Schema, calls *calldomain.Algebra, packs *pack.Schema, selectors *keymatch.SelectorProjection) (*Topology, SealDiagnostic) {
	if values == nil || calls == nil || packs == nil || !values.OwnsHeapSchema(heap) || !values.LinkOwner().Matches(calls.LinkOwner()) || !values.LinkOwner().Matches(heap.LinkOwner()) || !values.LinkOwner().Matches(packs.LinkOwner()) || !heap.ContentID().Available() {
		return nil, sealDiagnostic(SealFailureInputs, -1, -1)
	}
	if calls.MountModuleCount() != values.MountCount() {
		return nil, sealDiagnostic(SealFailureBoundary, -1, -1)
	}
	seenModules := make(map[identity.ContentID]struct{}, calls.MountModuleCount())
	for mountIndex := 0; mountIndex < calls.MountModuleCount(); mountIndex++ {
		module, moduleOK := calls.MountModuleAt(mountIndex)
		if !moduleOK || !module.Available() {
			return nil, sealDiagnostic(SealFailureBoundary, mountIndex, -1)
		}
		if _, duplicate := seenModules[module]; duplicate {
			return nil, sealDiagnostic(SealFailureBoundary, mountIndex, -1)
		}
		if _, mountOK := heap.OccurrenceMountForModule(module); !mountOK {
			return nil, sealDiagnostic(SealFailureBoundary, mountIndex, -1)
		}
		seenModules[module] = struct{}{}
	}
	// The key/class projection is a pure function of the two sealed schemas,
	// so the composition seals it once and hands it here. Topology proves it
	// belongs to the exact pair it is about to read and never builds a second.
	if !selectors.FencedTo(heap, values) {
		return nil, sealDiagnostic(SealFailureValidity, -1, -1)
	}
	topology := &Topology{heap: heap, values: values, calls: calls, packs: packs, selectors: selectors, freshByRoot: make(map[heapdomain.Key]uint32), freshByApp: make(map[identity.ContentID]uint32), moduleExports: make(map[heapdomain.Key]moduleExportRoute)}
	if !topology.build() {
		return nil, sealDiagnostic(SealFailureRoots, -1, -1)
	}
	if !topology.buildModuleExports() {
		return nil, sealDiagnostic(SealFailureRoots, -1, -1)
	}
	if !topology.buildIndexRows() {
		return nil, sealDiagnostic(SealFailureValidity, -1, -1)
	}
	if !topology.buildRouteFacts() {
		return nil, sealDiagnostic(SealFailureValidity, -1, -1)
	}
	payloads, sources, sourceRefs, byPayloadSource, payloadsOK := buildRawPayloads(topology, packs)
	if !payloadsOK {
		return nil, sealDiagnostic(SealFailureValidity, -1, -1)
	}
	bootInitials, bootInitialsOK := buildRawBootInitials(topology, values)
	if !bootInitialsOK {
		return nil, sealDiagnostic(SealFailureValidity, -1, -1)
	}
	topology.catalog = &rawCatalog{payloads: payloads, sources: sources, sourceRefs: sourceRefs, byPayloadSource: byPayloadSource, bootInitials: bootInitials}
	if !topology.valid() {
		return nil, sealDiagnostic(SealFailureValidity, -1, -1)
	}
	// The support is fixed after Seal. Seed reusable route and exact-demand
	// dedup storage now; sync.Pool may be reclaimed, so this is a steady-state
	// allocation envelope, not a semantic or unconditional allocation guarantee.
	topology.scratch.New = func() any {
		return &routeScratch{
			emitted:  make(map[rawSetRouteFact]struct{}, len(topology.static)+len(topology.fresh)*2),
			demanded: make([]uint64, len(topology.freshApps)),
		}
	}
	topology.scratch.Put(&routeScratch{
		emitted:  make(map[rawSetRouteFact]struct{}, len(topology.static)+len(topology.fresh)*2),
		demanded: make([]uint64, len(topology.freshApps)),
	})
	return topology, SealDiagnostic{}
}

func (topology *Topology) valid() bool {
	return topology.baseValid() && topology.catalog != nil
}

func (topology *Topology) baseValid() bool {
	return topology != nil && topology.values != nil && topology.values.Valid() && topology.calls != nil && topology.packs != nil && topology.selectors != nil && topology.values.LinkOwner().Available() && topology.values.LinkOwner().Matches(topology.heap.LinkOwner()) && topology.values.LinkOwner().Matches(topology.calls.LinkOwner()) && topology.values.LinkOwner().Matches(topology.packs.LinkOwner()) &&
		topology.values.CoordinateCount() != 0 && topology.moduleExports != nil &&
		topology.values.OwnsHeapSchema(topology.heap) && topology.heap.ContentID().Available()
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
			if _, _, _, kind, _, source := topology.heap.AllocationOriginForKey(key); source {
				if kind == heapdomain.AllocationTable && !topology.appendStatic(key) {
					return false
				}
				continue
			}
			applicationID, outcomeResultID, _, fresh := key.FreshResultID()
			if !fresh {
				return false
			}
			if !outcomeResultID.Available() {
				return false
			}
			// Fresh rows are valid only when their source application is an
			// existing Call key. Their nominal kind is operation-dependent and
			// intentionally not read through a Link selection bridge.
			if _, ok := topology.calls.KeyForApplicationID(applicationID); !ok {
				return false
			}
			if topology.freshByRoot[key] != 0 {
				return false
			}
			appIndex := topology.freshByApp[applicationID]
			if appIndex == 0 {
				if uint64(len(topology.freshApps)) == uint64(^uint32(0)) {
					return false
				}
				topology.freshApps = append(topology.freshApps, freshApplication{applicationID: applicationID, tag: uint32(len(topology.freshApps) + 1)})
				appIndex = uint32(len(topology.freshApps))
				topology.freshByApp[applicationID] = appIndex
			}
			group := &topology.freshApps[appIndex-1]
			if group.tag == 0 {
				return false
			}
			topology.fresh = append(topology.fresh, freshRoot{key: key, applicationID: applicationID, tag: group.tag})
			topology.freshByRoot[key] = uint32(len(topology.fresh))
		default:
			return false
		}
	}
	return true
}

// buildModuleExports copies only the Value-owned composition proof into this
// Topology's detached owner. The fresh Heap denominator remains canonical;
// this pass merely validates and retains the already-issued table roots for
// exact routing. Missing proofs are valid and leave generic fresh behavior
// unchanged.
func (topology *Topology) buildModuleExports() bool {
	if topology == nil || topology.values == nil || topology.moduleExports == nil {
		return false
	}
	for index := 0; index < topology.heap.FreshCount(); index++ {
		_, key, keyOK := topology.heap.FreshAt(index)
		if !keyOK {
			return false
		}
		operation, operationOK := topology.values.ModuleExportFreshOperation(key)
		if !operationOK {
			continue
		}
		count := topology.values.ModuleExportFreshRootCount(key)
		if count == 0 {
			return false
		}
		roots := make([]heapdomain.Key, 0, count)
		seen := make(map[heapdomain.Key]struct{}, count)
		for rootIndex := 0; rootIndex < count; rootIndex++ {
			root, rootOK := topology.values.ModuleExportFreshRootAt(key, rootIndex)
			_, _, _, kind, _, originOK := topology.heap.AllocationOriginForKey(root)
			if !rootOK || !originOK || kind != heapdomain.AllocationTable {
				return false
			}
			if _, duplicate := seen[root]; duplicate {
				return false
			}
			seen[root] = struct{}{}
			roots = append(roots, root)
		}
		topology.moduleExports[key] = moduleExportRoute{operation: operation, roots: roots}
	}
	return true
}

func (topology *Topology) appendStatic(key heapdomain.Key) bool {
	added := false
	for _, role := range materialization.Roles() {
		if _, ok := topology.heap.Reference(key, role); ok {
			topology.static = append(topology.static, rawSetRouteFact{key: key, role: role})
			added = true
		}
	}
	return added
}

func (topology *Topology) buildIndexRows() bool {
	if topology == nil {
		return false
	}
	rows := make(map[heapdomain.IndexAccess]*indexRow, topology.heap.IndexAccessCount())
	for index := 0; index < topology.heap.IndexAccessCount(); index++ {
		indexAccess, accessOK := topology.heap.IndexAccessAt(index)
		if !accessOK {
			return false
		}
		id, idOK := topology.heap.IndexAccessID(indexAccess)
		geometry, geometryOK := topology.heap.IndexAccessGeometry(indexAccess)
		// Heap stores the already-issued Boundary Value identities for access
		// geometry. Artifact semantic IDs use CoordinateForMountedSemantic; the
		// two ContentID namespaces must not be conflated.
		receiver, receiverOK := topology.values.CoordinateForID(geometry.BaseValueID)
		slot, slotOK := topology.heap.SlotForIndexAccess(indexAccess)
		if !idOK || !id.Available() || !geometryOK || !receiverOK || !slotOK {
			return false
		}
		row := &indexRow{topology: topology, indexAccess: indexAccess, receiver: receiver, slot: slot, id: id}
		if geometry.Read {
			resultID, resultValueOK := topology.heap.IndexAccessResultID(indexAccess)
			result, resultOK := topology.values.CoordinateForID(resultID)
			if !resultValueOK || !resultOK {
				return false
			}
			row.result = result
		}
		if geometry.DynamicKey {
			coordinate, coordinateOK := topology.values.CoordinateForID(geometry.KeyValueID)
			if !coordinateOK {
				return false
			}
			row.dynamicKey = coordinate
		}
		rows[indexAccess] = row
	}
	topology.indexRows = rows
	return len(rows) == topology.heap.IndexAccessCount()
}

func (topology *Topology) buildRouteFacts() bool {
	if topology == nil {
		return false
	}
	routes := make([]rawSetRouteFact, len(topology.static))
	routesByTag := make(map[heapdomain.RawRouteTag]uint32, len(topology.static))
	for index, static := range topology.static {
		tag, ok := topology.heap.RouteTag(static.key, static.role)
		if !ok || tag == 0 {
			return false
		}
		route := rawSetRouteFact{key: static.key, role: static.role, tag: tag}
		if _, duplicate := routesByTag[tag]; duplicate {
			return false
		}
		routes[index] = route
		routesByTag[tag] = uint32(index + 1)
	}
	topology.static = routes
	topology.staticByTag = routesByTag
	return true
}

// Access returns the exact existing Heap candidate row. All geometry comes
// from the sealed Heap row; this path never reopens Flow or reconstructs a
// Lens.
func (topology *Topology) Access(indexAccess heapdomain.IndexAccess) (Index, bool) {
	if topology == nil || !topology.valid() || topology.indexRows == nil {
		return Index{}, false
	}
	row, ok := topology.indexRows[indexAccess]
	return Index{indexRow: row}, ok && topology.ownsRow(row)
}

// OwnsAccess is the topology ownership fence for one retained index operand.
// A valid Index from a duplicate same-content Topology is still foreign:
// the issuing Topology pointer is part of the semantic owner boundary. The
// canonical reissue also verifies every derived coordinate and policy field,
// so a matching portable ID cannot substitute for ownership.
func (topology *Topology) OwnsAccess(access Index) bool {
	return topology != nil && topology.ownsRow(access.indexRow)
}

func (topology *Topology) ownsRow(row *indexRow) bool {
	if topology == nil || row == nil || row.topology != topology || topology.indexRows == nil || topology.indexRows[row.indexAccess] != row || !row.id.Available() {
		return false
	}
	_, geometryOK := topology.heap.IndexAccessGeometry(row.indexAccess)
	return geometryOK
}

func (access Index) valid() bool {
	if access.indexRow == nil || access.topology == nil || !access.id.Available() {
		return false
	}
	return access.topology.ownsRow(access.indexRow)
}

func (access Index) IndexAccess() (heapdomain.IndexAccess, bool) {
	if !access.valid() {
		return heapdomain.IndexAccess{}, false
	}
	return access.indexAccess, true
}
func (access Index) Receiver() (valuedomain.Coordinate, bool) {
	if !access.valid() {
		return valuedomain.Coordinate{}, false
	}
	return access.receiver, true
}

// Result returns the fixed result Value coordinate for a typed Read candidate.
// Write candidates have no result coordinate and fail closed, so a Rule cannot
// turn an assignment source into an index-read result.
func (access Index) Result() (valuedomain.Coordinate, bool) {
	if !access.valid() || !access.result.Valid() {
		return valuedomain.Coordinate{}, false
	}
	return access.result, true
}

// DynamicKey returns the existing Value coordinate for a dynamic heap key.
// Exact-key lenses return false: their selector is already carried by Slot,
// so no synthetic literal Value coordinate is introduced.
func (access Index) DynamicKey() (valuedomain.Coordinate, bool) {
	if !access.valid() || !access.dynamicKey.Valid() {
		return valuedomain.Coordinate{}, false
	}
	return access.dynamicKey, true
}

// Read reports whether the Heap candidate row is a typed Read. Raw-get
// admission is based solely on this row membership; no operation enum is
// reclassified at query time.
func (access Index) Read() bool {
	if !access.valid() {
		return false
	}
	geometry, ok := access.topology.heap.IndexAccessGeometry(access.indexAccess)
	return ok && geometry.Read
}

// Write reports whether this existing Heap candidate row is a typed indexed
// write.  Write rows deliberately retain no result coordinate; mutation rules
// must use the sealed RHS Payload rather than treating a write as a read.
func (access Index) Write() bool {
	if !access.valid() {
		return false
	}
	geometry, ok := access.topology.heap.IndexAccessGeometry(access.indexAccess)
	return ok && !geometry.Read
}

func (access Index) Slot() (heapdomain.Slot, bool) {
	if !access.valid() {
		return heapdomain.Slot{}, false
	}
	return access.slot, true
}
func (access Index) ID() (identity.ContentID, bool) {
	if !access.valid() {
		return identity.ContentID{}, false
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
			key, ok := topology.calls.KeyForApplicationID(group.applicationID)
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
		key, ok := topology.calls.KeyForApplicationID(fresh.applicationID)
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
			identity := rawSetRouteFact{key: root, role: role}
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
	if rootID, ok := reference.BootRootID(); ok {
		return topology.heap.KeyForBootID(rootID)
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

// CallKeyForTag resolves the topology-local demand tag issued by
// VisitReceiverCallDemand. It is the sole hot mapping for receipt-selected
// fresh Call rows; callers never retain a copied fresh-application slice.
func (topology *Topology) CallKeyForTag(tag uint64) (calldomain.Key, bool) {
	if topology == nil || !topology.valid() || tag == 0 || tag > uint64(len(topology.freshApps)) {
		return calldomain.Key{}, false
	}
	return topology.calls.KeyForApplicationID(topology.freshApps[tag-1].applicationID)
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
	key, ok := topology.calls.KeyForApplicationID(fresh.applicationID)
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
	if route, proved := topology.moduleExports[fresh.key]; proved {
		matched, unknown := false, state.HasOpaqueAlternative()
		for index := 0; index < state.KnownTargetCount(); index++ {
			target, targetOK := state.KnownTargetAt(index)
			operation, operationOK := target.Operation()
			// The numeric operation is necessary but not sufficient: an
			// ordinary seed with the same Target operation must not masquerade
			// as the scoped loader that authored the Module Import proof.
			if !targetOK || !operationOK || !target.IsScopedLoader() || operation != route.operation {
				unknown = true
				continue
			}
			matched = true
		}
		if matched {
			emitted := false
			for _, root := range route.roots {
				if _, referenceOK := topology.heap.Reference(root, role); !referenceOK {
					unknown = true
					continue
				}
				emitted = true
				if !emit(topology.rootRoute(root, role)) {
					return false
				}
			}
			if emitted && !unknown {
				return true
			}
		}
	}
	return emit(topology.unknownRoute())
}

// visitFreshApplication is the Value.Top path. It retains only Link's direct
// root-to-Application edge; selected-operation reconstruction is forbidden.
func (topology *Topology) visitFreshApplication(group freshApplication, callState CallState, emit func(Route) bool) bool {
	key, ok := topology.calls.KeyForApplicationID(group.applicationID)
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
