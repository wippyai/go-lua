package ownership

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

// OriginKind is Ownership's closed source-duty vocabulary.  Link supplies
// typed structural constituents; only Ownership forms their union, assigns
// identities, and decides whether a constituent carries a duty.
type OriginKind uint8

const (
	OriginInvalid OriginKind = iota
	OriginAllocationRoot
	OriginCaptureRetention
	OriginStoreCommit
	OriginReturnBoundary
	OriginGlobalBinding
	OriginApplication
	OriginInput
	OriginOutcome
	OriginCallbackOccurrence
	OriginModuleCacheEntry
	OriginModuleCoordinate
	OriginModuleInitGeneration
	OriginSuspensionOccurrence
	OriginResumeOccurrence
	OriginTransferArmOccurrence
	OriginBootRoot
	OriginHostExposure
	originKindLimit
)

func (kind OriginKind) Valid() bool { return kind > OriginInvalid && kind < originKindLimit }

// Origin is an Ownership-issued opaque source coordinate.  It is not a Link
// alias: its validity, identity and rebinding are all owned by Schema.
type Origin struct {
	owner *schema
	index uint32
}

func (origin Origin) valid() bool {
	return origin.owner != nil && origin.index != 0 && uint64(origin.index) <= uint64(len(origin.owner.origins))
}

func (origin Origin) Valid() bool { return origin.valid() }

func (origin Origin) Kind() OriginKind {
	if !origin.valid() {
		return OriginInvalid
	}
	return origin.owner.origins[origin.index-1].kind
}

// OriginRef is a portable Ownership-origin identity.  Link content fences
// replay, but Link does not issue or interpret this domain coordinate.
type OriginRef struct {
	linkID keyspace.ContentID
	kind   OriginKind
	id     keyspace.ContentID
}

func (ref OriginRef) LinkID() keyspace.ContentID { return ref.linkID }
func (ref OriginRef) Kind() OriginKind           { return ref.kind }
func (ref OriginRef) ID() keyspace.ContentID     { return ref.id }

type originRow struct {
	kind       OriginKind
	ordinal    uint32
	id         keyspace.ContentID
	roots      []linkmodule.AnalysisRoot
	allocation heap.Key
	port       originPort
	program    programOrigin
}

// programOrigin is Ownership's direct coordinate for executable Program
// structure that carries an ownership duty.  Link supplies only the sealed
// shard-to-Program topology; it does not materialize a retention union or
// issue another handle for these relations.
//
// CaptureRetention uses term=function and index=capture position.
// StoreCommit uses term=write and index=0.
// ReturnBoundary uses term=return and index=0.
type programOrigin struct {
	shard linkproject.Shard
	term  keyspace.Term
	index uint32
}

// originPort is Ownership's private application×Target coordinate.  It never
// stores a Link application-input, outcome, or occurrence handle.
type originPort struct {
	application linkproject.Application
	operation   target.Operation
	transfer    target.TransferID
	port        uint32
}

// OriginCount and OriginAt enumerate the complete closed source range used by
// Ownership.  Constituents remain typed Link rows; no Link union exists.
func (schema Schema) OriginCount() int {
	if !schema.Valid() {
		return 0
	}
	return len(schema.owner.origins)
}

func (schema Schema) OriginAt(index int) (Origin, bool) {
	if !schema.Valid() || index < 0 || index >= len(schema.owner.origins) {
		return Origin{}, false
	}
	return Origin{owner: schema.owner, index: uint32(index + 1)}, true
}

func (schema Schema) origin(origin Origin) (originRow, bool) {
	if !schema.Valid() || !origin.valid() || origin.owner != schema.owner {
		return originRow{}, false
	}
	return schema.owner.origins[origin.index-1], true
}

func (schema Schema) OriginID(origin Origin) (keyspace.ContentID, bool) {
	row, ok := schema.origin(origin)
	if !ok || !row.id.Available() {
		return keyspace.ContentID{}, false
	}
	return row.id, true
}

func (schema Schema) OriginRef(origin Origin) (OriginRef, bool) {
	row, ok := schema.origin(origin)
	if !ok || !row.id.Available() {
		return OriginRef{}, false
	}
	return OriginRef{linkID: schema.owner.linkID, kind: row.kind, id: row.id}, true
}

