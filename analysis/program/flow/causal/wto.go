package causal

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/recurrence"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// wtoFailure is seal-private diagnostic provenance for LocalWTO
// materialization. It deliberately carries only dense transaction indexes;
// no graph coordinate, plan, or source term escapes this phase.
type wtoFailure struct {
	phase       wtoFailurePhase
	reason      wtoFailureReason
	row         int
	event       int
	path        int
	site        int
	route       int
	ref         int
	local       bool
	arm         BoundaryArmKind
	fromFamily  wtoLogicalFamily
	toFamily    wtoLogicalFamily
	fromOutcome wtoOutcomeKind
	toOutcome   wtoOutcomeKind
	fromPhase   wtoPhaseClass
	toPhase     wtoPhaseClass
	endpoint    identity.ContentID
	routeID     identity.ContentID
}

// routeDiagnostic is deliberately seal-private. Its fields are closed
// classifications or semantic path/route scalars; it never retains a Term,
// owner, graph node, or plan capability.
type routeDiagnostic struct {
	fromFamily  wtoLogicalFamily
	toFamily    wtoLogicalFamily
	fromOutcome wtoOutcomeKind
	toOutcome   wtoOutcomeKind
	fromPhase   wtoPhaseClass
	toPhase     wtoPhaseClass
}

type wtoLogicalFamily keyspace.Family

var wtoLogicalFamilyNames = [...]string{
	"invalid", "nil", "bool", "integer", "float", "string", "values", "lens-exact", "lens-key", "return", "break", "label", "goto", "body", "cell", "read", "vararg", "unary", "binary", "select", "bind", "assign", "function", "call", "branch", "loop", "table", "key", "type-alias", "type-interface", "type-param", "type-primitive", "type-literal", "type-optional", "type-union", "type-intersection", "type-ref", "type-generic", "type-array", "type-map", "type-record", "type-field", "type-function", "type-asserts", "declared-type", "type-publication", "type-value", "value-claim", "annotation", "type-of", "type-key-of", "type-index-access", "type-conditional", "write", "table-field", "outcome", "control-fault", "import",
}

func (family wtoLogicalFamily) String() string {
	index := int(family)
	if index < 0 || index >= len(wtoLogicalFamilyNames) {
		return "invalid"
	}
	return wtoLogicalFamilyNames[index]
}

type wtoOutcomeKind uint8

const (
	wtoOutcomeNone wtoOutcomeKind = iota
	wtoOutcomeNormal
	wtoOutcomeReturn
	wtoOutcomeThrow
	wtoOutcomeBreak
	wtoOutcomeGoto
	wtoOutcomeYield
	wtoOutcomeCancel
	wtoOutcomeInvalid
)

func wtoOutcome(outcome kind.OutcomeKind) wtoOutcomeKind {
	switch outcome {
	case kind.OutcomeNormal:
		return wtoOutcomeNormal
	case kind.OutcomeReturn:
		return wtoOutcomeReturn
	case kind.OutcomeThrow:
		return wtoOutcomeThrow
	case kind.OutcomeBreak:
		return wtoOutcomeBreak
	case kind.OutcomeGoto:
		return wtoOutcomeGoto
	case kind.OutcomeYield:
		return wtoOutcomeYield
	case kind.OutcomeCancel:
		return wtoOutcomeCancel
	default:
		return wtoOutcomeInvalid
	}
}

func (outcome wtoOutcomeKind) String() string {
	switch outcome {
	case wtoOutcomeNone:
		return "none"
	case wtoOutcomeNormal:
		return "normal"
	case wtoOutcomeReturn:
		return "return"
	case wtoOutcomeThrow:
		return "throw"
	case wtoOutcomeBreak:
		return "break"
	case wtoOutcomeGoto:
		return "goto"
	case wtoOutcomeYield:
		return "yield"
	case wtoOutcomeCancel:
		return "cancel"
	default:
		return "invalid"
	}
}

type wtoPhaseClass uint8

const (
	wtoPhaseUnavailable wtoPhaseClass = iota
	wtoPhaseCSR
	wtoPhaseOutcome
)

func (phase wtoPhaseClass) String() string {
	switch phase {
	case wtoPhaseCSR:
		return "csr"
	case wtoPhaseOutcome:
		return "outcome"
	default:
		return "unavailable"
	}
}

type wtoFailurePhase uint8

const (
	wtoFailurePhaseInvalid wtoFailurePhase = iota
	wtoFailurePhasePreflight
	wtoFailurePhaseVertexPaths
	wtoFailurePhaseEvent
	wtoFailurePhasePoint
	wtoFailurePhaseBracket
	wtoFailurePhaseRows
	wtoFailurePhaseClassify
)

type wtoFailureReason uint8

const (
	wtoFailureReasonInvalid wtoFailureReason = iota
	wtoFailureReasonPreflight
	wtoFailureReasonVertexPaths
	wtoFailureReasonEventUnavailable
	wtoFailureReasonEventPath
	wtoFailureReasonEventNode
	wtoFailureReasonPointUnavailable
	wtoFailureReasonPointSiteSet
	wtoFailureReasonPointDuplicate
	wtoFailureReasonEnterID
	wtoFailureReasonEnterDuplicate
	wtoFailureReasonSingletonID
	wtoFailureReasonSingletonDuplicate
	wtoFailureReasonExitBracket
	wtoFailureReasonEventKind
	wtoFailureReasonUnclosedBracket
	wtoFailureReasonRowPreflight
	wtoFailureReasonRowEvent
	wtoFailureReasonRowPathEmpty
	wtoFailureReasonRowRouteFromSite
	wtoFailureReasonRowRouteToSite
	wtoFailureReasonRowRouteFromAttach
	wtoFailureReasonRowRouteToAttach
	wtoFailureReasonRowOutcomeSite
	wtoFailureReasonRowOutcomeAttach
	wtoFailureReasonRowRouteFromPath
	wtoFailureReasonRowRouteToPath
	wtoFailureReasonClassifyPreflight
	wtoFailureReasonClassifyParent
	wtoFailureReasonClassifyEndpoint
	wtoFailureReasonClassifyLCA
	wtoFailureReasonClassifyRoute
	wtoFailureReasonClassifyMembership
	wtoFailureReasonClassifyBoundary
	wtoFailureReasonClassifyWrite
	wtoFailureReasonClassifyOrder
)

