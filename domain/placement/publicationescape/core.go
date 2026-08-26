package publicationescape

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type sourceTag uint64
type routeTag uint64

type sourceSpec struct {
	tag        sourceTag
	rowID      identity.ContentID
	operation  vocabulary.Operation
	member     int
	coordinate valuedomain.Coordinate
}

type publicationRow struct {
	id          identity.ContentID
	requirement placementdomain.Placement
	subjectOpen bool
	// subjectNil records Pack's proven-nil subject: the mounted call authors no
	// fixed actual at the descriptor's formal position and no actual tail can
	// reach it.  A nil subject reaches no allocation root, so the row routes
	// nothing.  It is mutually exclusive with subjectOpen.
	subjectNil bool
	// subjectEmpty records Pack's empty-value-list subject: a ValuesVar or
	// AllInputs projection that selects no member on a closed mounted call.
	// The parent publishes this as a resolved fact about the call, distinct
	// from the proven-nil ValueFormal and from a tail-fed unknown, so the row
	// carries no allocation root and routes nothing. It is mutually exclusive
	// with subjectOpen and subjectNil.
	subjectEmpty bool
	operation    vocabulary.Operation
}

type preparedBatch struct {
	batch   effectfactor.MountedPublicationBatch
	id      identity.ContentID
	module  identity.ContentID
	call    identity.ContentID
	rows    []publicationRow
	sources []sourceSpec
	byTag   map[sourceTag]sourceSpec
	// prepared is set only after the complete Effect-owned row/source walk has
	// succeeded.  Route planning also accepts small unit-test plans assembled
	// directly in this package, so this bit lets the hot path distinguish those
	// fixtures from an authenticated mounted batch when it needs the stronger
	// source/cardinality checks.
	prepared bool
}

const (
	inlineOperationCapacity = 8
	inlineSourceCapacity    = 8
	inlineFactCapacity      = 8
	inlineKeyCapacity       = 8
	inlineRouteCapacity     = 8
)

type operationGate struct {
	inline   [inlineOperationCapacity]vocabulary.Operation
	count    int
	overflow []vocabulary.Operation
	opaque   bool
}

func (gate operationGate) admits(operation vocabulary.Operation) bool {
	for index := 0; index < gate.count && index < inlineOperationCapacity; index++ {
		if gate.inline[index] == operation {
			return true
		}
	}
	for _, candidate := range gate.overflow {
		if candidate == operation {
			return true
		}
	}
	return false
}

func (gate *operationGate) add(operation vocabulary.Operation) {
	if gate == nil || operation == 0 || gate.admits(operation) {
		return
	}
	if gate.count < inlineOperationCapacity {
		gate.inline[gate.count] = operation
		gate.count++
		return
	}
	gate.overflow = append(gate.overflow, operation)
	gate.count++
}

type plannedRoute struct {
	key      heapdomain.Key
	tag      routeTag
	required placementdomain.Placement
	unknown  bool
}

type sourceView struct {
	inline   [inlineSourceCapacity]sourceSpec
	count    int
	overflow []sourceSpec
	byTag    map[sourceTag]sourceSpec
	gate     operationGate
}

func (view *sourceView) add(source sourceSpec) {
	if view == nil {
		return
	}
	if view.count < inlineSourceCapacity {
		view.inline[view.count] = source
		view.count++
		return
	}
	view.overflow = append(view.overflow, source)
	view.count++
}

func (view sourceView) len() int { return view.count }

func (view sourceView) at(index int) (sourceSpec, bool) {
	if index < 0 || index >= view.count {
		return sourceSpec{}, false
	}
	if index < inlineSourceCapacity {
		return view.inline[index], true
	}
	overflow := index - inlineSourceCapacity
	if overflow < 0 || overflow >= len(view.overflow) {
		return sourceSpec{}, false
	}
	return view.overflow[overflow], true
}

func (view sourceView) find(tag sourceTag) (sourceSpec, bool) {
	if view.byTag != nil {
		source, found := view.byTag[tag]
		if found && view.gate.admits(source.operation) {
			return source, true
		}
		return sourceSpec{}, false
	}
	for index := 0; index < view.count; index++ {
		source, _ := view.at(index)
		if source.tag == tag {
			return source, true
		}
	}
	return sourceSpec{}, false
}

type factEntry struct {
	rowID   identity.ContentID
	value   valuedomain.Value
	present bool
}

type factBuffer struct {
	inline   [inlineFactCapacity]factEntry
	count    int
	overflow []factEntry
}

func (facts *factBuffer) set(entry factEntry) bool {
	if facts == nil {
		return false
	}
	for index := 0; index < facts.count; index++ {
		prior, _ := facts.at(index)
		if prior.rowID == entry.rowID {
			return false
		}
	}
	if facts.count < inlineFactCapacity {
		facts.inline[facts.count] = entry
		facts.count++
		return true
	}
	facts.overflow = append(facts.overflow, entry)
	facts.count++
	return true
}