func (schema Schema) FindOrigin(ref OriginRef) (Origin, bool) {
	if !schema.Valid() || ref.linkID != schema.owner.linkID || !ref.kind.Valid() || !ref.id.Available() {
		return Origin{}, false
	}
	index, ok := schema.owner.originIndex[ref.id]
	if !ok || index == 0 {
		return Origin{}, false
	}
	origin := Origin{owner: schema.owner, index: index}
	if origin.Kind() != ref.kind {
		return Origin{}, false
	}
	return origin, true
}

func (schema Schema) FindOriginID(id keyspace.ContentID) (Origin, bool) {
	if !schema.Valid() || !id.Available() {
		return Origin{}, false
	}
	index, ok := schema.owner.originIndex[id]
	if !ok || index == 0 {
		return Origin{}, false
	}
	return Origin{owner: schema.owner, index: index}, true
}

func (schema Schema) RebindOrigin(origin Origin) (Origin, bool) {
	if !schema.Valid() || !origin.valid() {
		return Origin{}, false
	}
	ref, ok := Schema{owner: origin.owner}.OriginRef(origin)
	if !ok {
		return Origin{}, false
	}
	return schema.FindOrigin(ref)
}

func (schema Schema) OriginAnalysisRootCount(origin Origin) int {
	row, ok := schema.origin(origin)
	if !ok {
		return 0
	}
	return len(row.roots)
}

func (schema Schema) OriginAnalysisRootAt(origin Origin, index int) (linkmodule.AnalysisRoot, bool) {
	row, ok := schema.origin(origin)
	if !ok || index < 0 || index >= len(row.roots) {
		return linkmodule.AnalysisRoot{}, false
	}
	return row.roots[index], true
}

func (schema Schema) OriginHeapKey(origin Origin) (heap.Key, bool) {
	row, ok := schema.origin(origin)
	if !ok || row.kind != OriginAllocationRoot || !schema.owner.heap.OwnsKey(row.allocation) {
		return heap.Key{}, false
	}
	return row.allocation, true
}

func buildOrigins(source *link.Link, heapSchema heap.Schema) ([]originRow, map[keyspace.ContentID]uint32, bool) {
	if source == nil || !source.ContentID().Available() || !heapSchema.Valid() || heapSchema.LinkContentID() != source.ContentID() {
		return nil, nil, false
	}
	counts := [originKindLimit]int{
		OriginGlobalBinding: source.Host().Globals().Count(), OriginApplication: source.Project().Applications().Calls().Count(),
		OriginModuleCacheEntry: source.Module().Cache().EntryCount(),
		OriginModuleCoordinate: source.Module().Coordinates().Count(), OriginModuleInitGeneration: source.Module().Generations().Count(),
		OriginBootRoot: source.Host().BootRoots().Count(), OriginHostExposure: source.Host().Exposures().Count(),
	}
	rows := make([]originRow, 0)
	index := make(map[keyspace.ContentID]uint32)
	if !appendAllocationOrigins(source, heapSchema, &rows, index) {
		return nil, nil, false
	}
	for kind := OriginKind(1); kind < originKindLimit; kind++ {
		if kind == OriginAllocationRoot {
			continue
		}
		if kind == OriginInput || kind == OriginOutcome || kind == OriginCallbackOccurrence || kind == OriginSuspensionOccurrence || kind == OriginResumeOccurrence || kind == OriginTransferArmOccurrence {
			continue
		}
		if kind == OriginCaptureRetention || kind == OriginStoreCommit || kind == OriginReturnBoundary {
			if !appendProgramOrigins(source, kind, &rows, index) {
				return nil, nil, false
			}
			continue
		}
		for ordinal := 1; ordinal <= counts[kind]; ordinal++ {
			roots, ok := originRoots(source, kind, uint32(ordinal))
			if !ok {
				return nil, nil, false
			}
			id := ownershipOriginID(source.ContentID(), kind, uint32(ordinal))
			if !id.Available() || index[id] != 0 || uint64(len(rows)) >= uint64(^uint32(0)) {
				return nil, nil, false
			}
			rows = append(rows, originRow{kind: kind, ordinal: uint32(ordinal), id: id, roots: roots})
			index[id] = uint32(len(rows))
		}
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return nil, nil, false
	}
	appendPort := func(kind OriginKind, port originPort, roots []linkmodule.AnalysisRoot) bool {
		ordinal := uint32(len(rows) + 1)
		id := ownershipOriginID(source.ContentID(), kind, ordinal)
		if !id.Available() || index[id] != 0 || uint64(len(rows)) >= uint64(^uint32(0)) {
			return false
		}
		rows = append(rows, originRow{kind: kind, ordinal: ordinal, id: id, roots: roots, port: port})
		index[id] = uint32(len(rows))
		return true
	}
	for appIndex := 0; appIndex < source.Project().Applications().Count(); appIndex++ {
		application, found := source.Project().Applications().At(appIndex)
		if !found {
			return nil, nil, false
		}
		roots, found := applicationRoots(source, application)
		if !found {
			continue
		}
		for opIndex := 0; opIndex < contract.OperationCount(); opIndex++ {
			operation, found := contract.OperationAt(opIndex)
			if !found {
				return nil, nil, false
			}
			if !source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
				continue
			}
			for port := 0; port < contract.ValueFormalCount(operation); port++ {
				if !appendPort(OriginInput, originPort{application: application, operation: operation, port: uint32(port)}, roots) {
					return nil, nil, false
				}
			}
			for port := 0; port < contract.OutcomeCount(operation); port++ {
				if !appendPort(OriginOutcome, originPort{application: application, operation: operation, port: uint32(port)}, roots) {
					return nil, nil, false
				}
			}
			for port := 0; port < contract.CallbackCount(operation); port++ {
				callback, found := contract.CallbackAt(operation, port)
				if !found || !appendPort(OriginCallbackOccurrence, originPort{application: application, operation: operation, port: uint32(callback)}, roots) {
					return nil, nil, false
				}
			}
			for port := 0; port < contract.SuspensionCount(operation); port++ {
				if !appendPort(OriginSuspensionOccurrence, originPort{application: application, operation: operation, port: uint32(port)}, roots) {
					return nil, nil, false
				}
			}
			for port := 0; port < contract.ResumeCount(operation); port++ {
				resume, found := contract.ResumeIDAt(operation, port)
				if !found || !appendPort(OriginResumeOccurrence, originPort{application: application, operation: operation, port: uint32(resume)}, roots) {
					return nil, nil, false
				}
			}
			for transferIndex := 0; transferIndex < contract.TransferCount(operation); transferIndex++ {
				transfer, found := contract.TransferIDAt(operation, transferIndex)
				if !found {
					return nil, nil, false
				}
				for result := 0; result < contract.TransferDeclarationOutcomeCount(transfer); result++ {
					outcome, _, found := contract.TransferDeclarationOutcomeAt(transfer, result)
					if !found || !appendPort(OriginTransferArmOccurrence, originPort{application: application, operation: operation, transfer: transfer, port: outcome}, roots) {
						return nil, nil, false
					}
				}
			}
		}
	}
	return rows, index, true
}

