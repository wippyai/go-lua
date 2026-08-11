package residence

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

const schemaFormat uint64 = 0x7265736964656e63 // "residenc"

// Schema is the sealed Residence family for one exact Link. K is exactly the
// structural boundary range. AnalysisRoot is solver State and is admitted by
// a factored boundary-to-root relation rather than a root×boundary key table.
type Schema struct{ owner *schema }

type schema struct {
	source *link.Link
	heap   heap.Schema
	linkID keyspace.ContentID
	id     keyspace.ContentID

	allocations     []heap.Key
	allocationIndex map[heap.Key]uint32

	boundaries    []boundaryRow
	boundaryIndex map[boundaryIdentity]uint32
	rootClasses   [][]linkmodule.AnalysisRoot
	classBuckets  map[uint64][]uint32
	shardClasses  map[linkproject.Shard]uint32

	potential uint64
	bottom    Value
	top       Value
}

type boundaryIdentity struct {
	kind BoundaryKind
	id   keyspace.ContentID
}

type boundaryRow struct {
	kind        BoundaryKind
	id          keyspace.ContentID
	class       uint32
	program     programBoundary
	application linkproject.Application
	operation   target.Operation
	port        uint32
	entry       linkmodule.ModuleCacheEntry
	coordinate  linkmodule.ModuleCoordinate
	global      linkhost.GlobalBinding
}

func Seal(source *link.Link, heapSchema heap.Schema) (Schema, bool) {
	// ContentID is the cold/replay identity; it is not a live ownership
	// capability.  Residence may only ingest allocation keys from a Heap
	// schema sealed against this exact Link authority.
	if source == nil || !source.ContentID().Available() || !heapSchema.Valid() || heapSchema.Link() != source || heapSchema.LinkContentID() != source.ContentID() {
		return Schema{}, false
	}
	owner := &schema{
		source:          source,
		heap:            heapSchema,
		linkID:          source.ContentID(),
		allocationIndex: make(map[heap.Key]uint32),
		boundaryIndex:   make(map[boundaryIdentity]uint32),
		classBuckets:    make(map[uint64][]uint32),
		shardClasses:    make(map[linkproject.Shard]uint32),
	}
	if !owner.addAllocations() || !owner.captureBoundaries() || !owner.storeBoundaries() || !owner.returnBoundaries() ||
		!owner.addApplicationBoundaries() ||
		!owner.addModuleEntryBoundaries() || !owner.addModuleCoordinateBoundaries() || !owner.addGlobalBoundaries() || !owner.finish() {
		return Schema{}, false
	}
	return Schema{owner: owner}, true
}

func (owner *schema) addAllocations() bool {
	if owner == nil || !owner.heap.Valid() {
		return false
	}
	for index := 0; index < owner.heap.KeyCount(); index++ {
		root, ok := owner.heap.KeyAt(index)
		if !ok {
			return false
		}
		if root.Kind() != heap.RootAllocation {
			continue
		}
		if !owner.heap.OwnsKey(root) || owner.allocationIndex[root] != 0 {
			return false
		}
		owner.allocations = append(owner.allocations, root)
		owner.allocationIndex[root] = uint32(len(owner.allocations))
	}
	return true
}

func (owner *schema) rootsForShard(shard linkproject.Shard) ([]linkmodule.AnalysisRoot, bool) {
	count := owner.source.Module().Roots().ForShardCount(shard)
	roots := make([]linkmodule.AnalysisRoot, 0, count)
	for index := 0; index < count; index++ {
		root, ok := owner.source.Module().Roots().ForShardAt(shard, index)
		if !ok {
			return nil, false
		}
		roots = append(roots, root)
	}
	return roots, true
}

func (owner *schema) classForShard(shard linkproject.Shard) (uint32, bool) {
	if stored := owner.shardClasses[shard]; stored != 0 {
		return stored - 1, true
	}
	roots, ok := owner.rootsForShard(shard)
	if !ok {
		return 0, false
	}
	class, ok := owner.internRootClass(roots)
	if !ok || class == ^uint32(0) {
		return 0, false
	}
	owner.shardClasses[shard] = class + 1
	return class, true
}