func (failure *wtoFailure) Error() string {
	if failure == nil {
		return "program/flow/causal: LocalWTO failure is unavailable"
	}
	return fmt.Sprintf("program/flow/causal: LocalWTO phase=%s reason=%s row=%d event=%d path=%d site=%d route=%d ref=%d local=%t arm=%s from-family=%s to-family=%s from-outcome=%s to-outcome=%s from-phase=%s to-phase=%s endpoint-path=%x route-id=%x", failure.phase, failure.reason, failure.row, failure.event, failure.path, failure.site, failure.route, failure.ref, failure.local, wtoArmName(failure.arm), failure.fromFamily, failure.toFamily, failure.fromOutcome, failure.toOutcome, failure.fromPhase, failure.toPhase, failure.endpoint, failure.routeID)
}

func failWTO(phase wtoFailurePhase, reason wtoFailureReason, row, event, route int) error {
	return &wtoFailure{phase: phase, reason: reason, row: row, event: event, path: -1, site: -1, route: route, ref: -1}
}

func failWTORow(reason wtoFailureReason, row, event, path, site, route int) error {
	return &wtoFailure{phase: wtoFailurePhaseRows, reason: reason, row: row, event: event, path: path, site: site, route: route, ref: -1}
}

func failWTORowRef(reason wtoFailureReason, ref successorRef, route, path, site int) error {
	endpoint := ref.fromPoint
	if reason == wtoFailureReasonRowRouteToPath || reason == wtoFailureReasonRowRouteToSite || reason == wtoFailureReasonRowRouteToAttach {
		endpoint = ref.toPoint
	}
	diagnostic := ref.diagnostic
	return &wtoFailure{phase: wtoFailurePhaseRows, reason: reason, row: -1, event: -1, path: path, site: site, route: route, ref: int(ref.index), local: ref.local, arm: ref.arm,
		fromFamily: diagnostic.fromFamily, toFamily: diagnostic.toFamily, fromOutcome: diagnostic.fromOutcome, toOutcome: diagnostic.toOutcome,
		fromPhase: diagnostic.fromPhase, toPhase: diagnostic.toPhase, endpoint: endpoint, routeID: ref.routeDigest}
}

func wtoArmName(arm BoundaryArmKind) string {
	switch arm {
	case BoundaryLocal:
		return "local"
	case BoundaryResume:
		return "resume"
	case BoundarySelectTrue:
		return "select-true"
	case BoundarySelectFalse:
		return "select-false"
	case BoundaryTail:
		return "tail"
	case BoundaryThrow:
		return "throw"
	case BoundaryYield:
		return "yield"
	case BoundaryCancel:
		return "cancel"
	default:
		return "invalid"
	}
}

func (phase wtoFailurePhase) String() string {
	switch phase {
	case wtoFailurePhasePreflight:
		return "preflight"
	case wtoFailurePhaseVertexPaths:
		return "vertex-paths"
	case wtoFailurePhaseEvent:
		return "event"
	case wtoFailurePhasePoint:
		return "point"
	case wtoFailurePhaseBracket:
		return "bracket"
	case wtoFailurePhaseRows:
		return "hierarchy-rows"
	case wtoFailurePhaseClassify:
		return "classify"
	default:
		return "invalid"
	}
}

func (reason wtoFailureReason) String() string {
	switch reason {
	case wtoFailureReasonPreflight:
		return "preflight"
	case wtoFailureReasonVertexPaths:
		return "vertex-paths"
	case wtoFailureReasonEventUnavailable:
		return "event-unavailable"
	case wtoFailureReasonEventPath:
		return "event-path"
	case wtoFailureReasonEventNode:
		return "event-node"
	case wtoFailureReasonPointUnavailable:
		return "point-unavailable"
	case wtoFailureReasonPointSiteSet:
		return "point-site-set"
	case wtoFailureReasonPointDuplicate:
		return "point-duplicate"
	case wtoFailureReasonEnterID:
		return "enter-id"
	case wtoFailureReasonEnterDuplicate:
		return "enter-duplicate"
	case wtoFailureReasonSingletonID:
		return "singleton-id"
	case wtoFailureReasonSingletonDuplicate:
		return "singleton-duplicate"
	case wtoFailureReasonExitBracket:
		return "exit-bracket"
	case wtoFailureReasonEventKind:
		return "event-kind"
	case wtoFailureReasonUnclosedBracket:
		return "unclosed-bracket"
	case wtoFailureReasonRowPreflight:
		return "row-preflight"
	case wtoFailureReasonRowEvent:
		return "row-event"
	case wtoFailureReasonRowPathEmpty:
		return "row-path-empty"
	case wtoFailureReasonRowRouteFromSite:
		return "row-route-from-site"
	case wtoFailureReasonRowRouteToSite:
		return "row-route-to-site"
	case wtoFailureReasonRowRouteFromAttach:
		return "row-route-from-attach"
	case wtoFailureReasonRowRouteToAttach:
		return "row-route-to-attach"
	case wtoFailureReasonRowOutcomeSite:
		return "row-outcome-site"
	case wtoFailureReasonRowOutcomeAttach:
		return "row-outcome-attach"
	case wtoFailureReasonRowRouteFromPath:
		return "row-route-from-path"
	case wtoFailureReasonRowRouteToPath:
		return "row-route-to-path"
	case wtoFailureReasonClassifyPreflight:
		return "classify-preflight"
	case wtoFailureReasonClassifyParent:
		return "classify-parent"
	case wtoFailureReasonClassifyEndpoint:
		return "classify-endpoint"
	case wtoFailureReasonClassifyLCA:
		return "classify-lca"
	case wtoFailureReasonClassifyRoute:
		return "classify-route"
	case wtoFailureReasonClassifyMembership:
		return "classify-membership"
	case wtoFailureReasonClassifyBoundary:
		return "classify-boundary"
	case wtoFailureReasonClassifyWrite:
		return "classify-write"
	case wtoFailureReasonClassifyOrder:
		return "classify-order"
	default:
		return "invalid"
	}
}

