package suspension

import (
	"crypto/sha256"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// source is one owner-fenced Program semantic subject projected into Value's
// existing mounted coordinate directory. The tag is local transport
// evidence for the selected-read route; module and id remain the canonical
// cold provenance and are rechecked before admission.
type source struct {
	module     identity.ContentID
	id         identity.ContentID
	coordinate valuedomain.Coordinate
	tag        routeTag
}

type catalogRow struct {
	operand operand
}

// valueAggregateEntry is the immutable Value-family geometry for one Values
// aggregate. Seal-time indexing makes the ValuesFamily lookup
// constant time while retaining the exact aggregate/member/tail order.
type valueAggregateEntry struct {
	ids       []identity.ContentID
	open      bool
	valid     bool
	duplicate bool
}

type aggregateSources struct {
	sources []source
	ok      bool
}

// valueAggregateIndex is built once for one mounted Program. sourceSets is a
// second, lazy index because coordinate projection is mount-specific. Each
// successful source slice is immutable after it is cached and is shared by
// every liveness row that names the same Values aggregate.
type valueAggregateIndex struct {
	valid      bool
	entries    map[identity.ContentID]valueAggregateEntry
	sourceSets map[identity.ContentID]aggregateSources
}

// Catalog is the Link-owned denominator for suspension Placement writes. It
// contains one row for every mounted Program subject-liveness row. Exact Heap
// allocation roots and Value mounted coordinates are retained when they can be
// authenticated. Cell and Values subjects retain their neutral source identity
// until a selected Value read projects their atoms onto Heap roots; no made-up
// Placement root is stored in the catalog.
//
// IDs are mount-scoped because one immutable Program may be mounted more than
// once in a Link. The Program row remains the source identity inside the
// sealed mount; occurrenceID only prevents those rows from colliding in the
// Link admission inventory.
type Catalog struct {
	schema placementdomain.Schema
	values *valuedomain.Schema
	ids    []identity.ContentID
	rows   map[identity.ContentID]catalogRow
}

// SealCatalog joins each mounted Program liveness row to Heap's existing
// allocation inverse through the owner-fenced Value directory. No Flow
// object, authored term, or Placement policy is retained in the catalog.
//
// Value is mandatory: Cell, Value, and Values subjects cannot be projected to
// allocation roots by a direct-root shortcut without either
// under-approximating aliases or inventing roots. Missing joins refuse catalog
// sealing: the hot route never receives a row it cannot authenticate.
func SealCatalog(schema placementdomain.Schema, values *valuedomain.Schema) (*Catalog, bool) {
	return sealCatalog(schema, values, false)
}

// SealEvidenceCatalog derives the same sealed Program/Value denominator for
// the independent evidence rule, but namespaces its occurrence identities.
// A Link bootstrap admits one occurrence ID to one Rule capability; the class
// and evidence producers therefore cannot reuse the class catalog's IDs even
// though they intentionally share the same subject-liveness rows and routes.
func SealEvidenceCatalog(schema placementdomain.Schema, values *valuedomain.Schema) (*Catalog, bool) {
	return sealCatalog(schema, values, true)
}

// sealCatalog performs the one cold join used by both suspension producers.
// evidence changes only the Link occurrence namespace after the same operand
// has been authenticated; it must not trigger a second Values-family scan.
func sealCatalog(schema placementdomain.Schema, values *valuedomain.Schema, evidence bool) (*Catalog, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return nil, false
	}
	mounts := schema.Heap().MountedArtifacts()
	catalog := &Catalog{
		schema: schema, values: values,
		ids: make([]identity.ContentID, 0), rows: make(map[identity.ContentID]catalogRow),
	}
	for _, mount := range mounts {
		if !mount.Available() || mount.Snapshot == nil {
			return nil, false
		}
		module := mount.ModuleKey
		program := mount.Snapshot.Program()
		if !program.Available() {
			return nil, false
		}
		state, stateOK := program.ColdState()
		view, viewOK := lifecycle.NewView(state)
		if !stateOK || !viewOK {
			return nil, false
		}
		spanCount, spanCountOK := view.SubjectLivenessSpanCount()
		boundaryCount, boundaryCountOK := view.SubjectYieldBoundaryCount()
		if !spanCountOK || spanCount < 0 || !boundaryCountOK || boundaryCount < 0 {
			return nil, false
		}
		// The spans are ranges over the ordered boundary sequence, so the
		// anchor lookup is by ordinal. The sequence is dense and emitted in
		// ordinal order, so one pass indexes it.
		boundaries := make([]lifecycle.SubjectYieldBoundary, boundaryCount)
		for index := 0; index < boundaryCount; index++ {
			boundary, boundaryOK := view.SubjectYieldBoundaryAt(index)
			if !boundaryOK || !boundary.Available() || int(boundary.Ordinal()) != index {
				return nil, false
			}
			boundaries[index] = boundary
		}
		issuer, issuerOK := schema.Heap().OccurrenceMountForModule(module)
		if !issuerOK {
			return nil, false
		}
		bodyTargets, targetsOK := bodyTargetBatch(program)
		if !targetsOK {
			return nil, false
		}
		var valueIndex valueAggregateIndex
		valueIndexBuilt := false
		for index := 0; index < spanCount; index++ {
			span, spanOK := view.SubjectLivenessSpanAt(index)
			if !spanOK || !span.Available() || int(span.Lo()) >= len(boundaries) {
				return nil, false
			}
			// The obligation is issued once, at the boundary where the subject
			// enters this answer. The answer is constant across the run by
			// construction, so the later boundaries in the span carry no
			// operand and are served by the range read.
			anchor := boundaries[span.Lo()]
			row := spanSubject{kind: span.SubjectKind(), subject: span.SubjectID(), state: span.State(), call: anchor.CallID()}
			if row.kind == lifecycle.SubjectLivenessValues && !valueIndexBuilt {
				valueIndex, _ = buildValueAggregateIndex(program)
				valueIndexBuilt = true
			}
			candidate, candidateOK := catalogOperandIndexed(schema, values, module, issuer, bodyTargets, program, view, row, valueIndex)
			if !candidateOK {
				return nil, false
			}
			id, idOK := occurrenceID(module, span.ID())
			if !idOK {
				return nil, false
			}
			candidate.id = id
			canonical, candidateOK := operandForCatalog(schema, values, candidate)
			if !candidateOK {
				return nil, false
			}
			placement, placementOK := PlacementForState(canonical.state)
			if !placementOK || !validPlacement(placement) {
				return nil, false
			}
			if evidence {
				evidenceID, evidenceOK := evidenceOccurrenceID(id)
				if !evidenceOK {
					return nil, false
				}
				canonical.id = evidenceID
				id = evidenceID
			}
			if _, duplicate := catalog.rows[id]; duplicate {
				return nil, false
			}
			catalog.ids = append(catalog.ids, id)
			catalog.rows[id] = catalogRow{operand: canonical}
		}
	}
	return catalog, catalog.FencedTo(schema, values)
}

