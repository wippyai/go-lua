package residence

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	programartifact "github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

const schemaFormat uint64 = 0x7265736964656e63 // "residenc"

// Schema is the sealed Residence family for one exact Link.  Its published
// storage is a scalar receipt: no Project, Boundary, Host, Module, Program or
// owner-fenced Link coordinate survives the seal.
type Schema struct{ owner *schema }

type schema struct {
	linkOwner link.OwnerCapability
	heap      heap.Schema
	id        keyspace.ContentID

	allocations     []heap.Key
	allocationIndex map[heap.Key]uint32

	boundaries    []boundaryRow
	boundaryIndex map[boundaryIdentity]uint32
	rootClasses   [][]keyspace.ContentID
	classBuckets  map[uint64][]uint32
	mountClasses  map[keyspace.ContentID]uint32

	potential   uint64
	bottom, top Value
}

type boundaryIdentity struct {
	kind BoundaryKind
	id   keyspace.ContentID
}
type boundaryRow struct {
	kind  BoundaryKind
	id    keyspace.ContentID
	class uint32
}

// SealWithArtifacts is cold ingress. It consumes the source Link only long
// enough to authenticate scalar mount/application/global/root identities;
// none of its handles enter the sealed Schema.
func SealWithArtifacts(source *link.Link, heapSchema heap.Schema, mounts []ArtifactMount) (Schema, bool) {
	if source == nil {
		return Schema{}, false
	}
	linkOwner := source.OwnerCapability()
	if !linkOwner.Available() || source.Project() == nil || source.Module() == nil || source.Host() == nil || !heapSchema.Valid() || !heapSchema.LinkOwner().Matches(linkOwner) || len(mounts) == 0 || len(mounts) != source.Project().Mounts().Count() {
		return Schema{}, false
	}
	owner := &schema{
		linkOwner: linkOwner, heap: heapSchema,
		allocationIndex: make(map[heap.Key]uint32), boundaryIndex: make(map[boundaryIdentity]uint32),
		classBuckets: make(map[uint64][]uint32), mountClasses: make(map[keyspace.ContentID]uint32, len(mounts)),
	}
	if !owner.addMountClasses(source, mounts) || !owner.addAllocations() || !owner.addProgramBoundaries(mounts) ||
		!owner.addApplicationBoundaries(source) || !owner.addModuleBoundaries(source) || !owner.addGlobalBoundaries(source) || !owner.finish() {
		return Schema{}, false
	}
	return Schema{owner: owner}, true
}

// residenceMountID turns the otherwise owner-fenced Project Shard position
// into a Link-specific immutable receipt. Ordinal is consumed only here at
// cold ingress, so duplicate mounts remain distinct without retaining Shard.
func residenceMountID(owner link.OwnerCapability, module, program keyspace.ContentID, ordinal int) keyspace.ContentID {
	if !owner.Available() || !module.Available() || !program.Available() || ordinal < 0 {
		return keyspace.ContentID{}
	}
	linkID := owner.ContentID()
	var image [112]byte
	copy(image[:32], linkID[:])
	copy(image[32:64], module[:])
	copy(image[64:96], program[:])
	binary.BigEndian.PutUint64(image[96:104], 0x7265732d6d6e74) // res-mnt
	binary.BigEndian.PutUint64(image[104:112], uint64(ordinal))
	return keyspace.ContentID(sha256.Sum256(image[:]))
}

func (owner *schema) addMountClasses(source *link.Link, mounts []ArtifactMount) bool {
	for index, mount := range mounts {
		shard, shardOK := source.Project().Mounts().At(index)
		module, moduleOK := source.Project().ModuleKey(shard)
		program, programOK := source.Project().Mounts().ProgramID(shard)
		expected := residenceMountID(owner.linkOwner, module, program, index)
		if !shardOK || !moduleOK || !programOK || !mount.Available() || mount.mount != expected || mount.module != module || mount.program != program || mount.artifact.CompileKey().ProgramID() != program {
			return false
		}
		rootCount := source.Module().Roots().ForShardCount(shard)
		roots := make([]keyspace.ContentID, 0, rootCount)
		rootsOK := true
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			root, rootOK := source.Module().Roots().ForShardAt(shard, rootIndex)
			rootID, rootIDOK := source.Module().Roots().ID(root)
			if !rootOK || !rootIDOK || !rootID.Available() {
				rootsOK = false
				break
			}
			roots = append(roots, rootID)
		}
		class, classOK := owner.internRootClass(roots)
		if !rootsOK || !classOK {
			return false
		}
		if _, duplicate := owner.mountClasses[mount.mount]; duplicate {
			return false
		}
		owner.mountClasses[mount.mount] = class
	}
	return true
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