// WTOEventKind is the parent-issued local schedule vocabulary.  It is kept
// below Flow's public facade so that physical result indexes never leave the
// causal owner.
type WTOEventKind uint8

const (
	WTOEventInvalid WTOEventKind = iota
	WTOEventEnter
	WTOEventPoint
	WTOEventExit
)

type wtoEvent struct {
	kind   WTOEventKind
	region uint32
	point  uint32
}

type wtoRegion struct {
	id        identity.ContentID
	parent    identity.ContentID
	headerID  identity.ContentID
	header    uint32
	hasHeader bool
	cyclic    bool
	routes    []successorRef
	points    []uint32
	sites     []uint32
}

type wtoPoint struct {
	path  identity.ContentID
	sites []uint32
}

type wtoStore struct {
	regions     []wtoRegion
	events      []wtoEvent
	byID        map[identity.ContentID]uint32
	points      []wtoPoint
	pointByPath map[identity.ContentID]uint32
}

const (
	wtoCyclicDomain    = "wippy/program/flow/local-wto-cyclic"
	wtoSingletonDomain = "wippy/program/flow/local-wto-singleton"
)

func wtoID(domain string, semantic identity.ContentID) identity.ContentID {
	if !semantic.Available() {
		return identity.ContentID{}
	}
	var encoded [96]byte
	offset := copy(encoded[:], domain)
	encoded[offset] = 0
	offset++
	offset += copy(encoded[offset:], semantic[:])
	return identity.ContentID(sha256.Sum256(encoded[:offset]))
}

// LocalWTO is Flow's final local schedule projection.  It owns no graph,
// SCC partition, or copied route row: regions/events borrow only the final
// Causal site and successor references issued during the one seal.
type LocalWTO struct{ result *Result }

func (r *Result) LocalWTO() LocalWTO { return LocalWTO{result: r} }

// WTORegion is an opaque exact-owner schedule region.  Its ID is portable
// semantic content; its Result pointer is the hot replay/owner fence.
type WTORegion struct {
	result *Result
	index  uint32
}

func (region WTORegion) row() (wtoRegion, bool) {
	if region.result == nil || !region.result.available() || uint64(region.index) >= uint64(len(region.result.wto.regions)) {
		return wtoRegion{}, false
	}
	row := region.result.wto.regions[region.index]
	if !row.id.Available() || !row.headerID.Available() || row.hasHeader && uint64(row.header) >= uint64(len(region.result.wto.points)) {
		return wtoRegion{}, false
	}
	return row, true
}

func (region WTORegion) Available() bool { _, ok := region.row(); return ok }
func (region WTORegion) ID() identity.ContentID {
	row, ok := region.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.id
}
func (region WTORegion) ParentID() identity.ContentID {
	row, ok := region.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.parent
}
func (region WTORegion) Cyclic() bool {
	row, ok := region.row()
	return ok && row.cyclic
}
func (region WTORegion) Header() (Site, bool) {
	row, ok := region.row()
	if !ok || !row.hasHeader {
		return Site{}, false
	}
	point, pointOK := region.result.wtoPointAt(int(row.header))
	if !pointOK || point.SiteCount() != 1 {
		return Site{}, false
	}
	return point.SiteAt(0)
}
func (region WTORegion) HeaderPoint() (WTOPoint, bool) {
	row, ok := region.row()
	if !ok || !row.hasHeader {
		return WTOPoint{}, false
	}
	return region.result.wtoPointAt(int(row.header))
}
func (region WTORegion) RouteCount() int {
	row, ok := region.row()
	if !ok {
		return 0
	}
	return len(row.routes)
}
func (region WTORegion) RouteAt(index int) (Successor, bool) {
	row, ok := region.row()
	if !ok || index < 0 || index >= len(row.routes) {
		return Successor{}, false
	}
	return region.result.successorForRef(row.routes[index])
}
func (region WTORegion) SiteCount() int {
	row, ok := region.row()
	if !ok {
		return 0
	}
	return len(row.sites)
}
func (region WTORegion) SiteAt(index int) (Site, bool) {
	row, ok := region.row()
	if !ok || index < 0 || index >= len(row.sites) {
		return Site{}, false
	}
	return region.result.siteAt(int(row.sites[index]))
}
func (region WTORegion) PointCount() int {
	row, ok := region.row()
	if !ok {
		return 0
	}
	return len(row.points)
}
func (region WTORegion) PointAt(index int) (WTOPoint, bool) {
	row, ok := region.row()
	if !ok || index < 0 || index >= len(row.points) {
		return WTOPoint{}, false
	}
	return region.result.wtoPointAt(int(row.points[index]))
}

func (view LocalWTO) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.wto.byID) != len(view.result.wto.regions) {
		return 0
	}
	return len(view.result.wto.regions)
}
func (view LocalWTO) At(index int) (WTORegion, bool) {
	if view.result == nil || index < 0 || index >= view.Count() {
		return WTORegion{}, false
	}
	region := WTORegion{result: view.result, index: uint32(index)}
	return region, region.Available()
}
func (view LocalWTO) Resolve(id identity.ContentID) (WTORegion, bool) {
	if view.result == nil || !id.Available() {
		return WTORegion{}, false
	}
	index, ok := view.result.wto.byID[id]
	if !ok {
		return WTORegion{}, false
	}
	return view.At(int(index))
}
func (view LocalWTO) EventCount() int {
	if view.result == nil || view.Count() == 0 {
		return 0
	}
	return len(view.result.wto.events)
}
func (view LocalWTO) EventAt(index int) (WTOEvent, bool) {
	if view.result == nil || index < 0 || index >= view.EventCount() {
		return WTOEvent{}, false
	}
	event := WTOEvent{result: view.result, index: uint32(index)}
	return event, event.Available()
}