// merge folds exact fixed members of one heterogeneous publication subject.
// Every member is observed independently.  An owner-authenticated sparse
// Bottom is neutral to the aggregate; if every member is sparse the aggregate
// remains absent/Bottom, while present members join under Value's algebra.
func (facts *factBuffer) merge(schema *valuedomain.Schema, entry factEntry) bool {
	if facts == nil || schema == nil || !schema.Valid() || !entry.rowID.Available() {
		return false
	}
	// An absent cell is a valid sparse Bottom only when the Value authority
	// actually supplied that Bottom.  A zero/foreign value must not be allowed
	// to masquerade as an absent member and later disappear from the route set.
	if entry.present {
		if !schema.Equal(entry.value, entry.value) {
			return false
		}
	} else if !schema.Equal(entry.value, schema.Bottom()) {
		return false
	}
	for index := 0; index < facts.count; index++ {
		prior, priorOK := facts.at(index)
		if !priorOK {
			return false
		}
		if prior.rowID != entry.rowID {
			continue
		}
		if !entry.present {
			// The owner-issued sparse Bottom is the neutral element of a
			// heterogeneous subject join.  It must not erase a present member
			// that was already authenticated for this publication row.
			return true
		}
		if !prior.present {
			// An authenticated sparse Bottom contributes no Value atoms; the
			// first present member therefore becomes the aggregate fact.
			prior.value = entry.value
			prior.present = true
			return facts.setAt(index, prior)
		}
		joined, ok := schema.Join(prior.value, entry.value)
		if !ok {
			return false
		}
		prior.value = joined
		return facts.setAt(index, prior)
	}
	return facts.set(entry)
}

func (facts *factBuffer) setAt(index int, entry factEntry) bool {
	if facts == nil || index < 0 || index >= facts.count {
		return false
	}
	if index < len(facts.inline) {
		facts.inline[index] = entry
		return true
	}
	overflow := index - len(facts.inline)
	if overflow < 0 || overflow >= len(facts.overflow) {
		return false
	}
	facts.overflow[overflow] = entry
	return true
}

func (facts factBuffer) at(index int) (factEntry, bool) {
	if index < 0 || index >= facts.count {
		return factEntry{}, false
	}
	if index < inlineFactCapacity {
		return facts.inline[index], true
	}
	overflow := index - inlineFactCapacity
	if overflow < 0 || overflow >= len(facts.overflow) {
		return factEntry{}, false
	}
	return facts.overflow[overflow], true
}

func (facts factBuffer) get(rowID identity.ContentID) (valuedomain.Value, bool, bool) {
	for index := 0; index < facts.count; index++ {
		entry, _ := facts.at(index)
		if entry.rowID == rowID {
			return entry.value, entry.present, true
		}
	}
	return valuedomain.Value{}, false, false
}

func (facts factBuffer) valid(schema *valuedomain.Schema) bool {
	if facts.count < 0 || schema == nil || !schema.Valid() {
		return false
	}
	for index := 0; index < facts.count; index++ {
		entry, entryOK := facts.at(index)
		if !entryOK || !entry.rowID.Available() {
			return false
		}
		if entry.present {
			if !schema.Equal(entry.value, entry.value) {
				return false
			}
			continue
		}
		if !schema.Equal(entry.value, schema.Bottom()) {
			return false
		}
	}
	return true
}

type keyBuffer struct {
	inline   [inlineKeyCapacity]heapdomain.Key
	count    int
	overflow []heapdomain.Key
}

func (keys *keyBuffer) add(key heapdomain.Key) {
	if keys == nil {
		return
	}
	for index := 0; index < keys.count; index++ {
		prior, _ := keys.at(index)
		if prior == key {
			return
		}
	}
	if keys.count < inlineKeyCapacity {
		keys.inline[keys.count] = key
		keys.count++
		return
	}
	keys.overflow = append(keys.overflow, key)
	keys.count++
}

func (keys keyBuffer) len() int { return keys.count }

func (keys keyBuffer) at(index int) (heapdomain.Key, bool) {
	if index < 0 || index >= keys.count {
		return heapdomain.Key{}, false
	}
	if index < inlineKeyCapacity {
		return keys.inline[index], true
	}
	overflow := index - inlineKeyCapacity
	if overflow < 0 || overflow >= len(keys.overflow) {
		return heapdomain.Key{}, false
	}
	return keys.overflow[overflow], true
}

type routeBuffer struct {
	inline   [inlineRouteCapacity]plannedRoute
	count    int
	overflow []plannedRoute

	// allRoot is a lazy owner-schema route plan.  The schema is already the
	// Placement authority's immutable Heap projection, so retaining it here
	// carries the complete allocation-root denominator without copying that
	// catalogue into a route slice.  count/overflow remain the exact-route
	// override set when an all-root plan is joined with exact Value roots.
	allRoot       bool
	allRootSchema placementdomain.Schema
	allRootCount  int
	// allRootPrefix is set only after the owner schema has been validated as
	// having every allocation root in dense prefix order.  A defensive
	// non-prefix schema uses allRootAt's sparse scan instead; no root index is
	// copied for that exceptional shape.
	allRootPrefix   bool
	allRootRequired placementdomain.Placement
}