// appendAllocationOrigins consumes Heap's one canonical allocation-key range.
// Fresh keys retain their exact creation occurrence inside Heap; this domain
// derives only the caller AnalysisRoot from that provenance and never creates
// a second root handle or a fresh-product range.
func appendAllocationOrigins(source *link.Link, heapSchema heap.Schema, rows *[]originRow, index map[keyspace.ContentID]uint32) bool {
	if source == nil || !heapSchema.Valid() || rows == nil || index == nil {
		return false
	}
	for keyIndex := 0; keyIndex < heapSchema.KeyCount(); keyIndex++ {
		key, ok := heapSchema.KeyAt(keyIndex)
		if !ok || key.Kind() != heap.RootAllocation {
			continue
		}
		keyID, ok := key.ContentID()
		if !ok || !keyID.Available() || uint64(len(*rows)) >= uint64(^uint32(0)) {
			return false
		}
		var roots []linkmodule.AnalysisRoot
		if shard, _, _, programRoot := key.ProgramAllocation(); programRoot {
			roots, ok = rootsForShard(source, shard)
		} else {
			application, _, _, _, _, _, fresh := key.FreshResult()
			if !fresh {
				return false
			}
			roots, ok = applicationRoots(source, application)
		}
		if !ok || len(roots) == 0 {
			return false
		}
		id := ownershipAllocationOriginID(source.ContentID(), keyID)
		if !id.Available() || index[id] != 0 {
			return false
		}
		*rows = append(*rows, originRow{kind: OriginAllocationRoot, ordinal: uint32(len(*rows) + 1), id: id, roots: roots, allocation: key})
		index[id] = uint32(len(*rows))
	}
	return true
}