// WTOEvent is one balanced parent-issued schedule action.  Point references
// an existing Site, while Enter/Exit reference an existing WTORegion.
type WTOEvent struct {
	result *Result
	index  uint32
}

func (event WTOEvent) row() (wtoEvent, bool) {
	if event.result == nil || !event.result.available() || uint64(event.index) >= uint64(len(event.result.wto.events)) {
		return wtoEvent{}, false
	}
	row := event.result.wto.events[event.index]
	switch row.kind {
	case WTOEventPoint:
		if uint64(row.point) >= uint64(len(event.result.wto.points)) || row.region != 0 {
			return wtoEvent{}, false
		}
	case WTOEventEnter, WTOEventExit:
		if uint64(row.region) >= uint64(len(event.result.wto.regions)) || row.point != 0 {
			return wtoEvent{}, false
		}
	default:
		return wtoEvent{}, false
	}
	return row, true
}
func (event WTOEvent) Available() bool { _, ok := event.row(); return ok }
func (event WTOEvent) Kind() WTOEventKind {
	row, ok := event.row()
	if !ok {
		return WTOEventInvalid
	}
	return row.kind
}
func (event WTOEvent) Region() (WTORegion, bool) {
	row, ok := event.row()
	if !ok || row.kind == WTOEventPoint {
		return WTORegion{}, false
	}
	return LocalWTO{result: event.result}.At(int(row.region))
}
func (event WTOEvent) Site() (Site, bool) {
	row, ok := event.row()
	if !ok || row.kind != WTOEventPoint {
		return Site{}, false
	}
	point, pointOK := event.result.wtoPointAt(int(row.point))
	if !pointOK || point.SiteCount() != 1 {
		return Site{}, false
	}
	return point.SiteAt(0)
}
func (event WTOEvent) Point() (WTOPoint, bool) {
	row, ok := event.row()
	if !ok || row.kind != WTOEventPoint {
		return WTOPoint{}, false
	}
	return event.result.wtoPointAt(int(row.point))
}

// WTOPoint is Flow's parent-issued local schedule point.  It exists for
// every reachable SourceControl vertex, including vertices with zero Site
// attachments.  Sites remain optional many-valued annotations.
type WTOPoint struct {
	result *Result
	index  uint32
}

func (point WTOPoint) row() (wtoPoint, bool) {
	if point.result == nil || !point.result.available() || uint64(point.index) >= uint64(len(point.result.wto.points)) {
		return wtoPoint{}, false
	}
	row := point.result.wto.points[point.index]
	if !row.path.Available() {
		return wtoPoint{}, false
	}
	for _, site := range row.sites {
		if uint64(site) >= uint64(len(point.result.sites.rows)) {
			return wtoPoint{}, false
		}
	}
	return row, true
}
func (point WTOPoint) Available() bool { _, ok := point.row(); return ok }
func (point WTOPoint) PathID() identity.ContentID {
	row, ok := point.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.path
}
func (point WTOPoint) SiteCount() int {
	row, ok := point.row()
	if !ok {
		return 0
	}
	return len(row.sites)
}
func (point WTOPoint) SiteAt(index int) (Site, bool) {
	row, ok := point.row()
	if !ok || index < 0 || index >= len(row.sites) {
		return Site{}, false
	}
	return point.result.siteAt(int(row.sites[index]))
}
func (r *Result) wtoPointAt(index int) (WTOPoint, bool) {
	if r == nil || index < 0 || index >= len(r.wto.points) {
		return WTOPoint{}, false
	}
	point := WTOPoint{result: r, index: uint32(index)}
	return point, point.Available()
}

func (s Successor) WTORegionID() identity.ContentID {
	if s.result == nil || !s.refValid || !s.ref.wtoRegion.Available() {
		return identity.ContentID{}
	}
	if _, ok := s.result.LocalWTO().Resolve(s.ref.wtoRegion); !ok {
		return identity.ContentID{}
	}
	return s.ref.wtoRegion
}