func (routes *routeBuffer) at(index int) (plannedRoute, bool) {
	if routes == nil || index < 0 {
		return plannedRoute{}, false
	}
	if routes.allRoot {
		if index >= routes.allRootCount {
			return plannedRoute{}, false
		}
		return routes.allRootAt(index)
	}
	if index >= routes.count {
		return plannedRoute{}, false
	}
	return routes.exactAt(index)
}

// exactAt reads only the bounded exact Value route set.  It deliberately does
// not use at: in lazy all-root mode at indexes the owner schema's roots,
// whereas add/set must index the exact override storage.
func (routes *routeBuffer) exactAt(index int) (plannedRoute, bool) {
	if routes == nil || index < 0 || index >= routes.count {
		return plannedRoute{}, false
	}
	if routes.overflow != nil {
		if index >= len(routes.overflow) {
			return plannedRoute{}, false
		}
		return routes.overflow[index], true
	}
	if index < inlineRouteCapacity {
		return routes.inline[index], true
	}
	return plannedRoute{}, false
}

func (routes routeBuffer) len() int {
	if routes.allRoot {
		return routes.allRootCount
	}
	return routes.count
}

// allRootAt lazily projects the index-th allocation root from the owner
// schema.  Exact routes are retained as small overrides and joined only for
// the root they name; this preserves the ordinary routeBuffer's monotone
// merge when an all-root policy and an exact Value fact overlap.
func (routes *routeBuffer) allRootAt(index int) (plannedRoute, bool) {
	if routes == nil || !routes.allRoot || index < 0 || index >= routes.allRootCount || !routes.allRootSchema.Valid() {
		return plannedRoute{}, false
	}
	if routes.allRootPrefix {
		key, keyOK := routes.allRootSchema.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			return plannedRoute{}, false
		}
		tag, tagOK := routeTagFor(routes.allRootSchema, key)
		if !tagOK {
			return plannedRoute{}, false
		}
		return routes.allRootRoute(key, tag)
	}
	rootIndex := 0
	for dense := 0; dense < routes.allRootSchema.DenseKeyCount(); dense++ {
		key, keyOK := routes.allRootSchema.KeyAt(dense)
		if !keyOK {
			return plannedRoute{}, false
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		tag, tagOK := routeTagFor(routes.allRootSchema, key)
		if !tagOK {
			return plannedRoute{}, false
		}
		if rootIndex != index {
			rootIndex++
			continue
		}
		return routes.allRootRoute(key, tag)
	}
	return plannedRoute{}, false
}

func (routes *routeBuffer) allRootRoute(key heapdomain.Key, tag routeTag) (plannedRoute, bool) {
	if routes == nil || !routes.allRoot || !key.Valid() || tag == 0 {
		return plannedRoute{}, false
	}
	route := plannedRoute{
		key:      key,
		tag:      tag,
		required: routes.allRootRequired,
	}
	for overrideIndex := 0; overrideIndex < routes.count; overrideIndex++ {
		override, overrideOK := routes.exactAt(overrideIndex)
		if !overrideOK {
			return plannedRoute{}, false
		}
		if override.key == key {
			route = mergeRoute(route, override)
			break
		}
	}
	return route, true
}

// find resolves a routed tag without walking all roots.  routeTag is the
// canonical dense Placement coordinate, so an all-root plan can validate and
// project the selected owner row directly from the schema.
func (routes *routeBuffer) find(tag routeTag) (plannedRoute, bool) {
	if routes == nil || tag == 0 {
		return plannedRoute{}, false
	}
	if !routes.allRoot {
		for index := 0; index < routes.count; index++ {
			candidate, candidateOK := routes.exactAt(index)
			if candidateOK && candidate.tag == tag {
				return candidate, true
			}
		}
		return plannedRoute{}, false
	}
	if !routes.allRootSchema.Valid() {
		return plannedRoute{}, false
	}
	dense64 := uint64(tag) - 1
	if dense64 >= uint64(routes.allRootSchema.DenseKeyCount()) {
		return plannedRoute{}, false
	}
	dense := int(dense64)
	key, keyOK := routes.allRootSchema.KeyAt(dense)
	if !keyOK || key.Kind() != heapdomain.RootAllocation {
		return plannedRoute{}, false
	}
	canonicalTag, canonicalOK := routeTagFor(routes.allRootSchema, key)
	if !canonicalOK || canonicalTag != tag {
		return plannedRoute{}, false
	}
	return routes.allRootRoute(key, tag)
}

func (routes *routeBuffer) set(index int, route plannedRoute) bool {
	if routes == nil || index < 0 || index >= routes.count {
		return false
	}
	if routes.overflow != nil {
		if index >= len(routes.overflow) {
			return false
		}
		routes.overflow[index] = route
	} else if index < inlineRouteCapacity {
		routes.inline[index] = route
	} else {
		return false
	}
	return true
}