func (owner *schema) classForApplication(application linkproject.Application) (uint32, bool) {
	shard, term, ok := owner.source.Project().Applications().Call(application)
	if !ok {
		return 0, false
	}
	p, ok := owner.source.Project().Mounts().Program(shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(term) {
		return 0, false
	}
	return owner.classForShard(shard)
}

func (owner *schema) addApplicationBoundaries() bool {
	contract, ok := owner.source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	for appIndex := 0; appIndex < owner.source.Project().Applications().Count(); appIndex++ {
		application, ok := owner.source.Project().Applications().At(appIndex)
		if !ok {
			return false
		}
		class, ok := owner.classForApplication(application)
		if !ok {
			continue
		}
		project := owner.source.Project()
		if project == nil {
			return false
		}
		appID, ok := project.ApplicationID(application)
		if !ok {
			return false
		}
		for opIndex := 0; opIndex < contract.OperationCount(); opIndex++ {
			operation, ok := contract.OperationAt(opIndex)
			if !ok {
				return false
			}
			if !owner.source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
				continue
			}
			appendPort := func(kind BoundaryKind, port uint32) bool {
				return owner.addBoundaryClass(boundaryRow{kind: kind, id: applicationBoundaryID(owner.linkID, appID, operation, kind, port), application: application, operation: operation, port: port}, class)
			}
			for index := 0; index < contract.CallbackCount(operation); index++ {
				callback, ok := contract.CallbackAt(operation, index)
				if !ok || !appendPort(BoundaryCallback, uint32(callback)) {
					return false
				}
			}
			for index := 0; index < contract.SuspensionCount(operation); index++ {
				if !appendPort(BoundarySuspension, uint32(index)) {
					return false
				}
			}
			for index := 0; index < contract.TransferCount(operation); index++ {
				transfer, ok := contract.TransferIDAt(operation, index)
				if !ok || !appendPort(BoundaryTransfer, uint32(transfer)) {
					return false
				}
			}
			for index := 0; index < contract.ResumeCount(operation); index++ {
				resume, ok := contract.ResumeIDAt(operation, index)
				if !ok || !appendPort(BoundaryResume, uint32(resume)) {
					return false
				}
			}
		}
	}
	return true
}

func applicationBoundaryID(linkID, applicationID keyspace.ContentID, operation target.Operation, kind BoundaryKind, port uint32) keyspace.ContentID {
	var image [32 + 32 + 4*8]byte
	copy(image[:32], linkID[:])
	copy(image[32:64], applicationID[:])
	binary.BigEndian.PutUint64(image[64:], 0x7265732d6170706f)
	binary.BigEndian.PutUint64(image[72:], uint64(operation))
	binary.BigEndian.PutUint64(image[80:], uint64(kind))
	binary.BigEndian.PutUint64(image[88:], uint64(port))
	return keyspace.ContentID(sha256.Sum256(image[:]))
}

func (owner *schema) addModuleEntryBoundaries() bool {
	for index := 0; index < owner.source.Module().Cache().EntryCount(); index++ {
		entry, ok := owner.source.Module().Cache().EntryAt(index)
		id, idOK := owner.source.Module().Cache().EntryID(entry)
		_, from, to, mappingOK := owner.source.Module().Cache().EntryMapping(entry)
		if !ok || !idOK || !mappingOK || !owner.addBoundary(boundaryRow{kind: BoundaryModuleEntry, id: id, entry: entry}, []linkmodule.AnalysisRoot{from, to}) {
			return false
		}
	}
	return true
}

type coordinateScope struct {
	actor          linkmodule.Actor
	shard          linkproject.Shard
	representative linkmodule.ModuleCacheInstance
}

