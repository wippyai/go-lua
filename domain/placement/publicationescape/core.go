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
	operation   vocabulary.Operation
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
// Every member is observed independently; a missing member remains absent,
// while present members join under the Value schema's own algebra.
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
			prior.present = false
			return facts.setAt(index, prior)
		}
		if !prior.present {
			return true
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
}

func (routes *routeBuffer) at(index int) (plannedRoute, bool) {
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

func (routes routeBuffer) len() int { return routes.count }

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
		prior, _ := routes.at(index)
		if prior.key == candidate.key {
			routes.set(index, mergeRoute(prior, candidate))
			return
		}
	}
	insert := routes.count
	for index := 0; index < routes.count; index++ {
		prior, _ := routes.at(index)
		if candidate.tag < prior.tag {
			insert = index
			break
		}
	}
	if routes.count < inlineRouteCapacity {
		for index := routes.count; index > insert; index-- {
			prior, _ := routes.at(index - 1)
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
	index, ok := schema.Heap().KeyIndex(key)
	if !ok || index < 0 || uint64(index) >= uint64(^uint32(0)) {
		return 0, false
	}
	canonical, canonicalOK := schema.KeyAt(index)
	if !canonicalOK || canonical != key {
		return 0, false
	}
	return routeTag(uint64(index) + 1), true
}

// widenAllRoots is invoked only after one of the three authenticated widening
// proofs has been established: an open MountedInput, an opaque Call gate, or
// an actual Value Top/opaque-reference fact.  It is never an error recovery
// path for a missing or malformed predecessor.
func widenAllRoots(schema placementdomain.Schema) (routeBuffer, bool) {
	var routes routeBuffer
	if !schema.Valid() {
		return routes, false
	}
	for dense := 0; dense < schema.DenseKeyCount(); dense++ {
		key, ok := schema.KeyAt(dense)
		if !ok {
			return routeBuffer{}, false
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		tag, ok := routeTagFor(schema, key)
		if !ok {
			return routeBuffer{}, false
		}
		routes.add(plannedRoute{key: key, tag: tag, required: placementdomain.Unknown, unknown: true})
	}
	return routes, true
}

// rootsForValue is intentionally local. placement/store.Plan has the same
// atom walk but its package owns Program storage/lifetime semantics; this rule
// only needs the neutral Value-to-Heap projection.
func rootsForValue(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value) (roots keyBuffer, unknown, ok bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return roots, false, false
	}
	// Check ownership before the lattice extremes.  IsBottom/IsTop are
	// intentionally owner-local, but a foreign schema can still carry a valid
	// extreme and must not be accepted as a local publication fact.
	if !values.Equal(fact, fact) {
		return roots, false, false
	}
	if fact.IsBottom() {
		return roots, false, true
	}
	if fact.IsTop() {
		return roots, true, true
	}
	valid := true
atoms:
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			valid = false
			break
		}
		classification, classificationOK := placementdomain.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			valid = false
			break atoms
		}
		switch classification.Class {
		case placementdomain.AtomClassAllocation:
			key := classification.Key
			if !schema.Heap().OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
				valid = false
				break atoms
			}
			index, indexOK := schema.Heap().KeyIndex(key)
			canonical, canonicalOK := schema.KeyAt(index)
			if !indexOK || !canonicalOK || canonical != key {
				valid = false
				break atoms
			}
			roots.add(key)
		case placementdomain.AtomClassOpaque:
			unknown = true
		}
	}
	return roots, unknown, valid
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
		// selected Value predecessor in this first rule.
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
		// publication rows are widened only at the authenticated opaque Call
		// boundary in routeSet; no source or coordinate is fabricated here.
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
	if !present {
		bottom := rule.calls.Algebra().Bottom()
		return bottom, false, rule.calls.Algebra().Admits(key, bottom)
	}
	return value, true, rule.calls.Algebra().Admits(key, value)
}

func (rule *HotRule) callValueFrame(frame engine.Frame[placementdomain.Placement, effectfactor.MountedPublicationBatch], batch effectfactor.MountedPublicationBatch) (calldomain.Value, bool, bool) {
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
	if !present {
		bottom := rule.calls.Algebra().Bottom()
		return bottom, false, rule.calls.Algebra().Admits(key, bottom)
	}
	return value, true, rule.calls.Algebra().Admits(key, value)
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

func (rule *HotRule) collectFrameFacts(frame engine.Frame[placementdomain.Placement, effectfactor.MountedPublicationBatch], sources sourceView, selection engine.Selection[sourceTag, engine.OrderedCells[valuedomain.Value]]) (factBuffer, bool) {
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
	if !validPreparedRoutes(prepared, rule.values.Schema()) || !facts.valid(rule.values.Schema()) {
		return routeBuffer{}, false
	}
	var routes routeBuffer
	for _, row := range prepared.rows {
		if !gate.admits(row.operation) {
			if !gate.opaque {
				continue
			}
			all, ok := widenAllRoots(schema)
			if !ok {
				return routeBuffer{}, false
			}
			for index := 0; index < all.len(); index++ {
				candidate, candidateOK := all.at(index)
				if !candidateOK {
					return routeBuffer{}, false
				}
				routes.add(candidate)
			}
			continue
		}
		if row.subjectOpen {
			if !prepared.prepared {
				// The open bit is widening authority only when it came from the
				// authenticated MountedInput retained by prepareBatch. A hand-built
				// row cannot self-attest that authority.
				return routeBuffer{}, false
			}
			all, ok := widenAllRoots(schema)
			if !ok {
				return routeBuffer{}, false
			}
			for index := 0; index < all.len(); index++ {
				candidate, candidateOK := all.at(index)
				if !candidateOK {
					return routeBuffer{}, false
				}
				routes.add(candidate)
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
		if !factPresent || fact.IsBottom() {
			continue
		}
		roots, unknown, ok := rootsForValue(schema, rule.values.Schema(), fact)
		if !ok {
			return routeBuffer{}, false
		}
		if unknown {
			all, allOK := widenAllRoots(schema)
			if !allOK {
				return routeBuffer{}, false
			}
			for index := 0; index < all.len(); index++ {
				candidate, candidateOK := all.at(index)
				if !candidateOK {
					return routeBuffer{}, false
				}
				routes.add(candidate)
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

func applyRoute(route plannedRoute, current placementdomain.Placement) placementdomain.Placement {
	if route.unknown || route.required == placementdomain.Unknown {
		return placementdomain.Unknown
	}
	switch route.required {
	case placementdomain.OwnedHeap:
		return placementdomain.Displace(current, placementdomain.Retain)
	case placementdomain.SharedHeap:
		return placementdomain.Displace(current, placementdomain.Send)
	default:
		return placementdomain.Unknown
	}
}