func (routes *routeBuffer) add(candidate plannedRoute) {
	if routes == nil {
		return
	}
	for index := 0; index < routes.count; index++ {
		prior, _ := routes.exactAt(index)
		if prior.key == candidate.key {
			routes.set(index, mergeRoute(prior, candidate))
			return
		}
	}
	insert := routes.count
	for index := 0; index < routes.count; index++ {
		prior, _ := routes.exactAt(index)
		if candidate.tag < prior.tag {
			insert = index
			break
		}
	}
	if routes.count < inlineRouteCapacity {
		for index := routes.count; index > insert; index-- {
			prior, _ := routes.exactAt(index - 1)
			routes.inline[index] = prior
		}
		routes.inline[insert] = candidate
		routes.count++
		return
	}
	if routes.overflow == nil {
		routes.overflow = make([]plannedRoute, inlineRouteCapacity, inlineRouteCapacity*2)
		copy(routes.overflow, routes.inline[:])
	}
	routes.overflow = append(routes.overflow, plannedRoute{})
	for index := routes.count; index > insert; index-- {
		routes.overflow[index] = routes.overflow[index-1]
	}
	routes.overflow[insert] = candidate
	routes.count++
}

// requirementForEscape deliberately does not inspect MountedInput context.
// A destination formal is not an authenticated actor-equivalence proof.
func requirementForEscape(escape vocabulary.PublicationEscapeDisposition) (placementdomain.Placement, bool) {
	switch escape {
	case vocabulary.PublicationEscapeReturn, vocabulary.PublicationEscapeCallback:
		return placementdomain.OwnedHeap, true
	case vocabulary.PublicationEscapeSendTransfer:
		return placementdomain.SharedHeap, true
	default:
		// Freeze, Mutation, Release, and None are outside this rule's scope.
		return placementdomain.Bottom, false
	}
}

func sourceTagFor(id identity.ContentID) (sourceTag, bool) {
	return sourceTagForMember(id, 0)
}

func sourceTagForMember(id identity.ContentID, member int) (sourceTag, bool) {
	if !id.Available() || member < 0 {
		return 0, false
	}
	const domain = "wippy.analysis.placement.publication-escape.source.v1\x00"
	if member == 0 {
		var preimage [len(domain) + 32]byte
		copy(preimage[:], domain)
		copy(preimage[len(domain):], id[:])
		digest := sha256.Sum256(preimage[:])
		tag := binary.BigEndian.Uint64(digest[:8])
		if tag == 0 {
			tag = 1
		}
		return sourceTag(tag), true
	}
	var preimage [len(domain) + 32 + 4]byte
	copy(preimage[:], domain)
	copy(preimage[len(domain):], id[:])
	binary.BigEndian.PutUint32(preimage[len(domain)+32:], uint32(member))
	digest := sha256.Sum256(preimage[:])
	tag := binary.BigEndian.Uint64(digest[:8])
	if tag == 0 {
		tag = 1
	}
	return sourceTag(tag), true
}

func routeTagFor(schema placementdomain.Schema, key heapdomain.Key) (routeTag, bool) {
	if !schema.Valid() || !key.Valid() || key.Kind() != heapdomain.RootAllocation || !schema.Heap().OwnsKey(key) {
		return 0, false
	}
	index, ok := schema.Heap().AllocationKeyIndex(key)
	if !ok || index < 0 || uint64(index) >= uint64(^uint32(0)) {
		return 0, false
	}
	canonical, canonicalOK := schema.KeyAt(index)
	if !canonicalOK || canonical != key {
		return 0, false
	}
	return routeTag(uint64(index) + 1), true
}

// broadcastAllRoots preserves a known publication placement requirement while
// widening only the allocation identity. An open MountedInput or a Value Top /
// opaque-reference fact therefore broadcasts Send as SharedHeap (or Return /
// Callback as OwnedHeap) instead of manufacturing a Placement.Unknown policy.
func broadcastAllRoots(schema placementdomain.Schema, requirement placementdomain.Placement) (routeBuffer, bool) {
	if !validRequirement(requirement) {
		return routeBuffer{}, false
	}
	return allRootsWithRequirement(schema, requirement)
}

// allRootsWithRequirement validates the complete owner denominator once, then
// retains only the owner schema and the policy metadata.  It deliberately does
// not issue one plannedRoute per allocation root: exact Value roots remain the
// only routes that use routeBuffer's inline/spill catalogue.
func allRootsWithRequirement(schema placementdomain.Schema, requirement placementdomain.Placement) (routeBuffer, bool) {
	var routes routeBuffer
	if !schema.Valid() {
		return routes, false
	}
	if !validRequirement(requirement) {
		return routes, false
	}
	rootCount := 0
	prefixCount := 0
	prefix := true
	for dense := 0; dense < schema.DenseKeyCount(); dense++ {
		key, ok := schema.KeyAt(dense)
		if !ok {
			return routeBuffer{}, false
		}
		if key.Kind() != heapdomain.RootAllocation {
			prefix = false
			continue
		}
		if _, ok := routeTagFor(schema, key); !ok {
			return routeBuffer{}, false
		}
		rootCount++
		if prefix {
			prefixCount++
		}
	}
	routes.allRoot = true
	routes.allRootSchema = schema
	routes.allRootCount = rootCount
	routes.allRootPrefix = prefixCount == rootCount
	routes.allRootRequired = requirement
	return routes, true
}