func (owner *schema) addModuleCoordinateBoundaries() bool {
	scopes := make(map[coordinateScope][]linkmodule.AnalysisRoot)
	for index := 0; index < owner.source.Module().Roots().Count(); index++ {
		root, ok := owner.source.Module().Roots().At(index)
		coordinate, coordinateOK := owner.source.Module().Coordinates().ForRoot(root)
		actor, shard, representative, mappingOK := owner.source.Module().Coordinates().Mapping(coordinate)
		if !ok || !coordinateOK || !mappingOK {
			return false
		}
		tuple := coordinateScope{actor: actor, shard: shard, representative: representative}
		scopes[tuple] = append(scopes[tuple], root)
	}
	for index := 0; index < owner.source.Module().Cache().EntryCount(); index++ {
		entry, ok := owner.source.Module().Cache().EntryAt(index)
		_, from, to, mappingOK := owner.source.Module().Cache().EntryMapping(entry)
		_, fromActor, fromInstance, fromOK := owner.source.Module().Roots().Mapping(from)
		toShard, toActor, _, toOK := owner.source.Module().Roots().Mapping(to)
		representative, representativeOK := owner.source.Module().Cache().Representative(fromInstance)
		if !ok || !mappingOK || !fromOK || !toOK || !representativeOK || fromActor != toActor {
			return false
		}
		tuple := coordinateScope{actor: fromActor, shard: toShard, representative: representative}
		scopes[tuple] = append(scopes[tuple], to)
	}
	for index := 0; index < owner.source.Module().Coordinates().Count(); index++ {
		coordinate, ok := owner.source.Module().Coordinates().At(index)
		id, idOK := owner.source.Module().Coordinates().ID(coordinate)
		actor, shard, representative, mappingOK := owner.source.Module().Coordinates().Mapping(coordinate)
		roots := scopes[coordinateScope{actor: actor, shard: shard, representative: representative}]
		if !ok || !idOK || !mappingOK || len(roots) == 0 || !owner.addBoundary(boundaryRow{kind: BoundaryModuleCoordinate, id: id, coordinate: coordinate}, roots) {
			return false
		}
	}
	return true
}

func (owner *schema) addGlobalBoundaries() bool {
	for index := 0; index < owner.source.Host().Globals().Count(); index++ {
		global, ok := owner.source.Host().Globals().At(index)
		id, idOK := residenceGlobalBindingID(owner.source, global)
		root, _, _, _, _, _, mappingOK := owner.source.Host().Globals().Mapping(global)
		if !ok || !idOK || !mappingOK || !owner.addBoundary(boundaryRow{kind: BoundaryGlobal, id: id, global: global}, []linkmodule.AnalysisRoot{root}) {
			return false
		}
	}
	return true
}

func (owner *schema) addBoundary(row boundaryRow, roots []linkmodule.AnalysisRoot) bool {
	if row.kind == BoundaryInvalid || !row.id.Available() {
		return false
	}
	normalized, ok := owner.normalizeRoots(roots)
	if !ok {
		return false
	}
	class, ok := owner.internRootClass(normalized)
	return ok && owner.addBoundaryClass(row, class)
}

func (owner *schema) addBoundaryClass(row boundaryRow, class uint32) bool {
	if row.kind == BoundaryInvalid || !row.id.Available() || uint64(class) >= uint64(len(owner.rootClasses)) {
		return false
	}
	identity := boundaryIdentity{kind: row.kind, id: row.id}
	if existing := owner.boundaryIndex[identity]; existing != 0 {
		oldClass := owner.boundaries[existing-1].class
		if oldClass == class {
			return true
		}
		merged, mergedOK := owner.mergeRoots(owner.rootClasses[oldClass], owner.rootClasses[class])
		if !mergedOK {
			return false
		}
		mergedClass, ok := owner.internRootClass(merged)
		if !ok {
			return false
		}
		owner.boundaries[existing-1].class = mergedClass
		return true
	}
	if uint64(len(owner.boundaries)) >= uint64(^uint32(0)) {
		return false
	}
	row.class = class
	owner.boundaries = append(owner.boundaries, row)
	owner.boundaryIndex[identity] = uint32(len(owner.boundaries))
	return true
}