// spanSubject is the neutral projection of one liveness span together with
// the boundary it is anchored at. It carries exactly what the per-pair row
// used to hand these helpers: which subject, what answer, and the Call the
// obligation is issued at.
type spanSubject struct {
	kind    lifecycle.SubjectLivenessKind
	subject identity.ContentID
	state   lifecycle.SubjectLivenessState
	call    identity.ContentID
}

func (row spanSubject) Available() bool {
	return row.kind.Valid() && row.subject.Available() && row.state.Valid() && row.call.Available()
}

func evidenceOccurrenceID(classID identity.ContentID) (identity.ContentID, bool) {
	if !classID.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("program/placement-suspension-evidence-occurrence-v1"))
	_, _ = hash.Write(classID[:])
	return identity.ContentID(hash.Sum(nil)), true
}

func catalogOperandIndexed(schema placementdomain.Schema, values *valuedomain.Schema, module identity.ContentID, issuer heapdomain.OccurrenceMount, bodyTargets map[identity.ContentID]identity.ContentID, program programschema.Program, view lifecycle.View, row spanSubject, valueIndex valueAggregateIndex) (operand, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) || !module.Available() || !program.Available() || !view.Available() || !row.Available() {
		return operand{}, false
	}
	result := operand{state: row.state}
	if key, keyOK := keyForRow(schema, issuer, bodyTargets, row); keyOK {
		result.key = key
		return result, true
	}
	// Root rows normally join through CallTarget. A missing body target cannot
	// be reconstructed from a neutral row, so the catalog must refuse it rather
	// than widen a fabricated operand across the allocation denominator.
	if row.kind == lifecycle.SubjectLivenessValues {
		sources, sourcesOK := valueIndex.sourcesFor(values, module, row.subject)
		if !sourcesOK || len(sources) == 0 {
			return operand{}, false
		}
		// This source set is an immutable per-mount aggregate projection.
		// Every liveness row naming this aggregate borrows it instead of
		// rebuilding the member/tail source slice.
		result.sources = sources
		return result, true
	}
	sourceIDs, sourceIDsOK := subjectValueIDsIndexed(program, view, row, valueIndex)
	if !sourceIDsOK || len(sourceIDs) == 0 {
		return operand{}, false
	}
	sources, sourceOK := sourcesForIDs(values, module, sourceIDs)
	if !sourceOK {
		// A selected read with a missing member cannot be made exhaustive by
		// widening. Refuse the catalog while the exact owner evidence is still
		// available for diagnosis.
		return operand{}, false
	}
	result.sources = sources
	return result, true
}