// mergeAllRoot joins a lazy all-root policy into the current route plan.  The
// exact route storage is intentionally retained as overrides: if an exact
// Value root has a stronger requirement than an all-root default, at/find
// joins it for that root only instead of widening the entire catalogue.
func (routes *routeBuffer) mergeAllRoot(candidate routeBuffer) bool {
	if routes == nil || !candidate.allRoot || !candidate.allRootSchema.Valid() {
		return false
	}
	if !routes.allRoot {
		routes.allRoot = true
		routes.allRootSchema = candidate.allRootSchema
		routes.allRootCount = candidate.allRootCount
		routes.allRootPrefix = candidate.allRootPrefix
		routes.allRootRequired = candidate.allRootRequired
		return true
	}
	if routes.allRootSchema != candidate.allRootSchema || routes.allRootCount != candidate.allRootCount || routes.allRootPrefix != candidate.allRootPrefix {
		return false
	}
	joined, joinedOK := placementdomain.JoinChecked(routes.allRootRequired, candidate.allRootRequired)
	if !joinedOK {
		return false
	}
	routes.allRootRequired = joined
	return true
}

// rootsForValue remains local because publicationescape owns its route-buffer
// and escape requirement. The Value-to-allocation-root authentication itself
// is Placement-owned and shared with the other route consumers.
func rootsForValue(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value) (roots keyBuffer, unknown, ok bool) {
	projection, projectionOK := placementdomain.ProjectValueAllocations(schema, values, fact)
	if !projectionOK {
		return roots, false, false
	}
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		if !keyOK {
			return keyBuffer{}, false, false
		}
		roots.add(key)
	}
	return roots, projection.Widened(), true
}

func mergeRoute(prior, candidate plannedRoute) plannedRoute {
	if prior.unknown || candidate.unknown {
		prior.required = placementdomain.Unknown
		prior.unknown = true
		return prior
	}
	prior.required = placementdomain.Join(prior.required, candidate.required)
	return prior
}

func (rule *HotRule) operationGateForBatch(batch *preparedBatch, value calldomain.Value) (operationGate, bool) {
	if rule == nil || batch == nil || rule.calls == nil || rule.calls.Algebra() == nil {
		return operationGate{}, false
	}
	key, keyOK := rule.callKeyForBatch(batch.batch)
	if !keyOK || !rule.calls.Algebra().Admits(key, value) {
		return operationGate{}, false
	}
	gate := operationGate{}
	if value.IsTop() {
		gate.opaque = true
		return gate, true
	}
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		if !targetOK {
			return operationGate{}, false
		}
		operation, operationKind := rule.calls.Algebra().ClassifyTargetOperation(target)
		switch operationKind {
		case calldomain.TargetOperationInvalid:
			return operationGate{}, false
		case calldomain.TargetOperationNone:
			// Body and non-operation seed alternatives are authenticated Call
			// alternatives but do not authorize an Effect publication row.
			continue
		case calldomain.TargetOperationPresent:
			gate.add(operation)
		}
	}
	gate.opaque = value.IsOpen()
	return gate, true
}

func (rule *HotRule) prepareBatch(batch effectfactor.MountedPublicationBatch) (*preparedBatch, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || !rule.values.Schema().Valid() || !batch.Valid() {
		return nil, false
	}
	id, idOK := batch.SealedContentID()
	module, call, provenanceOK := batch.CallProvenance()
	if !idOK || !id.Available() || !provenanceOK || !module.Available() || !call.Available() {
		return nil, false
	}
	prepared := &preparedBatch{batch: batch, id: id, module: module, call: call, byTag: make(map[sourceTag]sourceSpec)}
	seen := make(map[identity.ContentID]struct{})
	for _, publication := range batch.Rows() {
		if !publication.Valid() {
			return nil, false
		}
		rowModule, rowCall, rowOK := publication.CallProvenance()
		rowID, rowIDOK := publication.ContentID()
		if !rowOK || rowModule != module || rowCall != call || !rowIDOK || !rowID.Available() {
			return nil, false
		}
		if _, duplicate := seen[rowID]; duplicate {
			return nil, false
		}
		seen[rowID] = struct{}{}
		requirement, routed := requirementForEscape(publication.Escape())
		if !routed {
			continue
		}
		operation := publication.Operation()
		if operation == 0 {
			return nil, false
		}
		subject, subjectOK := publication.SubjectInput()
		if !subjectOK || !subject.Valid() {
			return nil, false
		}
		hasContext := false
		context, contextPresent := publication.ContextInput()
		if contextPresent {
			if !context.Valid() {
				return nil, false
			}
			hasContext = true
		}
		if requirement == placementdomain.SharedHeap && !hasContext {
			return nil, false
		}
		if requirement == placementdomain.OwnedHeap && hasContext {
			return nil, false
		}
		row := publicationRow{id: rowID, requirement: requirement, operation: operation}
		if subject.IsOpen() {
			row.subjectOpen = true
		}
		if subject.IsProvenNil() {
			row.subjectNil = true
		}
		if !subject.IsOpen() && !subject.IsProvenNil() && subject.MemberCount() == 0 {
			row.subjectEmpty = true
		}
		for member := 0; member < subject.MemberCount(); member++ {
			coordinate, coordinateOK := packtransfer.CoordinateForInputMember(rule.values.Schema(), subject, member)
			tag, tagOK := sourceTagForMember(rowID, member)
			if !coordinateOK || !tagOK {
				return nil, false
			}
			prepared.sources = append(prepared.sources, sourceSpec{tag: tag, rowID: rowID, operation: operation, member: member, coordinate: coordinate})
		}
		// Context is validated and mapped for the future actor-equivalence
		// proof, but it never changes this rule's route policy and is not a
		// selected Value predecessor in this first rule.  A proven-nil or
		// tail-fed destination therefore has no member to map and leaves the
		// requirement at its conservative escape disposition.
		if hasContext {
			for member := 0; member < context.MemberCount(); member++ {
				if _, coordinateOK := packtransfer.CoordinateForInputMember(rule.values.Schema(), context, member); !coordinateOK {
					return nil, false
				}
			}
		}
		prepared.rows = append(prepared.rows, row)
	}
	for _, source := range prepared.sources {
		if _, duplicate := prepared.byTag[source.tag]; duplicate {
			return nil, false
		}
		prepared.byTag[source.tag] = source
	}
	prepared.prepared = true
	return prepared, true
}