func (owner *schema) normalizeRoots(roots []linkmodule.AnalysisRoot) ([]linkmodule.AnalysisRoot, bool) {
	result := append([]linkmodule.AnalysisRoot(nil), roots...)
	for _, root := range result {
		if _, ok := owner.source.Module().Roots().ID(root); !ok {
			return nil, false
		}
	}
	rootView := owner.source.Module().Roots()
	sort.Slice(result, func(left, right int) bool {
		order, ok := rootView.Compare(result[left], result[right])
		return ok && order < 0
	})
	end := 0
	for _, root := range result {
		if end == 0 || result[end-1] != root {
			result[end] = root
			end++
		}
	}
	return result[:end], true
}

func (owner *schema) rootClassHash(roots []linkmodule.AnalysisRoot) (uint64, bool) {
	if owner == nil || owner.source == nil {
		return 0, false
	}
	hash := uint64(1469598103934665603)
	for _, root := range roots {
		id, ok := owner.source.Module().Roots().ID(root)
		if !ok {
			return 0, false
		}
		for _, byte := range id {
			hash ^= uint64(byte)
			hash *= 1099511628211
		}
	}
	return hash, true
}

func (owner *schema) internRootClass(roots []linkmodule.AnalysisRoot) (uint32, bool) {
	hash, ok := owner.rootClassHash(roots)
	if !ok {
		return 0, false
	}
	for _, id := range owner.classBuckets[hash] {
		candidate := owner.rootClasses[id]
		if len(candidate) != len(roots) {
			continue
		}
		equal := true
		for index := range roots {
			if candidate[index] != roots[index] {
				equal = false
				break
			}
		}
		if equal {
			return id, true
		}
	}
	if uint64(len(owner.rootClasses)) >= uint64(^uint32(0)) {
		return 0, false
	}
	id := uint32(len(owner.rootClasses))
	owner.rootClasses = append(owner.rootClasses, append([]linkmodule.AnalysisRoot(nil), roots...))
	owner.classBuckets[hash] = append(owner.classBuckets[hash], id)
	return id, true
}

func (owner *schema) mergeRoots(left, right []linkmodule.AnalysisRoot) ([]linkmodule.AnalysisRoot, bool) {
	if owner == nil || owner.source == nil {
		return nil, false
	}
	roots := owner.source.Module().Roots()
	result := make([]linkmodule.AnalysisRoot, 0, len(left)+len(right))
	for li, ri := 0, 0; li < len(left) || ri < len(right); {
		if li == len(left) {
			result = append(result, right[ri:]...)
			break
		}
		if ri == len(right) {
			result = append(result, left[li:]...)
			break
		}
		order, ok := roots.Compare(left[li], right[ri])
		if !ok {
			return nil, false
		}
		switch {
		case order < 0:
			result = append(result, left[li])
			li++
		case order > 0:
			result = append(result, right[ri])
			ri++
		default:
			result = append(result, left[li])
			li++
			ri++
		}
	}
	return result, true
}

func (owner *schema) finish() bool {
	if owner == nil || owner.source == nil {
		return false
	}
	potential, ok := owner.exactPotential()
	if !ok {
		return false
	}
	owner.potential = potential
	owner.id = residenceContentID(owner.linkID)
	if !owner.id.Available() {
		return false
	}
	owner.bottom = Value{owner: owner}
	owner.top = Value{owner: owner, top: true}
	return true
}

