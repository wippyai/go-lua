package recurrence

import (
	"errors"
	"fmt"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/routeplan"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Binding is recurrence-issued, ordinal-aligned SCC truth for one exact
// routeplan.Plan. Only recurrence can construct it while the sourcecontrol
// owner and private SCC partition are live. Causal can only claim records.
type Binding struct {
	result *Result
	plan   *routeplan.Plan
	// components is the exact ordered directory of canonical cyclic heads
	// issued from the live SCC partition. It contains no node or arc
	// projection; its order is canonical across equivalent replay.
	components []Component
	// hierarchy is the one recurrence-issued nested schedule for this exact
	// route Plan. Causal must take it before the component directory may close.
	hierarchy      HierarchyProof
	hierarchyTaken bool
	mu             sync.Mutex
	claimed        []bool
	closed         bool
	rows           []bindingRow
}

type bindingRow struct {
	member    bool
	component keyspace.Term
	// Endpoint paths are copied from the one SourceControl vertex catalog
	// while the route binding is live.  Causal consumes these receipts after
	// the graph/Plan capabilities have been released; it must not resolve the
	// route origin a second time.
	fromPath identity.ContentID
	toPath   identity.ContentID
	muHead   keyspace.Term
	first    uint32
	past     uint32
}

// Component is the opaque recurrence directory entry transferred to Causal.
// The head capability and semantic path remain paired.
type Component struct {
	head keyspace.Term
	path identity.ContentID
}

func (component Component) Head() (keyspace.Term, bool) {
	return component.head, component.head != 0 && component.path.Available()
}

func (component Component) HeadPath() (identity.ContentID, bool) {
	if component.head == 0 || !component.path.Available() {
		return identity.ContentID{}, false
	}
	return component.path, true
}

// BoundRoute is an opaque recurrence-issued row. Its accessors distinguish a
// valid acyclic route from a cyclic member without exposing construction. A
// zero BoundRoute is never a certificate: consumers must require Valid before
// accepting an acyclic result.
type BoundRoute struct {
	row    bindingRow
	issued bool
}

func (route BoundRoute) Valid() bool { return route.issued }

// Matches proves this opaque issuance belongs to the exact recurrence Result
// and Plan selected by the current seal transaction. It is verification only;
// callers cannot reserve, mint, or rebind an issuance.
func (binding *Binding) Matches(plan *routeplan.Plan, result *Result) bool {
	if binding == nil || plan == nil || result == nil {
		return false
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	return !binding.closed && binding.plan == plan && binding.result == result
}

func (route BoundRoute) Member() (keyspace.Term, bool) { return route.row.component, route.row.member }
func (route BoundRoute) FromPath() (identity.ContentID, bool) {
	if !route.issued || !route.row.fromPath.Available() {
		return identity.ContentID{}, false
	}
	return route.row.fromPath, true
}
func (route BoundRoute) ToPath() (identity.ContentID, bool) {
	if !route.issued || !route.row.toPath.Available() {
		return identity.ContentID{}, false
	}
	return route.row.toPath, true
}
func (route BoundRoute) Mu() (keyspace.Term, uint32, uint32, bool) {
	if route.row.muHead == 0 {
		return 0, 0, 0, false
	}
	return route.row.muHead, route.row.first, route.row.past, true
}

// Claim consumes one exact Plan ordinal. A duplicate, foreign-plan, or closed
// claim fails closed.
func (binding *Binding) Claim(plan *routeplan.Plan, index int) (BoundRoute, bool) {
	if binding == nil || plan == nil {
		return BoundRoute{}, false
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed || binding.plan != plan || index < 0 || index >= len(binding.rows) || binding.claimed[index] {
		return BoundRoute{}, false
	}
	binding.claimed[index] = true
	return BoundRoute{row: binding.rows[index], issued: true}, true
}

// CompleteAndTakeDirectory proves every ordinal was consumed exactly once,
// transfers the ordered recurrence-issued directory verbatim, and terminally
// clears all Binding authority in one lock acquisition.
func (binding *Binding) CompleteAndTakeDirectory(plan *routeplan.Plan) ([]Component, bool) {
	if binding == nil || plan == nil {
		return nil, false
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed || binding.plan != plan || !binding.hierarchyTaken {
		return nil, false
	}
	for _, claimed := range binding.claimed {
		if !claimed {
			return nil, false
		}
	}
	if binding.components == nil {
		return nil, false
	}
	components := binding.components
	binding.closed = true
	binding.rows = nil
	binding.components = nil
	binding.hierarchy.events = nil
	binding.hierarchy.events = nil
	binding.claimed = nil
	binding.plan = nil
	binding.result = nil
	return components, true
}

// CompleteAndTakeHierarchy transfers the recurrence-issued nested bracket
// stream only after every final route was claimed. It does not close the
// Binding: Causal must still take the matching component directory in the
// same transaction.
func (binding *Binding) CompleteAndTakeHierarchy(plan *routeplan.Plan) (HierarchyProof, bool) {
	if binding == nil || plan == nil {
		return HierarchyProof{}, false
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed || binding.plan != plan || binding.hierarchyTaken {
		return HierarchyProof{}, false
	}
	for _, claimed := range binding.claimed {
		if !claimed {
			return HierarchyProof{}, false
		}
	}
	if len(binding.hierarchy.events) == 0 {
		return HierarchyProof{}, false
	}
	hierarchy := binding.hierarchy
	binding.hierarchyTaken = true
	return hierarchy, true
}

// Abort terminally clears an unconsumed binding after its private Causal seal
// transaction fails. It is intentionally an internal-package cleanup hook,
// not a reservation or construction surface; a closed binding is unchanged.
func (binding *Binding) Abort(plan *routeplan.Plan) {
	if binding == nil || plan == nil {
		return
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed || binding.plan != plan {
		return
	}
	binding.closed = true
	binding.rows = nil
	binding.components = nil
	binding.claimed = nil
	binding.plan = nil
	binding.result = nil
}

// bindPlan is intentionally unexported: SealWithPlan invokes it while
// graph/parts/heads exist. It never scans endpoint families; every Plan
// origin is an exact ArcRef or exact NodeRef pair.
func bindPlan(result *Result, plan *routeplan.Plan, graph *sourcecontrol.Result, parts components, heads []keyspace.Term, hierarchy HierarchyProof, sourceView source.View, flow authored.View) (*Binding, error) {
	if !available(result) || plan == nil || graph == nil || !graph.OwnsOwner(plan.Owner()) ||
		!sourcecontrol.Matches(graph, result.sourceID, result.flowID, result.staticID, result.moduleID) ||
		result.ArcCount() != graph.ArcCount() {
		return nil, errors.New("program/flow/recurrence: route plan owner is unavailable")
	}
	components, err := componentDirectory(parts, heads, graph, sourceView, flow)
	if err != nil {
		return nil, err
	}
	// Endpoint phases must be a splice into the exact recurrence schedule
	// transferred by this seal. This rejects a same-owner PhaseRef whose path
	// was issued by a sibling schedule or was never published into the WTO
	// extension, while keeping endpoint identity separate from SCC metadata.
	hierarchyPaths := make(map[identity.ContentID]struct{}, len(hierarchy.events))
	for _, event := range hierarchy.events {
		if event.Kind == HierarchyExit {
			continue
		}
		if !event.path.Available() {
			return nil, errors.New("program/flow/recurrence: hierarchy endpoint path is unavailable")
		}
		if _, duplicate := hierarchyPaths[event.path]; duplicate {
			return nil, errors.New("program/flow/recurrence: hierarchy endpoint path is duplicated")
		}
		hierarchyPaths[event.path] = struct{}{}
	}
	rows := make([]bindingRow, plan.Count())
	for ordinal := 0; ordinal < plan.Count(); ordinal++ {
		route, origin, ok := plan.At(ordinal)
		if !ok {
			return nil, errors.New("program/flow/recurrence: route plan row is malformed")
		}
		endpoint, endpointOK := origin.EndpointPhaseReceipt()
		if !endpointOK {
			return nil, errors.New("program/flow/recurrence: route endpoint phase receipt is unavailable")
		}
		fromPhase, toPhase, phasesOK := endpoint.Endpoints()
		if !phasesOK {
			return nil, errors.New("program/flow/recurrence: route endpoint phase receipt is malformed")
		}
		fromPath, fromPathOK := graph.ResolvePhaseRef(fromPhase)
		toPath, toPathOK := graph.ResolvePhaseRef(toPhase)
		if !fromPathOK || !toPathOK {
			return nil, errors.New("program/flow/recurrence: route endpoint phase path is unavailable")
		}
		if _, present := hierarchyPaths[fromPath]; !present {
			return nil, errors.New("program/flow/recurrence: route source phase is outside hierarchy")
		}
		if _, present := hierarchyPaths[toPath]; !present {
			return nil, errors.New("program/flow/recurrence: route target phase is outside hierarchy")
		}
		carrier, carrierOK := origin.RecurrenceCarrier()
		if !carrierOK {
			return nil, errors.New("program/flow/recurrence: route recurrence carrier is unavailable")
		}
		var from, to uint32
		if arcRef, arcOK := carrier.ArcRef(); arcOK {
			index, arc, resolved := graph.ResolveArcRef(arcRef)
			if !resolved {
				return nil, errors.New("program/flow/recurrence: route plan ArcRef is foreign")
			}
			from, to = arc.From, arc.To
			arcFromPath, arcFromPathOK := graph.VertexPathAt(from)
			arcToPath, arcToPathOK := graph.VertexPathAt(to)
			if !arcFromPathOK || !arcToPathOK {
				return nil, errors.New("program/flow/recurrence: route endpoint vertex path is unavailable")
			}
			// A CSR endpoint is an exact splice to the structural carrier;
			// an Outcome phase is intentionally a distinct subdivision point.
			if (!fromPhase.OutcomePhase() && fromPath != arcFromPath) ||
				(!toPhase.OutcomePhase() && toPath != arcToPath) {
				return nil, fmt.Errorf("program/flow/recurrence: Arc carrier disagrees with CSR endpoint splice route=%d arm=%d from-family=%d to-family=%d from-outcome=%t to-outcome=%t from-match=%t to-match=%t", ordinal, route.Arm, keyspace.TermFamily(route.From), keyspace.TermFamily(route.To), fromPhase.OutcomePhase(), toPhase.OutcomePhase(), fromPath == arcFromPath, toPath == arcToPath)
			}
			row := bindingForNodes(parts, heads, from, to)
			row.fromPath, row.toPath = fromPath, toPath
			if !graph.Reachable(from) || !graph.Reachable(to) {
				return nil, errors.New("program/flow/recurrence: Arc carrier is outside reachable recurrence region")
			}
			annotation, annotationOK := result.ArcAt(index)
			if !annotationOK {
				return nil, errors.New("program/flow/recurrence: route plan Arc annotation is unavailable")
			}
			if annotation.Head == 0 && (annotation.First != 0 || annotation.Past != 0) {
				return nil, errors.New("program/flow/recurrence: Mu-less Arc annotation carries a reset range")
			}
			if annotation.Head != 0 {
				streamCount, streamOK := result.DecisionCount(annotation.Head)
				if !row.member || row.component != annotation.Head || annotation.Past < annotation.First || !streamOK || streamCount < 0 || annotation.Past > uint32(streamCount) || !canonicalHead(annotation.Head) {
					return nil, errors.New("program/flow/recurrence: Mu Arc binding disagrees with component")
				}
				row.muHead, row.first, row.past = annotation.Head, annotation.First, annotation.Past
			}
			rows[ordinal] = row
			continue
		}
		fromRef, toRef, nodePair := carrier.NodePair()
		if nodePair {
			from, ok = graph.ResolveNodeRef(fromRef)
			if !ok {
				return nil, errors.New("program/flow/recurrence: route plan source NodeRef is foreign")
			}
			to, ok = graph.ResolveNodeRef(toRef)
			if !ok {
				return nil, errors.New("program/flow/recurrence: route plan target NodeRef is foreign")
			}
			carrierFromPath, carrierFromPathOK := graph.VertexPathAt(from)
			carrierToPath, carrierToPathOK := graph.VertexPathAt(to)
			if !carrierFromPathOK || !carrierToPathOK || !graph.Reachable(from) || !graph.Reachable(to) {
				return nil, errors.New("program/flow/recurrence: route endpoint vertex path is unavailable")
			}
			if (!fromPhase.OutcomePhase() && fromPath != carrierFromPath) ||
				(!toPhase.OutcomePhase() && toPath != carrierToPath) {
				return nil, errors.New("program/flow/recurrence: NodePair carrier disagrees with CSR endpoint splice")
			}
			row := bindingForNodes(parts, heads, from, to)
			row.fromPath, row.toPath = fromPath, toPath
			rows[ordinal] = row
			continue
		}
		if carrier.Kind() != routeplan.CarrierNone {
			return nil, errors.New("program/flow/recurrence: route recurrence carrier is malformed")
		}
		// Endpoint-only phase pairs, including Outcome phases, never carry
		// structural SCC/Mu membership.
		row := bindingRow{}
		row.fromPath, row.toPath = fromPath, toPath
		rows[ordinal] = row
	}
	// All fallible row construction is complete. Claim issuance only now so a
	// rejected Plan cannot burn an otherwise valid Result; this private fence
	// still prevents an independently reissued Binding for the same Result.
	result.bindingMu.Lock()
	defer result.bindingMu.Unlock()
	if result.bindingIssued {
		return nil, errors.New("program/flow/recurrence: recurrence result already issued a route binding")
	}
	result.bindingIssued = true
	if len(hierarchy.events) == 0 {
		return nil, errors.New("program/flow/recurrence: hierarchy certificate is empty")
	}
	return &Binding{result: result, plan: plan, components: components, hierarchy: hierarchy, claimed: make([]bool, len(rows)), rows: rows}, nil
}

type componentEntry struct {
	head keyspace.Term
	path identity.ContentID
}

func componentDirectory(parts components, heads []keyspace.Term, graph *sourcecontrol.Result, sourceView source.View, flow authored.View) ([]Component, error) {
	if len(parts.cyclic) != len(heads) {
		return nil, errors.New("program/flow/recurrence: cyclic component directory is malformed")
	}
	directory := make([]componentEntry, 0)
	for component, cyclic := range parts.cyclic {
		if !cyclic {
			continue
		}
		head := heads[component]
		if !canonicalHead(head) {
			return nil, errors.New("program/flow/recurrence: cyclic component lacks a canonical head")
		}
		node, found := decisionCoordinate(sourceView, flow, graph, head)
		if !found {
			return nil, errors.New("program/flow/recurrence: cyclic component head coordinate is unavailable")
		}
		path, found := graph.VertexPathAt(node)
		if !found {
			return nil, errors.New("program/flow/recurrence: cyclic component head path is unavailable")
		}
		directory = append(directory, componentEntry{head: head, path: path})
	}
	identity.SortByContentID(directory, componentEntryPath)
	for index := 1; index < len(directory); index++ {
		if directory[index-1].head == directory[index].head || directory[index-1].path == directory[index].path {
			return nil, errors.New("program/flow/recurrence: cyclic components share a canonical head")
		}
	}
	result := make([]Component, len(directory))
	for index, entry := range directory {
		result[index] = Component{head: entry.head, path: entry.path}
	}
	return result, nil
}

func bindingForNodes(parts components, heads []keyspace.Term, from, to uint32) bindingRow {
	if uint64(from) >= uint64(len(parts.of)) || uint64(to) >= uint64(len(parts.of)) || parts.of[from] == unassignedComponent || parts.of[from] != parts.of[to] {
		return bindingRow{}
	}
	component := parts.of[from]
	if uint64(component) >= uint64(len(heads)) || uint64(component) >= uint64(len(parts.cyclic)) ||
		!parts.cyclic[component] || !canonicalHead(heads[component]) {
		return bindingRow{}
	}
	return bindingRow{member: true, component: heads[component]}
}

func canonicalHead(head keyspace.Term) bool {
	family := keyspace.TermFamily(head)
	return keyspace.TermOrdinal(head) != 0 && (family == keyspace.FamilyLabel || family == keyspace.FamilyLoop)
}

func componentEntryPath(entry componentEntry) identity.ContentID { return entry.path }