func (prepared *preparedBatch) sourcesForGate(gate operationGate) sourceView {
	if prepared == nil {
		return sourceView{}
	}
	sources := sourceView{byTag: prepared.byTag, gate: gate}
	for _, source := range prepared.sources {
		// Opaque Call alternatives do not select a Value source. Their
		// publication rows have no typed Effect receipt, so no source or
		// coordinate is fabricated here; only exact known operation rows enter
		// the Value predecessor lane.
		if !gate.admits(source.operation) {
			continue
		}
		sources.add(source)
	}
	return sources
}

func (rule *HotRule) callKeyForBatch(batch effectfactor.MountedPublicationBatch) (calldomain.Key, bool) {
	if rule == nil || rule.calls == nil || rule.calls.Algebra() == nil || !rule.calls.Algebra().Valid() {
		return calldomain.Key{}, false
	}
	module, occurrence, ok := batch.CallProvenance()
	if !ok {
		return calldomain.Key{}, false
	}
	algebra := rule.calls.Algebra()
	_, key, keyOK := algebra.MountedCallKeyForOccurrence(module, occurrence)
	return key, keyOK
}

func (rule *HotRule) callValueSelector(context engine.SelectorContext, batch effectfactor.MountedPublicationBatch) (calldomain.Value, bool, bool) {
	if rule == nil || rule.calls == nil || rule.calls.Algebra() == nil {
		return calldomain.Value{}, false, false
	}
	cells, readable := engine.SelectorRead(context, rule.callRead)
	if !readable || cells.Count() != 1 {
		return calldomain.Value{}, false, false
	}
	value, present, available := cells.At(0)
	key, keyOK := rule.callKeyForBatch(batch)
	if !available || !keyOK {
		return calldomain.Value{}, false, false
	}
	return admitCallCell(rule.calls.Algebra(), key, value, present)
}

func (rule *HotRule) callValueFrame(frame engine.Frame[placementdomain.Fact, effectfactor.MountedPublicationBatch], batch effectfactor.MountedPublicationBatch) (calldomain.Value, bool, bool) {
	if rule == nil || rule.calls == nil || rule.calls.Algebra() == nil {
		return calldomain.Value{}, false, false
	}
	cells, readable := engine.ReadValue(frame, rule.callRead)
	if !readable || cells.Count() != 1 {
		return calldomain.Value{}, false, false
	}
	value, present, available := cells.At(0)
	key, keyOK := rule.callKeyForBatch(batch)
	if !available || !keyOK {
		return calldomain.Value{}, false, false
	}
	return admitCallCell(rule.calls.Algebra(), key, value, present)
}

// admitCallCell authenticates the exact typed Call predecessor before any
// consumer treats its sparse bit as semantic state.  Call's owner supplies its
// Factor Default in an absent observation; this rule accepts that sparse form
// only when the observed value is equal under the same Algebra to its exact
// Bottom.  A missing/malformed row has no value to authenticate and refuses;
// this helper never manufactures Bottom from the read metadata.
func admitCallCell(algebra *calldomain.Algebra, key calldomain.Key, value calldomain.Value, present bool) (calldomain.Value, bool, bool) {
	if algebra == nil || !algebra.Valid() || !key.Valid() || !algebra.Admits(key, value) {
		return calldomain.Value{}, false, false
	}
	if !present && !algebra.Equal(value, algebra.Bottom()) {
		return calldomain.Value{}, false, false
	}
	return value, present, true
}