// exactPotential derives the finite relation cardinality from the sealed
// vocabulary itself.  Keeping this as enumeration rather than a product
// constant makes the rank proof stay correct when a lawful alternative is
// added or removed: Fact.valid is the sole semantic admission authority.
func (owner *schema) exactPotential() (uint64, bool) {
	if owner == nil {
		return 0, false
	}
	roles := [...]materialization.Role{materialization.Exact, materialization.Recent, materialization.Summary}
	locations := [...]Location{ActorLocal, Shared, Module, Global}
	retentions := [...]Retention{NotRetained, Retained}
	survivals := [...]Survival{Dead, Live}
	lastUses := [...]LastUse{LastUseEligible, LastUseRevoked}
	var count uint64
	for root := uint32(1); int(root) <= len(owner.allocations); root++ {
		for _, role := range roles {
			for _, location := range locations {
				for _, retention := range retentions {
					for _, survival := range survivals {
						for _, lastUse := range lastUses {
							fact := Fact{owner: owner, reference: Reference{owner: owner, root: root, role: role}, location: location, retention: retention, survival: survival, lastUse: lastUse}
							if !fact.valid() {
								continue
							}
							if count == ^uint64(0) {
								return 0, false
							}
							count++
						}
					}
				}
			}
		}
	}
	return count, true
}

func residenceContentID(linkID keyspace.ContentID) (id keyspace.ContentID) {
	var payload [32 + 2*8]byte
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:40], schemaFormat)
	binary.BigEndian.PutUint64(payload[40:48], 2)
	digest := sha256.Sum256(payload[:])
	copy(id[:], digest[:])
	return id
}

func (schema Schema) valid() bool {
	return schema.owner != nil && schema.owner.source != nil && schema.owner.heap.Valid() && schema.owner.linkID.Available() && schema.owner.id.Available() &&
		schema.owner.heap.Link() == schema.owner.source && schema.owner.source.ContentID() == schema.owner.linkID && schema.owner.heap.LinkContentID() == schema.owner.linkID
}
func (schema Schema) ContentID() keyspace.ContentID {
	if !schema.valid() {
		return keyspace.ContentID{}
	}
	return schema.owner.id
}
func (schema Schema) LinkContentID() keyspace.ContentID {
	if !schema.valid() {
		return keyspace.ContentID{}
	}
	return schema.owner.linkID
}

func (schema Schema) Rebind(source *link.Link) (Schema, bool) {
	if !schema.valid() || source == nil || source.ContentID() != schema.owner.linkID {
		return Schema{}, false
	}
	heapSchema, heapOK := schema.owner.heap.Rebind(source)
	if !heapOK {
		return Schema{}, false
	}
	rebound, ok := Seal(source, heapSchema)
	if !ok || rebound.ContentID() != schema.ContentID() {
		return Schema{}, false
	}
	return rebound, true
}

func (schema Schema) KeyCount() int {
	if !schema.valid() {
		return 0
	}
	return len(schema.owner.boundaries)
}

func (schema Schema) KeyAt(index int) (Key, bool) {
	if !schema.valid() || index < 0 || index >= len(schema.owner.boundaries) {
		return Key{}, false
	}
	return Key{owner: schema.owner, id: uint32(index + 1)}, true
}

// keyFor returns the owner-issued Key for one already validated exact boundary.
// boundaryIndex is the canonical reverse index for every Residence family.
func (schema Schema) keyFor(kind BoundaryKind, id keyspace.ContentID) (Key, bool) {
	if !schema.valid() || kind == BoundaryInvalid || !id.Available() {
		return Key{}, false
	}
	key := Key{owner: schema.owner, id: schema.owner.boundaryIndex[boundaryIdentity{kind: kind, id: id}]}
	return key, key.valid()
}

// KeyForCapture returns the exact Residence key for an executable Program
// closure capture in this schema's project topology.
func (schema Schema) KeyForCapture(shard linkproject.Shard, function keyspace.Term, index int) (Key, bool) {
	if !schema.valid() || index < 0 || uint64(index) > uint64(^uint32(0)) {
		return Key{}, false
	}
	coordinate := programBoundary{shard: shard, term: function, index: uint32(index)}
	if !schema.owner.validProgramBoundary(BoundaryCapture, coordinate) {
		return Key{}, false
	}
	return schema.keyFor(BoundaryCapture, schema.owner.programBoundaryID(BoundaryCapture, coordinate))
}