func (owner *schema) addApplicationBoundaries(source *link.Link) bool {
	boundary := source.Boundary()
	contract, contractOK := boundary.Target()
	if boundary == nil || !contractOK || contract == nil {
		return false
	}
	for index := 0; index < source.Project().Applications().Count(); index++ {
		application, appOK := source.Project().Applications().At(index)
		appID, appIDOK := source.Project().ApplicationID(application)
		shard, _, callOK := source.Project().Applications().Call(application)
		mountIndex, mountIndexOK := source.Project().Mounts().Index(shard)
		if !appOK || !appIDOK || !callOK || !mountIndexOK {
			continue
		}
		mountShard, mountOK := source.Project().Mounts().At(mountIndex)
		module, moduleOK := source.Project().ModuleKey(mountShard)
		program, programOK := source.Project().Mounts().ProgramID(mountShard)
		mountID := residenceMountID(owner.linkOwner, module, program, mountIndex)
		class, classOK := owner.mountClasses[mountID]
		if !mountOK || !moduleOK || !programOK || !classOK || !appID.Available() {
			return false
		}
		for opIndex := 0; opIndex < contract.OperationCount(); opIndex++ {
			operation, opOK := contract.OperationAt(opIndex)
			if !opOK {
				return false
			}
			if !boundary.ApplicationOperationAvailable(contract, application, operation) {
				continue
			}
			appendPort := func(kind BoundaryKind, port uint32) bool {
				return owner.addBoundaryClass(boundaryRow{kind: kind, id: applicationBoundaryID(owner.linkOwner, appID, operation, kind, port)}, class)
			}
			for portIndex := 0; portIndex < contract.CallbackCount(operation); portIndex++ {
				callback, ok := contract.CallbackAt(operation, portIndex)
				if !ok || !appendPort(BoundaryCallback, uint32(callback)) {
					return false
				}
			}
			for portIndex := 0; portIndex < contract.SuspensionCount(operation); portIndex++ {
				if !appendPort(BoundarySuspension, uint32(portIndex)) {
					return false
				}
			}
			for portIndex := 0; portIndex < contract.TransferCount(operation); portIndex++ {
				transfer, ok := contract.TransferIDAt(operation, portIndex)
				if !ok || !appendPort(BoundaryTransfer, uint32(transfer)) {
					return false
				}
			}
			for portIndex := 0; portIndex < contract.ResumeCount(operation); portIndex++ {
				resume, ok := contract.ResumeIDAt(operation, portIndex)
				if !ok || !appendPort(BoundaryResume, uint32(resume)) {
					return false
				}
			}
		}
	}
	return true
}

func applicationBoundaryID(owner link.OwnerCapability, applicationID keyspace.ContentID, operation target.Operation, kind BoundaryKind, port uint32) keyspace.ContentID {
	if !owner.Available() || !applicationID.Available() {
		return keyspace.ContentID{}
	}
	linkID := owner.ContentID()
	var image [96]byte
	copy(image[:32], linkID[:])
	copy(image[32:64], applicationID[:])
	binary.BigEndian.PutUint64(image[64:], 0x7265732d6170706f)
	binary.BigEndian.PutUint64(image[72:], uint64(operation))
	binary.BigEndian.PutUint64(image[80:], uint64(kind))
	binary.BigEndian.PutUint64(image[88:], uint64(port))
	return keyspace.ContentID(sha256.Sum256(image[:]))
}