// buildValueAggregateIndex reads one mounted Program Values family once. A
// failed cold reads are represented by ok=false. A mount containing a Values
// liveness row then refuses sealing; there is no unresolved denominator row.
func buildValueAggregateIndex(program programschema.Program) (valueAggregateIndex, bool) {
	// Initialize both maps before returning the value. Seal passes this small
	// index by value to each row join; a shared map backing keeps source sets
	// cached even across those copies.
	index := valueAggregateIndex{
		entries:    make(map[identity.ContentID]valueAggregateEntry),
		sourceSets: make(map[identity.ContentID]aggregateSources),
	}
	if !program.Available() {
		return index, false
	}
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	if !catalogOK {
		return index, false
	}
	family := programschema.ValuesFamily()
	count, countOK := family.Count(&program.Frozen, catalog)
	if !countOK || count < 0 {
		return index, false
	}
	index.valid = true
	for ordinal := 0; ordinal < count; ordinal++ {
		aggregate, aggregateOK := family.At(&program.Frozen, catalog, ordinal)
		if !aggregateOK || !aggregate.Available() {
			// Match subjectValueIDs: an unavailable/non-readable row cannot
			// match an available SubjectLivenessValues identity.
			continue
		}
		id := aggregate.ID()
		if _, duplicate := index.entries[id]; duplicate {
			// Only the matching ID is ambiguous. Unrelated duplicate rows
			// must not make a catalog fail if no liveness row names them.
			index.entries[id] = valueAggregateEntry{duplicate: true}
			continue
		}
		ids, open, idsOK := valueAggregateIDs(program, catalog, aggregate)
		index.entries[id] = valueAggregateEntry{ids: ids, open: open, valid: idsOK}
	}
	return index, true
}

// valueAggregateIDs retains the canonical aggregate/member/tail geometry. It
// is called once per Values aggregate while the index is built, rather than
// once per liveness row.
func valueAggregateIDs(program programschema.Program, catalog identity.ContentID, aggregate programschema.Values) ([]identity.ContentID, bool, bool) {
	if !program.Available() || !aggregate.Available() {
		return nil, false, false
	}
	ids := make([]identity.ContentID, 0, aggregate.MemberCount()+2)
	ids = append(ids, aggregate.ID())
	offset, memberCount, spanOK := aggregate.MemberSpan()
	if !spanOK {
		return nil, false, false
	}
	memberCatalog := programschema.ValuesMemberFamily()
	for index := 0; index < int(memberCount); index++ {
		member, memberOK := memberCatalog.At(&program.Frozen, catalog, int(offset)+index)
		if !memberOK || !member.Available() {
			return nil, false, false
		}
		ids = append(ids, member.ID())
	}
	_, open := aggregate.Tail()
	unique, uniqueOK := uniqueSubjectValueIDs(ids)
	return unique, open, uniqueOK
}

func subjectValueIDsIndexed(program programschema.Program, view lifecycle.View, row spanSubject, index valueAggregateIndex) ([]identity.ContentID, bool) {
	if !program.Available() || !view.Available() || !row.Available() {
		return nil, false
	}
	switch row.kind {
	case lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessValue:
		if row.kind == lifecycle.SubjectLivenessCell {
			if _, lifetimeOK := view.StorageCellLifetimeForID(row.subject); !lifetimeOK {
				return nil, false
			}
		}
		return []identity.ContentID{row.subject}, true
	case lifecycle.SubjectLivenessValues:
		if !index.valid {
			return nil, false
		}
		entry, entryOK := index.entries[row.subject]
		if !entryOK || entry.duplicate || !entry.valid {
			return nil, false
		}
		return entry.ids, true
	default:
		return nil, false
	}
}