// installBoundRoutePaths attaches recurrence endpoint rows after the
// combined index is sealed. It performs no Plan/graph lookup.
func (r *Result) installBoundRoutePaths() error {
	if r == nil || (len(r.boundRouteRows) == 0 && len(r.index.refs) != 0) {
		return fmt.Errorf("bound-route phase=directory row=-1 arm=none reason=row-empty")
	}
	canonicalIndexBySlot := make([]int, len(r.routeIndex))
	for index := range canonicalIndexBySlot {
		canonicalIndexBySlot[index] = -1
	}
	for index, ref := range r.index.refs {
		if uint64(ref.routeIndexOrdinal) >= uint64(len(r.routeIndex)) || canonicalIndexBySlot[ref.routeIndexOrdinal] != -1 || !routeRefsEqual(ref, r.routeIndex[ref.routeIndexOrdinal].ref) {
			return fmt.Errorf("bound-route phase=canonical row=%d arm=%d reason=route-index-mismatch", index, ref.arm)
		}
		canonicalIndexBySlot[ref.routeIndexOrdinal] = index
	}
	for index := range r.index.refs {
		ref := &r.index.refs[index]
		if !ref.planOrdinalSet || int(ref.planOrdinal) >= len(r.boundRouteRows) {
			return fmt.Errorf("bound-route phase=index row=%d arm=%d reason=plan-ordinal-missing", index, ref.arm)
		}
		row := r.boundRouteRows[ref.planOrdinal]
		if !row.fromPath.Available() || !row.toPath.Available() {
			return fmt.Errorf("bound-route phase=index row=%d arm=%d reason=row-oob", index, ref.arm)
		}
		ref.fromPoint, ref.toPoint = row.fromPath, row.toPath
		ref.diagnostic = row.diagnostic
		if uint64(ref.routeIndexOrdinal) >= uint64(len(r.routeIndex)) {
			return fmt.Errorf("bound-route phase=index row=%d arm=%d reason=from-to-unavailable", index, ref.arm)
		}
		slot := ref.routeIndexOrdinal
		if !routeRefsEqual(*ref, r.routeIndex[slot].ref) {
			return fmt.Errorf("bound-route phase=index row=%d arm=%d reason=route-index-oob", index, ref.arm)
		}
		r.routeIndex[slot].ref = *ref
	}
	for index := range r.boundaries.rows {
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			if !boundaryArmPresent(r.boundaries.rows[index], arm) {
				continue
			}
			ref := &r.boundaries.rows[index].refs[arm]
			if !ref.planOrdinalSet || int(ref.planOrdinal) >= len(r.boundRouteRows) {
				return fmt.Errorf("bound-route phase=boundary row=%d arm=%d reason=plan-ordinal-missing", index, arm)
			}
			{
				row := r.boundRouteRows[ref.planOrdinal]
				if !row.fromPath.Available() || !row.toPath.Available() {
					return fmt.Errorf("bound-route phase=boundary row=%d arm=%d reason=row-oob", index, arm)
				}
				ref.fromPoint, ref.toPoint = row.fromPath, row.toPath
				if uint64(ref.routeIndexOrdinal) >= uint64(len(r.routeIndex)) {
					return fmt.Errorf("bound-route phase=boundary row=%d arm=%d reason=route-index-oob", index, arm)
				}
				slot := ref.routeIndexOrdinal
				canonical := r.routeIndex[slot].ref
				canonicalIndex := canonicalIndexBySlot[slot]
				if canonicalIndex < 0 || !routeRefsEqual(r.index.refs[canonicalIndex], canonical) || !routeRefsEqual(*ref, canonical) {
					return fmt.Errorf("bound-route phase=boundary row=%d arm=%d reason=canonical-route-mismatch", index, arm)
				}
				if !ref.planOrdinalSet || !canonical.planOrdinalSet || ref.planOrdinal != canonical.planOrdinal {
					return fmt.Errorf("bound-route phase=boundary row=%d arm=%d reason=plan-ordinal-mismatch", index, arm)
				}
				if ref.routeIndexOrdinal != slot || canonical.routeIndexOrdinal != slot || r.index.refs[canonicalIndex].routeIndexOrdinal != slot ||
					!canonical.fromPoint.Available() || !canonical.toPoint.Available() ||
					ref.fromPoint != canonical.fromPoint || ref.toPoint != canonical.toPoint {
					return fmt.Errorf("bound-route phase=boundary row=%d arm=%d reason=endpoint-row-mismatch", index, arm)
				}
				ref.fromPoint, ref.toPoint, ref.diagnostic = canonical.fromPoint, canonical.toPoint, canonical.diagnostic
			}
		}
	}
	for index := range r.index.writeCommitRefs {
		ref := &r.index.writeCommitRefs[index]
		if ref.routeDigest.Available() || ref.planOrdinalSet {
			if uint64(ref.routeIndexOrdinal) >= uint64(len(r.routeIndex)) {
				return fmt.Errorf("bound-route phase=write row=%d arm=%d reason=route-index-oob", index, ref.arm)
			}
			*ref = r.routeIndex[ref.routeIndexOrdinal].ref
		}
	}
	// All aliases now contain the exact canonical ref. Drop every seal-only
	// ordinal before publication so no Plan capability survives in any copy.
	for index := range r.index.refs {
		r.index.refs[index].planOrdinal, r.index.refs[index].planOrdinalSet = 0, false
	}
	for index := range r.routeIndex {
		r.routeIndex[index].ref.planOrdinal, r.routeIndex[index].ref.planOrdinalSet = 0, false
	}
	for row := range r.boundaries.rows {
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			ref := &r.boundaries.rows[row].refs[arm]
			ref.planOrdinal, ref.planOrdinalSet = 0, false
		}
	}
	for index := range r.index.writeCommitRefs {
		r.index.writeCommitRefs[index].planOrdinal, r.index.writeCommitRefs[index].planOrdinalSet = 0, false
	}
	return nil
}