func (rule *HotRule) locateValues(context engine.SelectorContext, batch effectfactor.MountedPublicationBatch) bool {
	value, present, callOK := rule.callValueSelector(context, batch)
	if !callOK {
		return false
	}
	prepared := rule.preparedFor(batch)
	if prepared == nil || rule.values == nil {
		return false
	}
	if !present {
		return true
	}
	gate, gateOK := rule.operationGateForBatch(prepared, value)
	if !gateOK {
		return false
	}
	sources := prepared.sourcesForGate(gate)
	for index := 0; index < sources.len(); index++ {
		source, sourceOK := sources.at(index)
		if !sourceOK {
			return false
		}
		if !valueowner.SelectRouteTyped(rule.values, context, source.coordinate, source.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) collectFacts(context engine.SelectorContext, sources sourceView, selection engine.Selection[sourceTag, engine.OrderedCells[valuedomain.Value]]) (factBuffer, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return factBuffer{}, false
	}
	count, countOK := engine.SelectorSelectionCount(context, selection)
	if !countOK || count != sources.len() {
		return factBuffer{}, false
	}
	var facts factBuffer
	for index := 0; index < count; index++ {
		tag, cells, selected := engine.SelectorSelectionAt(context, selection, index)
		source, sourceOK := sources.find(tag)
		if !selected || !sourceOK || cells.Count() != 1 {
			return factBuffer{}, false
		}
		if !rule.values.Schema().AdmitsCoordinate(source.coordinate, rule.values.Schema().Bottom()) || source.tag == 0 || !source.rowID.Available() || source.operation == 0 || source.member < 0 {
			return factBuffer{}, false
		}
		value, valuePresent, available := cells.At(0)
		if !available || valuePresent && !rule.values.Schema().AdmitsCoordinate(source.coordinate, value) || !valuePresent && !rule.values.Schema().Equal(value, rule.values.Schema().Bottom()) {
			return factBuffer{}, false
		}
		if !facts.merge(rule.values.Schema(), factEntry{rowID: source.rowID, value: value, present: valuePresent}) {
			return factBuffer{}, false
		}
	}
	return facts, true
}

func (rule *HotRule) collectFrameFacts(frame engine.Frame[placementdomain.Fact, effectfactor.MountedPublicationBatch], sources sourceView, selection engine.Selection[sourceTag, engine.OrderedCells[valuedomain.Value]]) (factBuffer, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return factBuffer{}, false
	}
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK || count != sources.len() {
		return factBuffer{}, false
	}
	var facts factBuffer
	for index := 0; index < count; index++ {
		tag, cells, selected := engine.SelectionAt(frame, selection, index)
		source, sourceOK := sources.find(tag)
		if !selected || !sourceOK || cells.Count() != 1 {
			return factBuffer{}, false
		}
		if !rule.values.Schema().AdmitsCoordinate(source.coordinate, rule.values.Schema().Bottom()) || source.tag == 0 || !source.rowID.Available() || source.operation == 0 || source.member < 0 {
			return factBuffer{}, false
		}
		value, valuePresent, available := cells.At(0)
		if !available || valuePresent && !rule.values.Schema().AdmitsCoordinate(source.coordinate, value) || !valuePresent && !rule.values.Schema().Equal(value, rule.values.Schema().Bottom()) {
			return factBuffer{}, false
		}
		if !facts.merge(rule.values.Schema(), factEntry{rowID: source.rowID, value: value, present: valuePresent}) {
			return factBuffer{}, false
		}
	}
	return facts, true
}

// validPreparedRoutes is the final local integrity fence before a route can
// widen.  BindHot already proves the same facts from Effect/Pack, but routeSet
// is also exercised by allocation-free planning tests with hand-built plans.
// Those plans must fail closed under the same rules; otherwise a malformed row
// can reach the opaque branch and be converted into an all-root Unknown.
func validPreparedRoutes(prepared *preparedBatch, values *valuedomain.Schema) bool {
	if prepared == nil || values == nil || !values.Valid() {
		return false
	}
	for rowIndex, row := range prepared.rows {
		if !row.id.Available() || row.operation == 0 || !validRequirement(row.requirement) {
			return false
		}
		// A subject is statically absent and proven nil, an empty selected
		// value list, or reachable by an actual tail and unknown. The three
		// readings are exclusive; any two together describe no call.
		if row.subjectNil && row.subjectOpen || row.subjectEmpty && (row.subjectNil || row.subjectOpen) {
			return false
		}
		for priorIndex := 0; priorIndex < rowIndex; priorIndex++ {
			if prepared.rows[priorIndex].id == row.id {
				return false
			}
		}
	}
	for sourceIndex, source := range prepared.sources {
		if source.tag == 0 || !source.rowID.Available() || source.operation == 0 || source.member < 0 ||
			!values.AdmitsCoordinate(source.coordinate, values.Bottom()) {
			return false
		}
		for priorIndex := 0; priorIndex < sourceIndex; priorIndex++ {
			if prepared.sources[priorIndex].tag == source.tag {
				return false
			}
		}
		row, rowOK := preparedRowByID(prepared.rows, source.rowID)
		if !rowOK || row.operation != source.operation {
			return false
		}
		// A proven-nil or empty-list subject selects no mounted semantic
		// source, so a source claiming that row contradicts the row itself.
		if row.subjectNil || row.subjectEmpty {
			return false
		}
	}
	if prepared.prepared {
		if prepared.byTag == nil || len(prepared.byTag) != len(prepared.sources) {
			return false
		}
		for _, source := range prepared.sources {
			indexed, indexedOK := prepared.byTag[source.tag]
			if !indexedOK || indexed != source {
				return false
			}
		}
	}
	return true
}