func sourcesForIDs(values *valuedomain.Schema, module identity.ContentID, ids []identity.ContentID) ([]source, bool) {
	if values == nil || !values.Valid() || !module.Available() || len(ids) == 0 {
		return nil, false
	}
	sources := make([]source, len(ids))
	indexed := make([]struct {
		item  source
		index uint32
	}, len(ids))
	for index, id := range ids {
		coordinate, coordinateOK := values.CoordinateForMountedSemantic(module, id)
		if !coordinateOK || !coordinate.Valid() {
			return nil, false
		}
		item := source{module: module, id: id, coordinate: coordinate}
		coordinateIndex, coordinateOK := values.CoordinateIndex(coordinate)
		if !coordinateOK {
			return nil, false
		}
		indexed[index] = struct {
			item  source
			index uint32
		}{item: item, index: coordinateIndex}
	}
	// The engine canonicalizes selected routes by their physical Value Unit
	// before materializing SelectionAt. Program Values families retain their
	// semantic aggregate/member order, while Boundary explicitly leaves its
	// mounted-semantic directory order unspecified, so those orders need not
	// coincide. Sort by this exact Value schema's coordinate and assign tags
	// afterward; otherwise the hot positional tag check would conservatively
	// widen a valid Values row whenever the two orders differ.
	sort.Slice(indexed, func(left, right int) bool { return indexed[left].index < indexed[right].index })
	for index, item := range indexed {
		item.item.tag = routeTag(uint64(index) + 1)
		sources[index] = item.item
	}
	return sources, true
}

func (index *valueAggregateIndex) sourcesFor(values *valuedomain.Schema, module identity.ContentID, id identity.ContentID) ([]source, bool) {
	if index == nil || !index.valid || values == nil || !values.Valid() || !module.Available() || !id.Available() {
		return nil, false
	}
	if index.sourceSets == nil {
		index.sourceSets = make(map[identity.ContentID]aggregateSources)
	}
	if cached, cachedOK := index.sourceSets[id]; cachedOK {
		return cached.sources, cached.ok
	}
	entry, entryOK := index.entries[id]
	if !entryOK || entry.duplicate || !entry.valid {
		index.sourceSets[id] = aggregateSources{ok: false}
		return nil, false
	}
	// An open tail names additional runtime Values that are not present in
	// this sealed directory. It is an incomplete read, not an opaque Value
	// fact; refuse it before the hot route can plan any allocation roots.
	if entry.open {
		index.sourceSets[id] = aggregateSources{ok: false}
		return nil, false
	}
	sources, sourcesOK := sourcesForIDs(values, module, entry.ids)
	index.sourceSets[id] = aggregateSources{sources: sources, ok: sourcesOK}
	return sources, sourcesOK
}

func uniqueSubjectValueIDs(ids []identity.ContentID) ([]identity.ContentID, bool) {
	if len(ids) == 0 {
		return nil, false
	}
	seen := make(map[identity.ContentID]struct{}, len(ids))
	for _, id := range ids {
		if !id.Available() {
			return nil, false
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, false
		}
		seen[id] = struct{}{}
	}
	return ids, true
}

func operandForCatalog(schema placementdomain.Schema, values *valuedomain.Schema, candidate operand) (operand, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) || !candidate.id.Available() || !candidate.state.Valid() {
		return operand{}, false
	}
	if candidate.key.Kind() == heapdomain.RootAllocation {
		canonical, ok := operandForSchema(schema, candidate)
		if !ok {
			return operand{}, false
		}
		canonical.sources = nil
		return canonical, true
	}
	if candidate.key != (heapdomain.Key{}) {
		return operand{}, false
	}
	canonical := candidate
	if len(canonical.sources) == 0 {
		return operand{}, false
	}
	// Sources are private rows emitted only by sourcesForIDs during this seal.
	// That constructor authenticates the mounted semantic coordinate, sorts by
	// the Value factor coordinate, and assigns the canonical one-based tags.
	// Reopening Value's directory here would repeat the same owner check for
	// every liveness row without adding a second authority boundary.
	return canonical, true
}

// bodyTargetBatch is one construction-time body→allocation join for a
// mounted Program. It avoids repeatedly scanning CallTarget for every Root
// liveness row while retaining no consumer-side inverse after Catalog seals.
func bodyTargetBatch(program programschema.Program) (map[identity.ContentID]identity.ContentID, bool) {
	if !program.Available() {
		return nil, false
	}
	state, stateOK := program.ColdState()
	view, viewOK := calltarget.NewView(state)
	count, countOK := view.Count()
	if !stateOK || !viewOK || !countOK || count < 0 {
		return nil, false
	}
	result := make(map[identity.ContentID]identity.ContentID, count)
	for index := 0; index < count; index++ {
		target, targetOK := view.At(index)
		if !targetOK || !target.Available() {
			return nil, false
		}
		if _, duplicate := result[target.BodyID()]; duplicate {
			return nil, false
		}
		result[target.BodyID()] = target.AllocationID()
	}
	return result, true
}