// appendProgramOrigins enumerates the three ownership-relevant executable
// Program relations directly.  The order deliberately matches the former
// per-kind Link projection: shard order, then Program family order, then a
// capture position.  Origin IDs therefore retain their existing canonical
// ordinal without retaining a second Link-owned semantic plane.
func appendProgramOrigins(source *link.Link, kind OriginKind, rows *[]originRow, index map[keyspace.ContentID]uint32) bool {
	if source == nil || rows == nil || index == nil {
		return false
	}
	project := source.Project()
	if project == nil {
		return false
	}
	mounts := project.Mounts()
	appendOne := func(ordinal uint32, coordinate programOrigin, roots []linkmodule.AnalysisRoot) bool {
		if ordinal == 0 || coordinate.term == 0 || len(roots) == 0 || uint64(len(*rows)) >= uint64(^uint32(0)) {
			return false
		}
		id := ownershipOriginID(source.ContentID(), kind, ordinal)
		if !id.Available() || index[id] != 0 {
			return false
		}
		*rows = append(*rows, originRow{kind: kind, ordinal: ordinal, id: id, roots: roots, program: coordinate})
		index[id] = uint32(len(*rows))
		return true
	}
	ordinal := uint32(0)
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, ok := mounts.At(shardIndex)
		if !ok {
			return false
		}
		p, ok := mounts.Program(shard)
		if !ok || p == nil {
			return false
		}
		roots, ok := rootsForShard(source, shard)
		if !ok {
			return false
		}
		flow := p.Flow()
		authored := flow.Authored()
		functions := authored.Functions()
		writes := authored.Storage().Writes()
		returns := authored.Control().Returns()
		executable := flow.Executable()
		exactLenses := authored.Access().Exact()
		dynamicLenses := authored.Access().Dynamic()
		switch kind {
		case OriginCaptureRetention:
			for functionIndex := 0; functionIndex < functions.Count(); functionIndex++ {
				function, ok := functions.At(functionIndex)
				if !ok || function == 0 || !executable.Contains(function) {
					continue
				}
				width, ok := functions.CaptureCount(function)
				if !ok || width < 0 || uint64(ordinal)+uint64(width) > uint64(^uint32(0)) {
					return false
				}
				for captureIndex := 0; captureIndex < width; captureIndex++ {
					inner, outer, ok := functions.CaptureAt(function, captureIndex)
					if !ok || inner == 0 || outer == 0 {
						return false
					}
					ordinal++
					if !appendOne(ordinal, programOrigin{shard: shard, term: function, index: uint32(captureIndex)}, roots) {
						return false
					}
				}
			}
		case OriginStoreCommit:
			for writeIndex := 0; writeIndex < writes.Count(); writeIndex++ {
				write, ok := writes.At(writeIndex)
				if !ok || write == 0 || !executable.Contains(write) {
					continue
				}
				_, target, ok := writes.Get(write)
				if !ok || target == 0 {
					return false
				}
				_, _, _, _, exactLens := exactLenses.Get(target)
				_, _, _, dynamicLens := dynamicLenses.Get(target)
				if !exactLens && !dynamicLens {
					continue
				}
				if ordinal == ^uint32(0) {
					return false
				}
				ordinal++
				if !appendOne(ordinal, programOrigin{shard: shard, term: write}, roots) {
					return false
				}
			}
		case OriginReturnBoundary:
			for returnIndex := 0; returnIndex < returns.Count(); returnIndex++ {
				returned, ok := returns.At(returnIndex)
				if !ok || returned == 0 || !executable.Contains(returned) {
					continue
				}
				if _, _, ok := returns.Get(returned); !ok {
					return false
				}
				if ordinal == ^uint32(0) {
					return false
				}
				ordinal++
				if !appendOne(ordinal, programOrigin{shard: shard, term: returned}, roots) {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

func rootsForShard(source *link.Link, shard linkproject.Shard) ([]linkmodule.AnalysisRoot, bool) {
	if source == nil {
		return nil, false
	}
	roots := make([]linkmodule.AnalysisRoot, 0, source.Module().Roots().ForShardCount(shard))
	for index := 0; index < source.Module().Roots().ForShardCount(shard); index++ {
		root, found := source.Module().Roots().ForShardAt(shard, index)
		if !found {
			return nil, false
		}
		roots = append(roots, root)
	}
	return roots, len(roots) != 0
}

func applicationRoots(source *link.Link, application linkproject.Application) ([]linkmodule.AnalysisRoot, bool) {
	if source == nil || source.Project() == nil {
		return nil, false
	}
	shard, term, ok := source.Project().Applications().Call(application)
	if !ok {
		return nil, false
	}
	p, ok := source.Project().Mounts().Program(shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(term) {
		return nil, false
	}
	roots := make([]linkmodule.AnalysisRoot, 0, source.Module().Roots().ForShardCount(shard))
	for index := 0; index < source.Module().Roots().ForShardCount(shard); index++ {
		root, found := source.Module().Roots().ForShardAt(shard, index)
		if !found {
			return nil, false
		}
		roots = append(roots, root)
	}
	return roots, len(roots) != 0
}

func ownershipOriginID(linkID keyspace.ContentID, kind OriginKind, ordinal uint32) keyspace.ContentID {
	var image [32 + 3*8]byte
	copy(image[:32], linkID[:])
	binary.BigEndian.PutUint64(image[32:40], 0x6f776e65722d6f72) // "owner-or"
	binary.BigEndian.PutUint64(image[40:48], uint64(kind))
	binary.BigEndian.PutUint64(image[48:56], uint64(ordinal))
	return keyspace.ContentID(sha256.Sum256(image[:]))
}

func ownershipAllocationOriginID(linkID, keyID keyspace.ContentID) keyspace.ContentID {
	var image [32 + 32 + 8]byte
	copy(image[:32], linkID[:])
	copy(image[32:64], keyID[:])
	binary.BigEndian.PutUint64(image[64:72], 0x6f776e65722d616c) // "owner-al"
	return keyspace.ContentID(sha256.Sum256(image[:]))
}

func originRoots(source *link.Link, kind OriginKind, ordinal uint32) ([]linkmodule.AnalysisRoot, bool) {
	if ordinal == 0 {
		return nil, false
	}
	one := func(root linkmodule.AnalysisRoot, ok bool) ([]linkmodule.AnalysisRoot, bool) {
		if !ok {
			return nil, false
		}
		return []linkmodule.AnalysisRoot{root}, true
	}
	byShard := func(shard linkproject.Shard, ok bool) ([]linkmodule.AnalysisRoot, bool) {
		if !ok {
			return nil, false
		}
		return rootsForShard(source, shard)
	}
	switch kind {
	case OriginGlobalBinding:
		value, ok := source.Host().Globals().At(int(ordinal - 1))
		if !ok {
			return nil, false
		}
		root, _, _, _, _, _, ok := source.Host().Globals().Mapping(value)
		return one(root, ok)
	case OriginApplication:
		value, ok := source.Project().Applications().Calls().At(int(ordinal - 1))
		if !ok {
			return nil, false
		}
		return applicationRoots(source, value)
	case OriginModuleCacheEntry:
		value, ok := source.Module().Cache().EntryAt(int(ordinal - 1))
		if !ok {
			return nil, false
		}
		_, from, to, ok := source.Module().Cache().EntryMapping(value)
		return distinctRoots(from, to, ok)
	case OriginModuleCoordinate:
		value, ok := source.Module().Coordinates().At(int(ordinal - 1))
		if !ok {
			return nil, false
		}
		roots := make([]linkmodule.AnalysisRoot, 0)
		for i := 0; ; i++ {
			root, found := source.Module().Roots().At(i)
			if !found {
				break
			}
			coordinate, found := source.Module().Coordinates().ForRoot(root)
			if found && coordinate == value {
				roots = append(roots, root)
			}
		}
		for index := 0; index < source.Module().Cache().EntryCount(); index++ {
			entry, found := source.Module().Cache().EntryAt(index)
			if !found {
				return nil, false
			}
			_, from, to, found := source.Module().Cache().EntryMapping(entry)
			if !found {
				return nil, false
			}
			_, fromActor, fromInstance, found := source.Module().Roots().Mapping(from)
			if !found {
				return nil, false
			}
			toShard, toActor, _, found := source.Module().Roots().Mapping(to)
			if !found || toActor != fromActor {
				return nil, false
			}
			representative, found := source.Module().Cache().Representative(fromInstance)
			if !found {
				return nil, false
			}
			actor, shard, coordinateRepresentative, found := source.Module().Coordinates().Mapping(value)
			if !found {
				return nil, false
			}
			if actor == fromActor && shard == toShard && coordinateRepresentative == representative {
				roots = appendUniqueRoots(roots, from, to)
			}
		}
		return roots, len(roots) != 0
	case OriginModuleInitGeneration:
		value, ok := source.Module().Generations().At(int(ordinal - 1))
		if !ok {
			return nil, false
		}
		entry, _, _, _, ok := source.Module().Generations().Entry(value)
		if !ok {
			return nil, false
		}
		_, from, to, ok := source.Module().Cache().EntryMapping(entry)
		return distinctRoots(from, to, ok)
	case OriginBootRoot:
		value, ok := source.Host().BootRoots().At(int(ordinal - 1))
		if !ok {
			return nil, false
		}
		actor, _, ok := source.Host().BootRoots().Mapping(value)
		if !ok {
			return nil, false
		}
		roots := make([]linkmodule.AnalysisRoot, 0)
		for i := 0; ; i++ {
			root, found := source.Module().Roots().At(i)
			if !found {
				break
			}
			_, rootActor, _, found := source.Module().Roots().Mapping(root)
			if found && rootActor == actor {
				roots = append(roots, root)
			}
		}
		return roots, len(roots) != 0
	case OriginHostExposure:
		shard, _, _, _, _, ok := source.Host().Exposures().At(int(ordinal - 1))
		return byShard(shard, ok)
	default:
		return nil, false
	}
}

func distinctRoots(left, right linkmodule.AnalysisRoot, ok bool) ([]linkmodule.AnalysisRoot, bool) {
	if !ok {
		return nil, false
	}
	if left == right {
		return []linkmodule.AnalysisRoot{left}, true
	}
	return []linkmodule.AnalysisRoot{left, right}, true
}

func appendUniqueRoots(roots []linkmodule.AnalysisRoot, values ...linkmodule.AnalysisRoot) []linkmodule.AnalysisRoot {
	for _, value := range values {
		present := false
		for _, existing := range roots {
			if existing == value {
				present = true
				break
			}
		}
		if !present {
			roots = append(roots, value)
		}
	}
	return roots
}

// rolesFor is Ownership's complete source-duty applicability law.
func rolesFor(schema Schema, source *link.Link, origin Origin) ([2]Role, int, bool) {
	var roles [2]Role
	one := func(role Role) ([2]Role, int, bool) { roles[0] = role; return roles, 1, true }
	two := func(first, second Role) ([2]Role, int, bool) {
		roles[0], roles[1] = first, second
		return roles, 2, true
	}
	none := func() ([2]Role, int, bool) { return roles, 0, true }
	if source == nil || !origin.Valid() {
		return roles, 0, false
	}
	switch origin.Kind() {
	case OriginAllocationRoot:
		return two(Owner, Lifetime)
	case OriginCaptureRetention, OriginStoreCommit, OriginGlobalBinding, OriginModuleCacheEntry:
		return two(Share, Lifetime)
	case OriginReturnBoundary, OriginOutcome, OriginResumeOccurrence:
		return two(Move, Lifetime)
	case OriginInput:
		return one(Borrow)
	case OriginApplication, OriginModuleCoordinate, OriginHostExposure:
		return none()
	case OriginModuleInitGeneration, OriginSuspensionOccurrence:
		return one(Lifetime)
	case OriginBootRoot:
		return two(Owner, Lifetime)
	case OriginCallbackOccurrence:
		row, ok := schema.origin(origin)
		contract, targetOK := source.Boundary().Target()
		if !ok || !targetOK || contract == nil {
			return roles, 0, false
		}
		lifecycle, ok := contract.CallbackLifecycle(target.CallbackID(row.port.port))
		if !ok {
			return roles, 0, false
		}
		switch lifecycle {
		case target.CallbackSyncOptionalOnce, target.CallbackSyncRequiredOnce, target.CallbackSyncOptionalMany, target.CallbackSyncRequiredMany:
			return one(Borrow)
		case target.CallbackRetainedOptionalOnce, target.CallbackRetainedRequiredOnce, target.CallbackRetainedOptionalMany, target.CallbackRetainedRequiredMany:
			return two(Share, Lifetime)
		default:
			return roles, 0, false
		}
	case OriginTransferArmOccurrence:
		row, ok := schema.origin(origin)
		contract, targetOK := source.Boundary().Target()
		if !ok || !targetOK || contract == nil {
			return roles, 0, false
		}
		for outcomeIndex := 0; outcomeIndex < contract.TransferDeclarationOutcomeCount(row.port.transfer); outcomeIndex++ {
			outcome, disposition, found := contract.TransferDeclarationOutcomeAt(row.port.transfer, outcomeIndex)
			if !found {
				return roles, 0, false
			}
			if outcome != row.port.port {
				continue
			}
			if disposition == target.TransferMayDeliver {
				return one(Send)
			}
			if disposition == target.TransferMayReject {
				return none()
			}
		}
		return roles, 0, false
	}
	return roles, 0, false
}