// prepareWTORows converts the recurrence path-only hierarchy and bound route
// endpoint rows into the private dense working planes expected by
// the WTO materializer. No Plan, SourceControl coordinate, or authored term
// is consulted for this conversion.
func (r *Result) prepareWTORows() error {
	if r == nil || r.pendingWTO.Count() == 0 || len(r.pendingNodeSites) != 0 {
		return failWTORow(wtoFailureReasonRowPreflight, -1, -1, -1, -1, -1)
	}
	pathNode := make(map[identity.ContentID]uint32)
	paths := make([]identity.ContentID, 0)
	for index := 0; index < r.pendingWTO.Count(); index++ {
		event, ok := r.pendingWTO.At(index)
		path, pathOK := event.VertexPath()
		if !ok || !pathOK {
			return failWTORow(wtoFailureReasonRowEvent, -1, index, -1, -1, -1)
		}
		if _, exists := pathNode[path]; !exists {
			pathNode[path] = uint32(len(paths))
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return failWTORow(wtoFailureReasonRowPathEmpty, -1, -1, -1, -1, -1)
	}
	sitesByPath := make([][]uint32, len(paths))
	addSite := func(path identity.ContentID, site Site) bool {
		node, exists := pathNode[path]
		if !exists || !site.Available() {
			return false
		}
		for _, existing := range sitesByPath[node] {
			if existing == site.index {
				return true
			}
		}
		sitesByPath[node] = append(sitesByPath[node], site.index)
		return true
	}
	for index, ref := range r.index.refs {
		from := mustSite(r, ref, true)
		if !from.Available() {
			return failWTORowRef(wtoFailureReasonRowRouteFromSite, ref, index, -1, -1)
		}
		fromPath, fromOK := pathNode[ref.fromPoint]
		if !fromOK {
			return failWTORowRef(wtoFailureReasonRowRouteFromPath, ref, index, -1, int(from.index))
		}
		if !addSite(ref.fromPoint, from) {
			return failWTORowRef(wtoFailureReasonRowRouteFromAttach, ref, index, int(fromPath), int(from.index))
		}
		to := mustSite(r, ref, false)
		if !to.Available() {
			return failWTORowRef(wtoFailureReasonRowRouteToSite, ref, index, -1, -1)
		}
		toPath, toOK := pathNode[ref.toPoint]
		if !toOK {
			return failWTORowRef(wtoFailureReasonRowRouteToPath, ref, index, -1, int(to.index))
		}
		if !addSite(ref.toPoint, to) {
			return failWTORowRef(wtoFailureReasonRowRouteToAttach, ref, index, int(toPath), int(to.index))
		}
	}
	for term, path := range r.outcomePhasePaths {
		site, siteOK := r.siteForTerm(term)
		if _, reachable := pathNode[path]; !reachable {
			// Static/unreachable terminal Outcomes remain valid Sites, but have
			// no reachable hierarchy point and therefore are not WTO members.
			continue
		}
		if !siteOK || !addSite(path, site) {
			if !siteOK {
				return failWTORow(wtoFailureReasonRowOutcomeSite, -1, -1, -1, -1, -1)
			}
			return failWTORow(wtoFailureReasonRowOutcomeAttach, -1, -1, int(pathNode[path]), int(site.index), -1)
		}
	}
	r.pendingNodeSites = sitesByPath
	r.pendingVertexPaths = paths
	r.pendingWTORoutes = make([]pendingWTORoute, len(r.index.refs))
	for index, ref := range r.index.refs {
		from, fromOK := pathNode[ref.fromPoint]
		to, toOK := pathNode[ref.toPoint]
		if !fromOK {
			return failWTORowRef(wtoFailureReasonRowRouteFromPath, ref, index, -1, -1)
		}
		if !toOK {
			return failWTORowRef(wtoFailureReasonRowRouteToPath, ref, index, -1, -1)
		}
		r.pendingWTORoutes[index] = pendingWTORoute{from: from, to: to, fromPath: ref.fromPoint, toPath: ref.toPoint}
	}
	// Both seal-local row maps have now been projected into the final WTO working
	// planes; retaining them would create a second endpoint authority.
	r.boundRouteRows = nil
	r.outcomePhasePaths = nil
	// Row diagnostics are valid only while attaching the parent hierarchy.
	// They must not survive as a second route/provenance authority.
	for index := range r.index.refs {
		r.index.refs[index].diagnostic = routeDiagnostic{}
	}
	for index := range r.routeIndex {
		r.routeIndex[index].ref.diagnostic = routeDiagnostic{}
	}
	for row := range r.boundaries.rows {
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			r.boundaries.rows[row].refs[arm].diagnostic = routeDiagnostic{}
		}
	}
	for index := range r.index.writeCommitRefs {
		r.index.writeCommitRefs[index].diagnostic = routeDiagnostic{}
	}
	return nil
}

func mustSite(r *Result, ref successorRef, from bool) Site {
	term, ok := r.successorForRef(ref)
	if !ok {
		return Site{}
	}
	if from {
		returnSite, _ := r.siteForTerm(term.From)
		return returnSite
	}
	returnSite, _ := r.siteForTerm(term.To)
	return returnSite
}