func keyForRow(schema placementdomain.Schema, issuer heapdomain.OccurrenceMount, bodyTargets map[identity.ContentID]identity.ContentID, row spanSubject) (heapdomain.Key, bool) {
	if !schema.Valid() || !row.Available() || bodyTargets == nil || !issuer.Module().Available() || !issuer.ProgramID().Available() {
		return heapdomain.Key{}, false
	}
	var allocationID identity.ContentID
	switch row.kind {
	case lifecycle.SubjectLivenessValue:
		allocationID = row.subject
	case lifecycle.SubjectLivenessRoot:
		var targetOK bool
		allocationID, targetOK = bodyTargets[row.subject]
		if !targetOK {
			return heapdomain.Key{}, false
		}
	default:
		return heapdomain.Key{}, false
	}
	if !allocationID.Available() {
		return heapdomain.Key{}, false
	}
	key, keyOK := issuer.AllocationRootForOccurrence(allocationID)
	if !keyOK || key.Kind() != heapdomain.RootAllocation || !schema.Heap().OwnsKey(key) {
		return heapdomain.Key{}, false
	}
	module, programID, canonicalAllocation, _, _, originOK := schema.Heap().AllocationOriginForKey(key)
	if !originOK || module != issuer.Module() || programID != issuer.ProgramID() || canonicalAllocation != allocationID {
		return heapdomain.Key{}, false
	}
	return key, true
}

func (catalog *Catalog) FencedTo(schema placementdomain.Schema, values *valuedomain.Schema) bool {
	if catalog == nil || catalog.schema != schema || !schema.Valid() ||
		len(catalog.rows) != len(catalog.ids) {
		return false
	}
	return catalog.values == values && values != nil && values.Valid() && values.OwnsHeapSchema(schema.Heap())
}

func (catalog *Catalog) Count() int {
	if catalog == nil || !catalog.FencedTo(catalog.schema, catalog.values) {
		return 0
	}
	return len(catalog.ids)
}

func (catalog *Catalog) IDAt(index int) (identity.ContentID, bool) {
	if catalog == nil || !catalog.FencedTo(catalog.schema, catalog.values) || index < 0 || index >= len(catalog.ids) {
		return identity.ContentID{}, false
	}
	id := catalog.ids[index]
	_, ok := catalog.operandForID(id)
	return id, ok
}

func (catalog *Catalog) operandForID(id identity.ContentID) (operand, bool) {
	if catalog == nil || !catalog.FencedTo(catalog.schema, catalog.values) || !id.Available() {
		return operand{}, false
	}
	row, ok := catalog.rows[id]
	if !ok || row.operand.id != id {
		return operand{}, false
	}
	// SealCatalog canonicalized this private immutable row against the exact
	// schema/value owner pair retained above. Reopening mounted artifacts or
	// rewriting the source slice on every hot lookup is both redundant and
	// unsafe under concurrent reads.
	return row.operand, true
}

func (catalog *Catalog) KeyForID(id identity.ContentID) (heapdomain.Key, bool) {
	operand, ok := catalog.operandForID(id)
	if !ok || operand.key.Kind() != heapdomain.RootAllocation {
		return heapdomain.Key{}, false
	}
	return operand.key, true
}

// ValueSchema returns the exact Value authority used to seal this catalog.
// It is never an invitation to substitute a foreign Value schema.
func (catalog *Catalog) ValueSchema() *valuedomain.Schema {
	if catalog == nil || !catalog.FencedTo(catalog.schema, catalog.values) {
		return nil
	}
	return catalog.values
}

// SourcesForID exposes the already-authenticated mounted Value coordinates
// carried by one Value-aware row.  The returned slice is a defensive copy so
// callers cannot mutate the catalog's sealed route tags or coordinate fence.
func (catalog *Catalog) SourcesForID(id identity.ContentID) ([]source, bool) {
	operand, ok := catalog.operandForID(id)
	if !ok || catalog.values == nil {
		return nil, false
	}
	return append([]source(nil), operand.sources...), true
}