func validRequirement(requirement placementdomain.Placement) bool {
	return requirement == placementdomain.OwnedHeap || requirement == placementdomain.SharedHeap
}

func preparedRowByID(rows []publicationRow, id identity.ContentID) (publicationRow, bool) {
	for _, row := range rows {
		if row.id == id {
			return row, true
		}
	}
	return publicationRow{}, false
}

func (rule *HotRule) routeSet(schema placementdomain.Schema, prepared *preparedBatch, gate operationGate, facts factBuffer) (routeBuffer, bool) {
	if rule == nil || prepared == nil || !schema.Valid() || rule.values == nil || rule.values.Schema() == nil {
		return routeBuffer{}, false
	}
	valuesSchema := rule.values.Schema()
	if !valuesSchema.Valid() || !valuesSchema.OwnsHeapSchema(schema.Heap()) {
		return routeBuffer{}, false
	}
	if !validPreparedRoutes(prepared, valuesSchema) || !facts.valid(valuesSchema) {
		return routeBuffer{}, false
	}
	var routes routeBuffer
	for _, row := range prepared.rows {
		if !gate.admits(row.operation) {
			// An opaque Call alternative is not a publication receipt.  Effect
			// only issues rows for authenticated Target publication descriptors;
			// the synthesized opaque operation instead carries unknown formal /
			// transfer tails, which are consumed by their own lanes.  Treating
			// that missing descriptor as an all-root Unknown publication would
			// duplicate those lanes and invent affected roots without a canonical
			// payload/formal actual.  A known operation remains routed below even
			// when the Call value also retains its opaque alternative.
			continue
		}
		if row.subjectNil {
			if !prepared.prepared {
				// The proven-nil bit suppresses a route, so it is admissible only
				// when it came from the authenticated MountedInput retained by
				// prepareBatch. A hand-built row cannot self-attest that authority.
				return routeBuffer{}, false
			}
			// Lua under-application proves the subject nil. A nil value holds no
			// allocation root, so this publication escapes nothing.
			continue
		}
		if row.subjectEmpty {
			if !prepared.prepared {
				// The empty-list bit is Pack's resolved projection shape, so it
				// is admissible only when it came from the authenticated
				// MountedInput retained by prepareBatch.
				return routeBuffer{}, false
			}
			// A closed ValuesVar/AllInputs projection that selects no member is
			// an empty value list. It holds no allocation root, so this
			// publication escapes nothing.
			continue
		}
		if row.subjectOpen {
			if !prepared.prepared {
				// The open bit is widening authority only when it came from the
				// authenticated MountedInput retained by prepareBatch. A hand-built
				// row cannot self-attest that authority.
				return routeBuffer{}, false
			}
			all, ok := broadcastAllRoots(schema, row.requirement)
			if !ok || !routes.mergeAllRoot(all) {
				return routeBuffer{}, false
			}
			continue
		}
		fact, factPresent, factOK := facts.get(row.id)
		if !factOK {
			// A closed, operation-admitted row must have an authenticated Value
			// fact.  Omitting it is an incomplete join, not evidence for an
			// all-root Unknown route.
			return routeBuffer{}, false
		}
		if !factPresent {
			// A sparse cell is admissible only when the typed Value owner supplied
			// its exact schema Bottom as the Factor Default.  The exact equality
			// check is repeated at the route boundary so a hand-built buffer cannot
			// turn an arbitrary missing row into a no-route result.
			if !rule.values.Schema().Equal(fact, rule.values.Schema().Bottom()) {
				return routeBuffer{}, false
			}
			continue
		}
		if fact.IsBottom() {
			continue
		}
		roots, unknown, ok := rootsForValue(schema, valuesSchema, fact)
		if !ok {
			return routeBuffer{}, false
		}
		if unknown {
			all, allOK := broadcastAllRoots(schema, row.requirement)
			if !allOK || !routes.mergeAllRoot(all) {
				return routeBuffer{}, false
			}
			continue
		}
		for index := 0; index < roots.len(); index++ {
			key, keyOK := roots.at(index)
			if !keyOK {
				return routeBuffer{}, false
			}
			tag, tagOK := routeTagFor(schema, key)
			if !tagOK {
				return routeBuffer{}, false
			}
			candidate := plannedRoute{key: key, tag: tag, required: row.requirement}
			routes.add(candidate)
		}
	}
	return routes, true
}

func applyRoute(route plannedRoute, current placementdomain.Fact) (placementdomain.Fact, bool) {
	if route.unknown || route.required == placementdomain.Unknown {
		return placementdomain.UnknownFact(), true
	}
	switch route.required {
	case placementdomain.OwnedHeap:
		return placementdomain.DisplaceFactChecked(current, placementdomain.Retain)
	case placementdomain.SharedHeap:
		return placementdomain.DisplaceFactChecked(current, placementdomain.Send)
	default:
		return placementdomain.BottomFact(), false
	}
}