// finalizeLocalWTO is the sole semantic publication cut.  Recurrence's
// private vertex brackets are converted through already-issued Site paths;
// no graph coordinate survives this method.
func (r *Result) finalizeLocalWTO() error {
	if r == nil || !r.available() || r.pendingWTO.Count() == 0 || len(r.pendingNodeSites) == 0 || len(r.pendingWTORoutes) != len(r.index.refs) || r.wto.regions != nil {
		return failWTO(wtoFailurePhasePreflight, wtoFailureReasonPreflight, -1, -1, -1)
	}
	store := wtoStore{byID: make(map[identity.ContentID]uint32), pointByPath: make(map[identity.ContentID]uint32)}
	vertexPaths, pathsOK := r.issuedVertexPaths()
	if !pathsOK {
		return failWTO(wtoFailurePhaseVertexPaths, wtoFailureReasonVertexPaths, -1, -1, -1)
	}
	nodeForPath := make(map[identity.ContentID]uint32, len(vertexPaths))
	for node, path := range vertexPaths {
		nodeForPath[path] = uint32(node)
	}
	const noPoint = ^uint32(0)
	nodeRegion := make([]uint32, len(r.pendingNodeSites))
	pointForNode := make([]uint32, len(r.pendingNodeSites))
	for index := range nodeRegion {
		nodeRegion[index] = ^uint32(0)
		pointForNode[index] = noPoint
	}
	ensurePoint := func(vertex uint32) (uint32, []uint32, wtoFailureReason) {
		if int(vertex) >= len(pointForNode) || !vertexPaths[vertex].Available() {
			return 0, nil, wtoFailureReasonPointUnavailable
		}
		if pointForNode[vertex] != noPoint {
			point := pointForNode[vertex]
			return point, append([]uint32(nil), store.points[point].sites...), wtoFailureReasonInvalid
		}
		sites, _, sitesOK := r.semanticWTOSites(vertex)
		if !sitesOK {
			return 0, nil, wtoFailureReasonPointSiteSet
		}
		path := vertexPaths[vertex]
		if _, exists := store.pointByPath[path]; exists {
			return 0, nil, wtoFailureReasonPointDuplicate
		}
		point := uint32(len(store.points))
		store.points = append(store.points, wtoPoint{path: path, sites: append([]uint32(nil), sites...)})
		store.pointByPath[path] = point
		pointForNode[vertex] = point
		return point, sites, wtoFailureReasonInvalid
	}
	stack := make([]uint32, 0)
	stackVertices := make([]uint32, 0)
	for index := 0; index < r.pendingWTO.Count(); index++ {
		event, ok := r.pendingWTO.At(index)
		if !ok {
			return failWTO(wtoFailurePhaseEvent, wtoFailureReasonEventUnavailable, -1, index, -1)
		}
		path, pathOK := event.VertexPath()
		if !pathOK {
			return failWTO(wtoFailurePhaseEvent, wtoFailureReasonEventPath, -1, index, -1)
		}
		node, nodeOK := nodeForPath[path]
		if !nodeOK || int(node) >= len(r.pendingNodeSites) {
			return failWTO(wtoFailurePhaseEvent, wtoFailureReasonEventNode, -1, index, -1)
		}
		point, sites, pointFailure := ensurePoint(node)
		if pointFailure != wtoFailureReasonInvalid {
			return failWTO(wtoFailurePhasePoint, pointFailure, int(node), index, -1)
		}
		switch event.Kind {
		case recurrence.HierarchyEnter:
			parent := identity.ContentID{}
			if len(stack) != 0 {
				parent = store.regions[stack[len(stack)-1]].id
			}
			id := wtoID(wtoCyclicDomain, digestWTOParent(parent, path))
			if !id.Available() {
				return failWTO(wtoFailurePhaseEvent, wtoFailureReasonEnterID, -1, index, -1)
			}
			if _, exists := store.byID[id]; exists {
				return failWTO(wtoFailurePhaseEvent, wtoFailureReasonEnterDuplicate, -1, index, -1)
			}
			region := uint32(len(store.regions))
			store.regions = append(store.regions, wtoRegion{id: id, parent: parent, headerID: path, header: point, hasHeader: true, cyclic: true, points: []uint32{point}, sites: append([]uint32(nil), sites...)})
			store.byID[id] = region
			store.events = append(store.events, wtoEvent{kind: WTOEventEnter, region: region})
			store.events = append(store.events, wtoEvent{kind: WTOEventPoint, point: point})
			nodeRegion[node] = region
			stack = append(stack, region)
			stackVertices = append(stackVertices, node)
		case recurrence.HierarchyPoint:
			if len(stack) == 0 {
				id := wtoID(wtoSingletonDomain, path)
				if !id.Available() {
					return failWTO(wtoFailurePhaseEvent, wtoFailureReasonSingletonID, -1, index, -1)
				}
				if _, exists := store.byID[id]; exists {
					return failWTO(wtoFailurePhaseEvent, wtoFailureReasonSingletonDuplicate, -1, index, -1)
				}
				region := uint32(len(store.regions))
				store.byID[id] = region
				store.regions = append(store.regions, wtoRegion{id: id, headerID: path, header: point, hasHeader: true, points: []uint32{point}, sites: append([]uint32(nil), sites...)})
				store.events = append(store.events, wtoEvent{kind: WTOEventEnter, region: region})
				store.events = append(store.events, wtoEvent{kind: WTOEventPoint, point: point})
				store.events = append(store.events, wtoEvent{kind: WTOEventExit, region: region})
				nodeRegion[node] = region
				continue
			}
			store.events = append(store.events, wtoEvent{kind: WTOEventPoint, point: point})
			region := &store.regions[stack[len(stack)-1]]
			region.points = append(region.points, point)
			region.sites = append(region.sites, sites...)
			nodeRegion[node] = stack[len(stack)-1]
		case recurrence.HierarchyExit:
			if len(stack) == 0 || len(stackVertices) == 0 || stackVertices[len(stackVertices)-1] != node {
				return failWTO(wtoFailurePhaseBracket, wtoFailureReasonExitBracket, -1, index, -1)
			}
			region := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			stackVertices = stackVertices[:len(stackVertices)-1]
			store.events = append(store.events, wtoEvent{kind: WTOEventExit, region: region})
		default:
			return failWTO(wtoFailurePhaseEvent, wtoFailureReasonEventKind, -1, index, -1)
		}
	}
	if len(stack) != 0 || len(stackVertices) != 0 {
		return failWTO(wtoFailurePhaseBracket, wtoFailureReasonUnclosedBracket, -1, r.pendingWTO.Count(), -1)
	}
	if err := r.classifyWTORoutes(&store, nodeRegion); err != nil {
		return err
	}
	r.pendingWTO = recurrence.HierarchyProof{}
	r.pendingNodeSites = nil
	r.pendingWTORoutes = nil
	r.pendingVertexPaths = nil
	r.wto = store
	return nil
}

// issuedVertexPaths returns SourceControl's already-issued VertexCatalog
// paths.  It deliberately performs no incident-route reconstruction: a
// reachable zero-site/zero-route vertex is still a real LocalWTO point.
func (r *Result) issuedVertexPaths() ([]identity.ContentID, bool) {
	if r == nil || len(r.pendingVertexPaths) != len(r.pendingNodeSites) || len(r.pendingWTORoutes) != len(r.index.refs) {
		return nil, false
	}
	paths := append([]identity.ContentID(nil), r.pendingVertexPaths...)
	seen := make(map[identity.ContentID]struct{}, len(paths))
	for _, path := range paths {
		if !path.Available() {
			return nil, false
		}
		if _, duplicate := seen[path]; duplicate {
			return nil, false
		}
		seen[path] = struct{}{}
	}
	return paths, true
}