// KeyForStore returns the exact Residence key for an executable Program
// Write whose target is a Lens.
func (schema Schema) KeyForStore(shard linkproject.Shard, write keyspace.Term) (Key, bool) {
	if !schema.valid() {
		return Key{}, false
	}
	coordinate := programBoundary{shard: shard, term: write}
	if !schema.owner.validProgramBoundary(BoundaryStore, coordinate) {
		return Key{}, false
	}
	return schema.keyFor(BoundaryStore, schema.owner.programBoundaryID(BoundaryStore, coordinate))
}

// KeyForReturn returns the exact Residence key for an executable Program
// Return relation.
func (schema Schema) KeyForReturn(shard linkproject.Shard, term keyspace.Term) (Key, bool) {
	if !schema.valid() {
		return Key{}, false
	}
	coordinate := programBoundary{shard: shard, term: term}
	if !schema.owner.validProgramBoundary(BoundaryReturn, coordinate) {
		return Key{}, false
	}
	return schema.keyFor(BoundaryReturn, schema.owner.programBoundaryID(BoundaryReturn, coordinate))
}

// KeyForModuleCacheEntry returns the exact Residence key for one sealed
// module-cache ingress boundary from this Schema's Link.
func (schema Schema) KeyForModuleCacheEntry(entry linkmodule.ModuleCacheEntry) (Key, bool) {
	if !schema.valid() {
		return Key{}, false
	}
	id, ok := schema.owner.source.Module().Cache().EntryID(entry)
	if !ok {
		return Key{}, false
	}
	return schema.keyFor(BoundaryModuleEntry, id)
}

// KeyForModuleCoordinate returns the exact Residence key for one sealed
// module-cache coordinate boundary from this Schema's Link.
func (schema Schema) KeyForModuleCoordinate(coordinate linkmodule.ModuleCoordinate) (Key, bool) {
	if !schema.valid() {
		return Key{}, false
	}
	id, ok := schema.owner.source.Module().Coordinates().ID(coordinate)
	if !ok {
		return Key{}, false
	}
	return schema.keyFor(BoundaryModuleCoordinate, id)
}

// KeyForGlobalBinding returns the exact Residence key for one sealed global
// binding boundary. GlobalBinding is a pre-existing Link-local scalar rather
// than a source-stamped opaque handle, so its issuing Link is required to
// make foreign use explicit and reject matching ordinals from another Link.
func (schema Schema) KeyForGlobalBinding(source *link.Link, binding linkhost.GlobalBinding) (Key, bool) {
	if !schema.valid() || source != schema.owner.source {
		return Key{}, false
	}
	id, ok := residenceGlobalBindingID(source, binding)
	if !ok {
		return Key{}, false
	}
	return schema.keyFor(BoundaryGlobal, id)
}

// residenceGlobalBindingID projects Host's detached identity for one exact
// global binding. Residence consumes that owner-issued identity directly; it
// neither exposes a host ordinal nor recreates Host's semantic key.
func residenceGlobalBindingID(source *link.Link, binding linkhost.GlobalBinding) (keyspace.ContentID, bool) {
	if source == nil || !source.ContentID().Available() || source.Host() == nil {
		return keyspace.ContentID{}, false
	}
	return source.Host().Globals().ID(binding)
}

// AdmitsAt is the exact factored State×K admission relation. It performs no
// root×boundary materialization and is logarithmic in the admitted root class.
func (schema Schema) AdmitsAt(root linkmodule.AnalysisRoot, key Key) bool {
	if !schema.valid() || !key.valid() || key.owner != schema.owner {
		return false
	}
	roots := schema.owner.rootClasses[schema.owner.boundaries[key.id-1].class]
	ownerRoots := schema.owner.source.Module().Roots()
	index := sort.Search(len(roots), func(index int) bool {
		order, ok := ownerRoots.Compare(roots[index], root)
		return ok && order >= 0
	})
	if index >= len(roots) {
		return false
	}
	order, ok := ownerRoots.Compare(roots[index], root)
	return ok && order == 0
}