func (owner *schema) addModuleBoundaries(source *link.Link) bool {
	module := source.Module()
	for index := 0; index < module.Cache().EntryCount(); index++ {
		entry, entryOK := module.Cache().EntryAt(index)
		entryID, idOK := module.Cache().EntryID(entry)
		_, from, to, mappingOK := module.Cache().EntryMapping(entry)
		fromID, fromOK := module.Roots().ID(from)
		toID, toOK := module.Roots().ID(to)
		if !entryOK || !idOK || !mappingOK || !fromOK || !toOK || !owner.addBoundary(boundaryRow{kind: BoundaryModuleEntry, id: entryID}, []keyspace.ContentID{fromID, toID}) {
			return false
		}
	}
	// This construction-local tuple preserves Module's exact cache transport
	// law. It is consumed into scalar coordinate/root IDs before Schema is
	// returned, so no Project Shard, Actor or Module instance survives.
	type coordinateScope struct {
		actor          linkmodule.Actor
		shard          linkproject.Shard
		representative linkmodule.ModuleCacheInstance
	}
	scopes := make(map[coordinateScope][]keyspace.ContentID)
	for index := 0; index < module.Roots().Count(); index++ {
		root, rootOK := module.Roots().At(index)
		coordinate, coordinateOK := module.Coordinates().ForRoot(root)
		actor, shard, representative, mappingOK := module.Coordinates().Mapping(coordinate)
		rootID, rootIDOK := module.Roots().ID(root)
		if !rootOK || !coordinateOK || !mappingOK || !rootIDOK {
			return false
		}
		scopes[coordinateScope{actor: actor, shard: shard, representative: representative}] = append(scopes[coordinateScope{actor: actor, shard: shard, representative: representative}], rootID)
	}
	for index := 0; index < module.Cache().EntryCount(); index++ {
		entry, entryOK := module.Cache().EntryAt(index)
		_, from, to, mappingOK := module.Cache().EntryMapping(entry)
		_, fromActor, fromInstance, fromOK := module.Roots().Mapping(from)
		toShard, toActor, _, toOK := module.Roots().Mapping(to)
		representative, representativeOK := module.Cache().Representative(fromInstance)
		toID, toOK := module.Roots().ID(to)
		if !entryOK || !mappingOK || !fromOK || !toOK || !representativeOK || fromActor != toActor {
			return false
		}
		tuple := coordinateScope{actor: fromActor, shard: toShard, representative: representative}
		scopes[tuple] = append(scopes[tuple], toID)
	}
	for index := 0; index < module.Coordinates().Count(); index++ {
		coordinate, coordinateOK := module.Coordinates().At(index)
		coordinateID, idOK := module.Coordinates().ID(coordinate)
		actor, shard, representative, mappingOK := module.Coordinates().Mapping(coordinate)
		roots := scopes[coordinateScope{actor: actor, shard: shard, representative: representative}]
		if !coordinateOK || !idOK || !mappingOK || len(roots) == 0 || !owner.addBoundary(boundaryRow{kind: BoundaryModuleCoordinate, id: coordinateID}, roots) {
			return false
		}
	}
	return true
}

func (owner *schema) addGlobalBoundaries(source *link.Link) bool {
	globals := source.Host().Globals()
	for index := 0; index < globals.Count(); index++ {
		global, globalOK := globals.At(index)
		globalID, idOK := globals.ID(global)
		root, _, _, _, _, _, mappingOK := globals.Mapping(global)
		rootID, rootIDOK := source.Module().Roots().ID(root)
		if !globalOK || !idOK || !mappingOK || !rootIDOK || !owner.addBoundary(boundaryRow{kind: BoundaryGlobal, id: globalID}, []keyspace.ContentID{rootID}) {
			return false
		}
	}
	return true
}

func (owner *schema) addBoundary(row boundaryRow, roots []keyspace.ContentID) bool {
	normalized, ok := normalizeRoots(roots)
	if !ok {
		return false
	}
	class, ok := owner.internRootClass(normalized)
	return ok && owner.addBoundaryClass(row, class)
}