// semanticWTOSites turns the exact issued site set for one private graph
// vertex into a canonical semantic sequence.  A site may legitimately occur
// under multiple vertices; only duplicate members of this one vertex set are
// removed.  This is the publication boundary that eliminates raw site-index
// ordering from region headers and event point order.
func (r *Result) semanticWTOSites(vertex uint32) ([]uint32, identity.ContentID, bool) {
	if r == nil || int(vertex) >= len(r.pendingNodeSites) {
		return nil, identity.ContentID{}, false
	}
	sites := append([]uint32(nil), r.pendingNodeSites[vertex]...)
	if len(sites) == 0 {
		return nil, identity.ContentID{}, true
	}
	paths := make(map[uint32]identity.ContentID, len(sites))
	for _, site := range sites {
		if int(site) >= len(r.sites.rows) {
			return nil, identity.ContentID{}, false
		}
		path := r.sites.rows[site].path
		if !path.Available() {
			return nil, identity.ContentID{}, false
		}
		paths[site] = path
	}
	identity.SortByContentID(sites, func(site uint32) identity.ContentID { return paths[site] })
	unique := sites[:0]
	for _, site := range sites {
		if len(unique) != 0 && paths[unique[len(unique)-1]] == paths[site] {
			if unique[len(unique)-1] != site {
				return nil, identity.ContentID{}, false
			}
			continue
		}
		unique = append(unique, site)
	}
	return unique, digestWTOSites(paths, unique), true
}

func digestWTOSites(paths map[uint32]identity.ContentID, sites []uint32) identity.ContentID {
	if len(sites) == 0 {
		return identity.ContentID{}
	}
	var encoded bytes.Buffer
	encoded.WriteString("wippy/program/flow/local-wto-site-set-v1")
	encoded.WriteByte(0)
	for _, site := range sites {
		path, ok := paths[site]
		if !ok || !path.Available() {
			return identity.ContentID{}
		}
		encoded.Write(path[:])
	}
	return identity.ContentID(sha256.Sum256(encoded.Bytes()))
}

// classifyWTORoutes assigns each exact final route once to the lowest
// enclosing published region. A zero membership is the explicit top-level or
// cross-region disposition. An iterative offline Tarjan pass answers every
// LCA together in O(C+R), never walking parent chains per route.
func (r *Result) classifyWTORoutes(store *wtoStore, nodeRegion []uint32) error {
	if r == nil || store == nil || len(store.regions) == 0 || len(nodeRegion) != len(r.pendingNodeSites) || len(r.pendingWTORoutes) != len(r.index.refs) {
		return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyPreflight, -1, -1, -1)
	}
	const none = recurrence.NoNode
	parents := make([]uint32, len(store.regions))
	roots := make([]uint32, len(store.regions))
	for index, row := range store.regions {
		parents[index] = none
		roots[index] = uint32(index)
		if row.parent.Available() {
			parent, ok := store.byID[row.parent]
			if !ok || parent >= uint32(index) {
				return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyParent, index, -1, -1)
			}
			parents[index] = parent
			roots[index] = roots[parent]
		}
	}
	left := make([]uint32, len(r.index.refs))
	right := make([]uint32, len(r.index.refs))
	for index, nodes := range r.pendingWTORoutes {
		if int(nodes.from) >= len(nodeRegion) || int(nodes.to) >= len(nodeRegion) {
			return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyEndpoint, -1, -1, index)
		}
		left[index], right[index] = nodeRegion[nodes.from], nodeRegion[nodes.to]
		if left[index] == none || right[index] == none || int(left[index]) >= len(store.regions) || int(right[index]) >= len(store.regions) {
			return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyMembership, -1, -1, index)
		}
	}
	lcas := recurrence.OfflineLCAs(parents, roots, left, right)
	if len(lcas) != len(r.index.refs) {
		return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyLCA, -1, -1, -1)
	}
	membership := make(map[identity.ContentID]identity.ContentID, len(r.index.refs))
	for index, ref := range r.index.refs {
		if !ref.routeDigest.Available() || !ref.semanticPath.Available() {
			return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyRoute, -1, -1, index)
		}
		region := lcas[index]
		id := identity.ContentID{}
		if region != none {
			id = store.regions[region].id
			if !id.Available() {
				return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyMembership, int(region), -1, index)
			}
		}
		if _, duplicate := membership[ref.routeDigest]; duplicate {
			return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyMembership, -1, -1, index)
		}
		membership[ref.routeDigest] = id
		r.index.refs[index].wtoRegion = id
		ref.wtoRegion = id
		if region != none {
			store.regions[region].routes = append(store.regions[region].routes, ref)
		}
		if !ref.local {
			if uint64(ref.index) >= uint64(len(r.boundaries.rows)) || !isBoundaryArm(ref.arm) {
				return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyBoundary, int(ref.index), -1, index)
			}
			r.boundaries.rows[ref.index].refs[ref.arm].wtoRegion = id
		}
	}
	for index := range r.routeIndex {
		id, present := membership[r.routeIndex[index].ref.routeDigest]
		if !present {
			return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyRoute, index, -1, -1)
		}
		r.routeIndex[index].ref.wtoRegion = id
	}
	for index, ref := range r.index.writeCommitRefs {
		if !ref.routeDigest.Available() {
			if ref != (successorRef{}) {
				return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyWrite, index, -1, -1)
			}
			continue
		}
		id, present := membership[ref.routeDigest]
		if !present || !ref.local || !isLocalArm(ref.arm) {
			return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyWrite, index, -1, -1)
		}
		r.index.writeCommitRefs[index].wtoRegion = id
	}
	for index := range store.regions {
		routes := store.regions[index].routes
		identity.SortByContentID(routes, successorRefSemanticPath)
		for routeIndex := 1; routeIndex < len(routes); routeIndex++ {
			if routes[routeIndex-1].semanticPath == routes[routeIndex].semanticPath {
				return failWTO(wtoFailurePhaseClassify, wtoFailureReasonClassifyOrder, index, -1, routeIndex)
			}
		}
		store.regions[index].routes = routes
	}
	return nil
}

func successorRefSemanticPath(route successorRef) identity.ContentID { return route.semanticPath }

func digestWTOParent(parent, path identity.ContentID) identity.ContentID {
	if !path.Available() {
		return identity.ContentID{}
	}
	if !parent.Available() {
		return path
	}
	var encoded [64]byte
	copy(encoded[:32], parent[:])
	copy(encoded[32:], path[:])
	return identity.ContentID(sha256.Sum256(encoded[:]))
}