func (owner *schema) addBoundaryClass(row boundaryRow, class uint32) bool {
	if row.kind == BoundaryInvalid || !row.id.Available() || int(class) >= len(owner.rootClasses) {
		return false
	}
	identity := boundaryIdentity{kind: row.kind, id: row.id}
	if existing := owner.boundaryIndex[identity]; existing != 0 {
		old := owner.boundaries[existing-1]
		if old.class == class {
			return true
		}
		merged, ok := mergeRootIDs(owner.rootClasses[old.class], owner.rootClasses[class])
		if !ok {
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

func normalizeRoots(roots []keyspace.ContentID) ([]keyspace.ContentID, bool) {
	if len(roots) == 0 {
		return nil, false
	}
	result := append([]keyspace.ContentID(nil), roots...)
	for _, id := range result {
		if !id.Available() {
			return nil, false
		}
	}
	sort.Slice(result, func(left, right int) bool { return bytes.Compare(result[left][:], result[right][:]) < 0 })
	end := 1
	for index := 1; index < len(result); index++ {
		if result[index] != result[end-1] {
			result[end] = result[index]
			end++
		}
	}
	return result[:end], true
}

func (owner *schema) internRootClass(roots []keyspace.ContentID) (uint32, bool) {
	normalized, ok := normalizeRoots(roots)
	if !ok {
		return 0, false
	}
	hash := uint64(1469598103934665603)
	for _, id := range normalized {
		for _, value := range id {
			hash ^= uint64(value)
			hash *= 1099511628211
		}
	}
	for _, candidateID := range owner.classBuckets[hash] {
		candidate := owner.rootClasses[candidateID]
		if len(candidate) == len(normalized) && equalRootIDs(candidate, normalized) {
			return candidateID, true
		}
	}
	if uint64(len(owner.rootClasses)) >= uint64(^uint32(0)) {
		return 0, false
	}
	id := uint32(len(owner.rootClasses))
	owner.rootClasses = append(owner.rootClasses, normalized)
	owner.classBuckets[hash] = append(owner.classBuckets[hash], id)
	return id, true
}

func equalRootIDs(left, right []keyspace.ContentID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
func mergeRootIDs(left, right []keyspace.ContentID) ([]keyspace.ContentID, bool) {
	return normalizeRoots(append(append([]keyspace.ContentID(nil), left...), right...))
}

func (owner *schema) finish() bool {
	potential, ok := owner.exactPotential()
	if !ok {
		return false
	}
	owner.potential = potential
	owner.id = residenceContentID(owner.linkOwner)
	if !owner.id.Available() {
		return false
	}
	owner.bottom = Value{owner: owner}
	owner.top = Value{owner: owner, top: true}
	return true
}

func (owner *schema) exactPotential() (uint64, bool) {
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
							if fact.valid() {
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
	}
	return count, true
}

func residenceContentID(owner link.OwnerCapability) (id keyspace.ContentID) {
	if !owner.Available() {
		return keyspace.ContentID{}
	}
	linkID := owner.ContentID()
	var payload [48]byte
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:40], schemaFormat)
	binary.BigEndian.PutUint64(payload[40:48], 3)
	return keyspace.ContentID(sha256.Sum256(payload[:]))
}

func (schema Schema) valid() bool {
	return schema.owner != nil && schema.owner.linkOwner.Available() && schema.owner.heap.Valid() && schema.owner.heap.LinkOwner().Matches(schema.owner.linkOwner) && schema.owner.id.Available()
}
func (schema Schema) ContentID() keyspace.ContentID {
	if !schema.valid() {
		return keyspace.ContentID{}
	}
	return schema.owner.id
}
func (schema Schema) LinkOwner() link.OwnerCapability {
	if !schema.valid() {
		return link.OwnerCapability{}
	}
	return schema.owner.linkOwner
}
func (schema Schema) Owner() link.OwnerCapability { return schema.LinkOwner() }
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
func (schema Schema) keyFor(kind BoundaryKind, id keyspace.ContentID) (Key, bool) {
	if !schema.valid() || kind == BoundaryInvalid || !id.Available() {
		return Key{}, false
	}
	key := Key{owner: schema.owner, id: schema.owner.boundaryIndex[boundaryIdentity{kind: kind, id: id}]}
	return key, key.valid()
}
func (schema Schema) KeyForCapture(mount ArtifactMount, row programartifact.BoundaryRow) (Key, bool) {
	if !schema.valid() || !schema.owner.validArtifactBoundary(BoundaryCapture, mount, row) {
		return Key{}, false
	}
	return schema.keyFor(BoundaryCapture, residenceProgramBoundaryID(schema.owner.linkOwner, mount.mount, row))
}
func (schema Schema) KeyForStore(mount ArtifactMount, row programartifact.BoundaryRow) (Key, bool) {
	if !schema.valid() || !schema.owner.validArtifactBoundary(BoundaryStore, mount, row) {
		return Key{}, false
	}
	return schema.keyFor(BoundaryStore, residenceProgramBoundaryID(schema.owner.linkOwner, mount.mount, row))
}
func (schema Schema) KeyForReturn(mount ArtifactMount, row programartifact.BoundaryRow) (Key, bool) {
	if !schema.valid() || !schema.owner.validArtifactBoundary(BoundaryReturn, mount, row) {
		return Key{}, false
	}
	return schema.keyFor(BoundaryReturn, residenceProgramBoundaryID(schema.owner.linkOwner, mount.mount, row))
}

// AdmitsAt receives a detached Module-root identity. A caller cannot keep or
// smuggle an owner-fenced module coordinate through a sealed Residence value.
func (schema Schema) AdmitsAt(root keyspace.ContentID, key Key) bool {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || !root.Available() {
		return false
	}
	roots := schema.owner.rootClasses[schema.owner.boundaries[key.id-1].class]
	index := sort.Search(len(roots), func(index int) bool { return bytes.Compare(roots[index][:], root[:]) >= 0 })
	return index < len(roots) && roots[index] == root
}
