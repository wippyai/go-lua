package heap

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/program"
	programflow "github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

const schemaFormat uint64 = 0x686561702d7638 // "heap-v8"

// Schema is the one immutable Heap family authority derived from one sealed
// Link. It is deliberately not a per-root schema: all Values share this fence
// and the Factor's sparse default is therefore constant and lawful.
type Schema struct{ owner *schema }

type schema struct {
	source *link.Link
	linkID keyspace.ContentID
	id     keyspace.ContentID

	roots            []rootRow // physical Program roots followed by Boot roots
	programRootCount uint32
	bootIndex        map[linkhost.BootRoot]uint32
	freshTemplates   []freshTemplate
	freshByKey       map[freshTemplateKey]uint32
	freshSets        []freshTemplateSet
	freshSetValues   []freshSetValue
	freshApps        []linkproject.Application
	freshAppSets     []uint32
	freshOffsets     []uint64
	freshCount       uint64
	// atomicKeys is the exact finite universe of selectors that may become
	// Partition exceptions. atomMaskCounts compresses that universe by its
	// owner-derived possible-kind mask for the fixed-coordinate rank law.
	atomicKeys     []keyAtom
	atomMaskCounts [1 << runtimekind.Count]uint64

	slots        []slotRow
	slotSupport  []slotSupport
	exactSlots   map[linkproject.Key]uint32
	dynamicSlots map[sourceCoordinate]uint32
	unknownSlot  uint32

	payloads       []payloadRow
	payloadSupport []payloadSupport
	payloadIndex   map[payloadRow]uint32
	localSlots     map[rootSlot]struct{}
	bootEntries    map[rootSlot]bootEntryRow
	bootEntryOrder []rootSlot
	bootInitials   map[rootPayload]bootEntryRow

	metatableRoutes     []metatableRouteRow
	bootMetatableRoutes map[linkhost.BootMetatableAttachment]uint32
	// fields and indexAccesses are Heap's direct sealed projections of Flow's
	// AccessGeometry. No authored Flow owner or generic Lens/access graph is
	// retained after sealing.
	fields []fieldRow

	indexAccesses []indexAccessRow

	// These inverse maps are derived only after every semantic root and
	// access row has sealed.  They are occurrence indexes, not a second
	// identity plane: each value is the already-issued dense Key/IndexAccess
	// selector for the exact owner-fenced Program tuple.
	programAllocationOrdinals map[programAllocationOccurrence]uint32
	indexAccessOrdinals       map[indexAccessOccurrence]uint32

	// These are sealed finite denominators for Heap's canonical Mu carrier;
	// they are rank witnesses, never work budgets or cardinality caps.
	referenceCount       uint64
	presentPotential     uint64
	fixedObjectRankBound uint64
	maxObjectRankSum     uint64
	bottom               Value
	top                  Value
}

type rootRow struct {
	kind          RootKind
	allocation    allocationSource
	fresh         freshSource
	boot          linkhost.BootRoot
	bootImmutable bool
	fieldStart    uint32
	fieldCount    uint32
}

// AllocationKind is the closed Program allocation family.  It belongs to
// Heap because a Heap Key, not Link, is the sole allocation coordinate.
type AllocationKind uint8

const (
	AllocationInvalid AllocationKind = iota
	AllocationTable
	AllocationClosure
)

type allocationSource struct {
	shard linkproject.Shard
	term  keyspace.Term
	kind  AllocationKind
}

// freshSource is a virtual exact fresh creation coordinate.
type freshSource struct {
	application linkproject.Application
	outcome     uint32
	result      uint32
	ordinal     uint32
	kinds       runtimekind.Set
}

type freshTemplateKey struct{ outcome, result, ordinal uint32 }
type freshTemplate struct {
	freshTemplateKey
}
type freshTemplateSet struct{ start, end uint32 }
type freshSetValue struct {
	template uint32
	kinds    runtimekind.Set
}

type sourceCoordinate struct {
	shard linkproject.Shard
	term  keyspace.Term
}

type fieldSource struct {
	root uint32
	term keyspace.Term
}

type fieldRow struct {
	fieldSource
	kind     flowkind.FieldKind
	keyTerm  keyspace.Term
	slot     uint32
	payload  uint32
	openTail uint32
}

type payloadKind uint8

const (
	payloadInvalid payloadKind = iota
	payloadValues
	payloadInitial
)

type slotRow struct {
	kind    SlotKind
	exact   linkproject.Key
	dynamic sourceCoordinate
	field   uint32
}

type payloadRow struct {
	kind    payloadKind
	shard   linkproject.Shard
	values  keyspace.Term
	index   uint32
	initial target.InitialValue
}

type bootEntryRow struct {
	raw        RawPresence
	payload    uint32
	initial    target.InitialValue
	valueChild uint32
	mutability target.InitialMutability
}

// metatableRouteRow is the sealed bootstrap projection of an existing Link
// primitive attachment. Mutable table attachment remains ordinary Heap state
// and never mutates this cold ledger.
type metatableRouteRow struct {
	primitive target.InitialValueKind
	metatable uint32
	role      materialization.Role
}

// slotSupport is the cold root-incidence law for one symbolic partition.
// Exact and unknown partitions are semantic key classes and therefore apply
// to every root. A dynamic access is likewise global because Value/identity
// selects its base at runtime. Constructor-only dynamic and open-tail
// partitions retain only their exact creation roots.
type slotSupport struct {
	global bool
}

type rootSlot struct {
	root uint32
	slot uint32
}

type rootPayload struct {
	root    uint32
	payload uint32
}

type indexAccessRow struct {
	shard    linkproject.Shard
	read     keyspace.Term
	write    keyspace.Term
	base     keyspace.Term
	keyTerm  keyspace.Term
	values   keyspace.Term
	position int
	lens     keyspace.Term
	slot     uint32
	payload  uint32
}

// programAllocationOccurrence is the one exact inverse key for a Program
// aggregate. Shard carries Project's owner fence; term carries the authored
// occurrence. The Program pointer is checked at query time against that Shard
// rather than copied into this map, so the map cannot become a second Program
// identity table.
type programAllocationOccurrence struct {
	shard linkproject.Shard
	term  keyspace.Term
}

// indexAccessOccurrence is the one exact inverse key for an authored index
// access. Read and Write terms are disjoint families, so the occurrence term
// alone selects the typed sealed row after the mounted Program validates it.
type indexAccessOccurrence struct {
	shard linkproject.Shard
	term  keyspace.Term
}

// Field is one schema-issued Program TableField route.  It has no Link
// counterpart and no independently constructible root/term pair.
type Field struct {
	owner *schema
	index uint32
}

// IndexAccess is one Heap-scoped direct typed AccessGeometry candidate row.
type IndexAccess struct {
	owner *schema
	index uint32
}

// IndexGeometry is Heap's direct immutable copy of one sealed candidate row.
// Exactly one of ReadTerm and WriteTerm is nonzero. Read rows have Position
// -1 and no Values; write rows retain their authored Values and position.
type IndexGeometry struct {
	Shard     linkproject.Shard
	ReadTerm  keyspace.Term
	WriteTerm keyspace.Term
	Base      keyspace.Term
	KeyTerm   keyspace.Term
	Values    keyspace.Term
	Position  int
	Lens      keyspace.Term
}

// payloadSupport prevents a source payload from being paired with an
// unrelated key or allocation root. Assignment payloads are factorized by
// slot and remain applicable to any selected base root; constructor payloads
// retain their exact root/slot incidence.
type payloadSupport struct {
	globalSlots map[uint32]struct{}
	local       map[rootSlot]struct{}
	// roots is the exact source-root projection used only for canonical
	// Unknown coarsening.  It preserves the original payload universe rather
	// than admitting payloads merely because their target slot is common.
	roots map[uint32]struct{}
}

// Seal derives the complete finite structural Heap support from Link. It
// creates no source identities: roots, fields, lenses, candidate rows, and
// Values-pack selections are all existing Link coordinates.
func Seal(source *link.Link) (Schema, bool) {
	if source == nil || !source.ContentID().Available() {
		return Schema{}, false
	}
	owner := &schema{
		source:              source,
		linkID:              source.ContentID(),
		bootIndex:           make(map[linkhost.BootRoot]uint32),
		exactSlots:          make(map[linkproject.Key]uint32),
		dynamicSlots:        make(map[sourceCoordinate]uint32),
		freshByKey:          make(map[freshTemplateKey]uint32),
		payloadIndex:        make(map[payloadRow]uint32),
		localSlots:          make(map[rootSlot]struct{}),
		bootEntries:         make(map[rootSlot]bootEntryRow),
		bootInitials:        make(map[rootPayload]bootEntryRow),
		bootMetatableRoutes: make(map[linkhost.BootMetatableAttachment]uint32),
	}
	if !owner.addProgramAllocations() {
		return Schema{}, false
	}
	owner.programRootCount = uint32(len(owner.roots))
	if !owner.addTargetFreshResults() {
		return Schema{}, false
	}
	if owner.rootCount() > uint64(^uint32(0)) {
		return Schema{}, false
	}
	for index := 0; index < source.Host().BootRoots().Count(); index++ {
		root, ok := source.Host().BootRoots().At(index)
		if !ok || !owner.addBootRoot(root) {
			return Schema{}, false
		}
	}
	if owner.rootCount() > uint64(^uint32(0)) {
		return Schema{}, false
	}
	if !owner.addBootEntries() || !owner.addBootMetatableRoutes() {
		return Schema{}, false
	}
	mounts := source.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		p, programOK := mounts.Program(shard)
		if !ok || !programOK || p == nil || !owner.addIndexAccesses(shard, p) {
			return Schema{}, false
		}
	}
	owner.unknownSlot = owner.ensureUnknownSlot()
	if owner.unknownSlot == 0 || !owner.finish() {
		return Schema{}, false
	}
	return Schema{owner: owner}, true
}

// addProgramAllocations seals Program-owned aggregate roots directly.  Link
// contributes only the shard-to-Program topology; it does not manufacture an
// allocation handle or cache a second root range.
func (owner *schema) addProgramAllocations() bool {
	if owner == nil || owner.source == nil {
		return false
	}
	mounts := owner.source.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		p, programOK := mounts.Program(shard)
		if !shardOK || !programOK || p == nil {
			return false
		}
		flow := p.Flow()
		geometry := flow.AccessGeometry()
		if !geometry.Available() {
			return false
		}
		authored := flow.Authored()
		tables := authored.Tables()
		functions := authored.Functions()
		executable := flow.Executable()
		for tableIndex := 0; tableIndex < tables.Count(); tableIndex++ {
			term, ok := tables.At(tableIndex)
			if !ok {
				return false
			}
			if !executable.Contains(term) {
				continue
			}
			if !owner.addProgramAllocation(shard, term, AllocationTable, geometry) {
				return false
			}
		}
		for functionIndex := 0; functionIndex < functions.Count(); functionIndex++ {
			term, ok := functions.At(functionIndex)
			if !ok {
				return false
			}
			if !executable.Contains(term) {
				continue
			}
			if !owner.addProgramAllocation(shard, term, AllocationClosure, geometry) {
				return false
			}
		}
	}
	return true
}

func (owner *schema) addProgramAllocation(shard linkproject.Shard, term keyspace.Term, kind AllocationKind, geometry programflow.AccessGeometry) bool {
	if owner == nil || owner.source == nil || term == 0 || uint64(len(owner.roots)) >= uint64(^uint32(0)) {
		return false
	}
	p, ok := owner.source.Project().Mounts().Program(shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(term) {
		return false
	}
	switch kind {
	case AllocationTable:
		if _, ok := p.Flow().Authored().Tables().Get(term); !ok {
			return false
		}
	case AllocationClosure:
		if _, _, _, ok := p.Flow().Authored().Functions().Get(term); !ok {
			return false
		}
	default:
		return false
	}
	owner.roots = append(owner.roots, rootRow{kind: RootAllocation, allocation: allocationSource{shard: shard, term: term, kind: kind}})
	root := uint32(len(owner.roots))
	if kind != AllocationTable {
		return true
	}
	tables := p.Flow().Authored().Tables()
	fieldCount, fieldsOK := tables.FieldCount(term)
	if !fieldsOK {
		return false
	}
	for fieldIndex := 0; fieldIndex < fieldCount; fieldIndex++ {
		field, fieldOK := tables.FieldAt(term, fieldIndex)
		if !fieldOK || !owner.addField(root, shard, field, geometry) {
			return false
		}
	}
	return true
}

// addTargetFreshResults factorizes fresh roots.  It stores global Target
// templates and one interned eligible-template set per Call Application; a
// Key decodes its `(Application,outcome,result,ordinal)` source on demand.
// No Application×template rows are retained.
func (owner *schema) addTargetFreshResults() bool {
	if owner == nil || owner.source == nil {
		return false
	}
	contract, ok := owner.source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	for operationIndex := 0; operationIndex < contract.OperationCount(); operationIndex++ {
		operation, operationOK := contract.OperationAt(operationIndex)
		if !operationOK {
			return false
		}
		for outcome := 0; outcome < contract.OutcomeCount(operation); outcome++ {
			for freshIndex := 0; freshIndex < contract.FreshResultCount(operation, outcome); freshIndex++ {
				result, ordinal, kind, freshOK := contract.FreshResultAt(operation, outcome, freshIndex)
				key := freshTemplateKey{outcome: uint32(outcome), result: result, ordinal: ordinal}
				if !freshOK {
					return false
				}
				_, heapKind := freshRootKinds(kind)
				if !heapKind {
					continue
				}
				if prior := owner.freshByKey[key]; prior != 0 {
					continue
				}
				owner.freshTemplates = append(owner.freshTemplates, freshTemplate{freshTemplateKey: key})
				owner.freshByKey[key] = uint32(len(owner.freshTemplates))
			}
		}
	}
	if len(owner.freshTemplates) == 0 {
		return true
	}
	for applicationIndex := 0; applicationIndex < owner.source.Project().Applications().Calls().Count(); applicationIndex++ {
		application, applicationOK := owner.source.Project().Applications().Calls().At(applicationIndex)
		if !applicationOK {
			return false
		}
		selected := make(map[uint32]runtimekind.Set)
		for operationIndex := 0; operationIndex < contract.OperationCount(); operationIndex++ {
			operation, operationOK := contract.OperationAt(operationIndex)
			if !operationOK {
				return false
			}
			if !owner.source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
				continue
			}
			for outcome := 0; outcome < contract.OutcomeCount(operation); outcome++ {
				for freshIndex := 0; freshIndex < contract.FreshResultCount(operation, outcome); freshIndex++ {
					result, ordinal, kind, freshOK := contract.FreshResultAt(operation, outcome, freshIndex)
					key := freshTemplateKey{outcome: uint32(outcome), result: result, ordinal: ordinal}
					if !freshOK {
						return false
					}
					mask, heapKind := freshRootKinds(kind)
					if !heapKind {
						continue
					}
					template := owner.freshByKey[key]
					if template == 0 {
						return false
					}
					selected[template] |= mask
				}
			}
		}
		if len(selected) == 0 {
			continue
		}
		values := make([]freshSetValue, 0, len(selected))
		for template, kinds := range selected {
			values = append(values, freshSetValue{template: template, kinds: kinds})
		}
		sort.Slice(values, func(i, j int) bool { return values[i].template < values[j].template })
		set := uint32(0)
		for index, candidate := range owner.freshSets {
			if int(candidate.end-candidate.start) != len(values) {
				continue
			}
			equal := true
			for offset, value := range values {
				stored := owner.freshSetValues[candidate.start+uint32(offset)]
				if stored != value {
					equal = false
					break
				}
			}
			if equal {
				set = uint32(index + 1)
				break
			}
		}
		if set == 0 {
			start := uint32(len(owner.freshSetValues))
			owner.freshSetValues = append(owner.freshSetValues, values...)
			owner.freshSets = append(owner.freshSets, freshTemplateSet{start: start, end: uint32(len(owner.freshSetValues))})
			set = uint32(len(owner.freshSets))
		}
		owner.freshApps = append(owner.freshApps, application)
		owner.freshAppSets = append(owner.freshAppSets, set)
		if owner.freshCount > ^uint64(0)-uint64(len(values)) {
			return false
		}
		owner.freshCount += uint64(len(values))
		owner.freshOffsets = append(owner.freshOffsets, owner.freshCount)
	}
	return true
}

func (owner *schema) rootCount() uint64 { return uint64(len(owner.roots)) + owner.freshCount }

// rootAt decodes the virtual fresh interval without allocating or retaining
// an Application×template row. Physical rows are Program allocations followed
// by Boot roots; Key slots place fresh roots between them.
func (owner *schema) rootAt(slot uint32) (rootRow, bool) {
	if owner == nil || slot == 0 || uint64(slot) > owner.rootCount() {
		return rootRow{}, false
	}
	if slot <= owner.programRootCount {
		return owner.roots[slot-1], true
	}
	freshStart := uint64(owner.programRootCount) + 1
	if uint64(slot) >= freshStart && uint64(slot) < freshStart+owner.freshCount {
		ordinal := uint64(slot) - freshStart
		appIndex := sort.Search(len(owner.freshOffsets), func(index int) bool { return owner.freshOffsets[index] > ordinal })
		if appIndex >= len(owner.freshApps) || appIndex >= len(owner.freshAppSets) {
			return rootRow{}, false
		}
		start := uint64(0)
		if appIndex > 0 {
			start = owner.freshOffsets[appIndex-1]
		}
		setID := owner.freshAppSets[appIndex]
		if setID == 0 || int(setID) > len(owner.freshSets) {
			return rootRow{}, false
		}
		set := owner.freshSets[setID-1]
		local := ordinal - start
		if local >= uint64(set.end-set.start) {
			return rootRow{}, false
		}
		setValue := owner.freshSetValues[set.start+uint32(local)]
		templateID := setValue.template
		if templateID == 0 || int(templateID) > len(owner.freshTemplates) {
			return rootRow{}, false
		}
		template := owner.freshTemplates[templateID-1]
		return rootRow{kind: RootAllocation, fresh: freshSource{application: owner.freshApps[appIndex], outcome: template.outcome, result: template.result, ordinal: template.ordinal, kinds: setValue.kinds}}, true
	}
	bootIndex := uint64(slot) - owner.freshCount
	if bootIndex == 0 || bootIndex > uint64(len(owner.roots)) {
		return rootRow{}, false
	}
	return owner.roots[bootIndex-1], true
}

func (owner *schema) addBootRoot(root linkhost.BootRoot) bool {
	if owner == nil || owner.source == nil || owner.bootIndex[root] != 0 || uint64(len(owner.roots)) >= uint64(^uint32(0)) {
		return false
	}
	_, initial, ok := owner.source.Host().BootRoots().Mapping(root)
	if !ok || initial == 0 {
		return false
	}
	contract, contractOK := owner.source.Boundary().Target()
	if !contractOK || contract == nil {
		return false
	}
	shape, shapeOK := contract.InitialRootBootShape(initial)
	immutable, immutableOK := contract.BootShapeImmutable(shape)
	if !shapeOK || !immutableOK {
		return false
	}
	owner.roots = append(owner.roots, rootRow{kind: RootBoot, boot: root, bootImmutable: immutable})
	virtual := uint64(len(owner.roots)) + owner.freshCount
	if virtual > uint64(^uint32(0)) {
		return false
	}
	owner.bootIndex[root] = uint32(virtual)
	return true
}

// addBootEntries projects Target's immutable per-initial-root ledger through
// Link's actor-local BootRoot image and its one shared exact-key universe.
// This is a cold schema projection only: it neither creates a runtime value
// nor seeds recurrent Heap state.
func (owner *schema) addBootEntries() bool {
	if owner == nil || owner.source == nil {
		return false
	}
	contract, ok := owner.source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	type entryRow struct {
		key        linkproject.Key
		value      target.InitialValue
		kind       target.InitialValueKind
		mutability target.InitialMutability
	}
	entries := make(map[target.InitialRoot][]entryRow)
	for index := 0; index < contract.InitialEntryCount(); index++ {
		entryRoot, exact, initialValue, mutability, entryOK := contract.InitialEntryAt(index)
		if !entryOK || initialValue == 0 || (mutability != target.InitialMutable && mutability != target.InitialFrozen) {
			return false
		}
		kind, valid := contract.InitialValueKind(initialValue)
		key, keyOK := owner.source.Project().Keys().ForTarget(contract, exact)
		if !valid || kind == target.InitialValueInvalid || !keyOK {
			return false
		}
		entries[entryRoot] = append(entries[entryRoot], entryRow{key: key, value: initialValue, kind: kind, mutability: mutability})
	}
	for _, root := range owner.roots {
		if root.kind != RootBoot {
			continue
		}
		virtualRoot := owner.bootIndex[root.boot]
		if virtualRoot == 0 {
			return false
		}
		actor, initial, mapped := owner.source.Host().BootRoots().Mapping(root.boot)
		if !mapped {
			return false
		}
		for _, entry := range entries[initial] {
			slot := owner.addExactSlot(entry.key)
			if slot == 0 || !owner.addLocalSlot(slot, virtualRoot) {
				return false
			}
			raw, rawOK := initialValueRawPresence(entry.kind)
			if !rawOK {
				return false
			}
			row := bootEntryRow{raw: raw, mutability: entry.mutability}
			// Target preserves Nil and Absent as distinct contract values.  Both
			// project to raw absence in Heap: Lua assignment of nil deletes the
			// slot, so Heap must never materialize a present nil payload.
			if raw == RawPresent {
				payload := owner.addPayload(payloadRow{kind: payloadInitial, initial: entry.value})
				if payload == 0 || !owner.addLocalPayload(payload, virtualRoot, slot) {
					return false
				}
				row.raw, row.payload, row.initial = RawPresent, payload, entry.value
				if entry.kind == target.InitialValueRoot {
					initialRoot, rootValue := contract.InitialValueRoot(entry.value)
					child, found := owner.source.Host().BootRoots().For(actor, initialRoot)
					childID := owner.bootIndex[child]
					if !rootValue || !found || childID == 0 {
						return false
					}
					row.valueChild = childID
				}
			}
			rootSlot := rootSlot{root: virtualRoot, slot: slot}
			if owner.bootEntries[rootSlot].mutability != target.InitialMutabilityInvalid {
				return false
			}
			owner.bootEntries[rootSlot] = row
			if row.raw == RawPresent {
				index := rootPayload{root: rootSlot.root, payload: row.payload}
				if previous, exists := owner.bootInitials[index]; exists && (previous.initial != row.initial || previous.valueChild != row.valueChild) {
					return false
				}
				owner.bootInitials[index] = row
			}
		}
	}
	return true
}

// initialValueRawPresence is the one Target-to-Heap boundary law. Target
// retains the Nil/Absent distinction for contract consumers, but neither is a
// raw stored table value in Lua. An unclassified Target value cannot silently
// become a Heap fact: sealing rejects it until its raw-storage law is named.
func initialValueRawPresence(kind target.InitialValueKind) (RawPresence, bool) {
	switch kind {
	case target.InitialValueNil, target.InitialValueAbsent:
		return RawAbsent, true
	case target.InitialValueBoolean,
		target.InitialValueInteger,
		target.InitialValueFloat,
		target.InitialValueString,
		target.InitialValueRoot,
		target.InitialValueOperation,
		target.InitialValueDeniedOperation:
		return RawPresent, true
	default:
		return RawInvalid, false
	}
}

func (owner *schema) addBootMetatableRoutes() bool {
	if owner == nil || owner.source == nil {
		return false
	}
	for index := 0; index < owner.source.Host().Attachments().Count(); index++ {
		attachment, ok := owner.source.Host().Attachments().At(index)
		if !ok || owner.bootMetatableRoutes[attachment] != 0 {
			return false
		}
		base, metatable, mapped := owner.source.Host().Attachments().Mapping(attachment)
		metatableID := owner.bootIndex[metatable]
		if !mapped || base == target.InitialValueInvalid || metatableID == 0 || uint64(len(owner.metatableRoutes)) >= uint64(^uint32(0)) {
			return false
		}
		owner.metatableRoutes = append(owner.metatableRoutes, metatableRouteRow{
			primitive: base,
			metatable: metatableID,
			role:      materialization.Exact,
		})
		owner.bootMetatableRoutes[attachment] = uint32(len(owner.metatableRoutes))
	}
	return true
}

func (owner *schema) addField(root uint32, shard linkproject.Shard, field keyspace.Term, geometry programflow.AccessGeometry) bool {
	if owner == nil || owner.source == nil || root == 0 || root > uint32(len(owner.roots)) || field == 0 || !geometry.Available() {
		return false
	}
	row := owner.roots[root-1]
	if row.kind != RootAllocation || row.allocation.shard != shard || row.allocation.kind != AllocationTable {
		return false
	}
	mounts := owner.source.Project().Mounts()
	_, shardIndexOK := mounts.Index(shard)
	if !shardIndexOK {
		return false
	}
	p, ok := mounts.Program(shard)
	if !ok || p == nil {
		return false
	}
	authored := p.Flow().Authored()
	table, keyTerm, values, fieldKind, fieldOK := authored.Fields().Get(field)
	if !fieldOK || table != row.allocation.term || keyTerm == 0 || values == 0 {
		return false
	}
	normalized, normalizedOK := geometry.TableFields().Get(field)
	if !normalizedOK {
		return false
	}
	var slotID uint32
	switch fieldKind {
	case flowkind.FieldKey:
		// FieldKey is the one dynamic field form. Its raw source term is the
		// dynamic selector identity; a zero normalized key is not enough to
		// classify a field as dynamic.
		slotID = owner.addDynamicSlot(sourceCoordinate{shard: shard, term: keyTerm})
	case flowkind.FieldList, flowkind.FieldName, flowkind.FieldExact:
		if normalized == 0 {
			// Nil, NaN, and other non-storable exact fields are not dynamic.
			// An executable constructor with one is malformed for Heap and
			// fails closed.
			return false
		}
		linkKey, keyOK := owner.source.Project().Keys().ForProgram(shard, p, normalized)
		if !keyOK {
			return false
		}
		slotID = owner.addExactSlot(linkKey)
	default:
		return false
	}
	if slotID == 0 || !owner.addLocalSlot(slotID, root) {
		return false
	}
	var finalOpen, valuesOK bool
	values, finalOpen, valuesOK = authored.Fields().Values(field)
	if !valuesOK || values == 0 {
		return false
	}
	payloadID := owner.addPayload(payloadRow{kind: payloadValues, shard: shard, values: values})
	if payloadID == 0 || !owner.addLocalPayload(payloadID, root, slotID) {
		return false
	}
	fieldID := uint32(len(owner.fields) + 1)
	owner.fields = append(owner.fields, fieldRow{fieldSource: fieldSource{root: root, term: field}, kind: fieldKind, keyTerm: keyTerm, slot: slotID, payload: payloadID})
	if owner.roots[root-1].fieldStart == 0 {
		owner.roots[root-1].fieldStart = fieldID
	}
	owner.roots[root-1].fieldCount++
	if finalOpen {
		id := owner.addSlot(slotRow{kind: SlotOpenTail, field: fieldID})
		if id == 0 || !owner.addLocalSlot(id, root) || !owner.addLocalPayload(payloadID, root, id) {
			return false
		}
		owner.fields[fieldID-1].openTail = id
	}
	return true
}

func (owner *schema) addIndexAccesses(shard linkproject.Shard, p *program.Program) bool {
	if owner == nil || owner.source == nil || p == nil {
		return false
	}
	flow := p.Flow()
	geometry := flow.AccessGeometry()
	if !geometry.Available() {
		return false
	}
	executable := flow.Executable()
	indexGeometry := geometry.IndexAccesses()
	reads, writes := indexGeometry.Reads(), indexGeometry.Writes()
	for index := 0; index < reads.Count(); index++ {
		read, ok := reads.At(index)
		if !ok || keyspace.TermFamily(read) != keyspace.FamilyRead || keyspace.TermOrdinal(read) == 0 || !executable.Contains(read) {
			return false
		}
		base, keyTerm, lens, rowOK := reads.Get(read)
		if !rowOK || base == 0 || keyTerm == 0 || lens == 0 || !owner.appendIndexRead(shard, geometry, read, base, keyTerm, lens) {
			return false
		}
	}
	for index := 0; index < writes.Count(); index++ {
		write, ok := writes.At(index)
		if !ok || keyspace.TermFamily(write) != keyspace.FamilyWrite || keyspace.TermOrdinal(write) == 0 || !executable.Contains(write) {
			return false
		}
		base, keyTerm, values, position, lens, rowOK := writes.Get(write)
		if !rowOK || base == 0 || keyTerm == 0 || values == 0 || position < 0 || lens == 0 || !owner.appendIndexWrite(shard, geometry, write, base, keyTerm, values, position, lens) {
			return false
		}
	}
	return true
}

func (owner *schema) indexSlot(shard linkproject.Shard, geometry programflow.AccessGeometry, lens, keyTerm keyspace.Term) (uint32, bool) {
	if lens == 0 || keyTerm == 0 {
		return 0, false
	}
	switch keyspace.TermFamily(lens) {
	case keyspace.FamilyLensExact:
		normalized, ok := geometry.ExactLenses().Get(lens)
		// A valid exact row with no storable key is never dynamic. Executable
		// candidates fail closed rather than admitting an invented selector.
		if !ok || normalized == 0 {
			return 0, false
		}
		mounts := owner.source.Project().Mounts()
		_, shardIndexOK := mounts.Index(shard)
		if !shardIndexOK {
			return 0, false
		}
		p, programOK := mounts.Program(shard)
		if !programOK || p == nil {
			return 0, false
		}
		key, keyOK := owner.source.Project().Keys().ForProgram(shard, p, normalized)
		if !keyOK {
			return 0, false
		}
		slot := owner.addExactSlot(key)
		return slot, slot != 0
	case keyspace.FamilyLensKey:
		if _, ok := geometry.DynamicLenses().Get(lens); !ok {
			return 0, false
		}
		// Dynamic selectors carry a runtime key term. It must be a Link value
		// so topology can evaluate it after sealing; exact selectors instead
		// use their normalized key and never require the authored key term.
		if _, ok := owner.source.Boundary().Values().Of(shard, keyTerm); !ok {
			return 0, false
		}
		slot := owner.addDynamicSlot(sourceCoordinate{shard: shard, term: keyTerm})
		if slot == 0 || !owner.markGlobalSlot(slot) {
			return 0, false
		}
		return slot, true
	default:
		return 0, false
	}
}

func (owner *schema) appendIndexRead(shard linkproject.Shard, geometry programflow.AccessGeometry, read, base, keyTerm, lens keyspace.Term) bool {
	if uint64(len(owner.indexAccesses)) >= uint64(^uint32(0)) {
		return false
	}
	if _, ok := owner.source.Boundary().Values().Of(shard, read); !ok {
		return false
	}
	if _, ok := owner.source.Boundary().Values().Of(shard, base); !ok {
		return false
	}
	slot, ok := owner.indexSlot(shard, geometry, lens, keyTerm)
	if !ok {
		return false
	}
	owner.indexAccesses = append(owner.indexAccesses, indexAccessRow{shard: shard, read: read, base: base, keyTerm: keyTerm, position: -1, lens: lens, slot: slot})
	return true
}

func (owner *schema) appendIndexWrite(shard linkproject.Shard, geometry programflow.AccessGeometry, write, base, keyTerm, values keyspace.Term, position int, lens keyspace.Term) bool {
	if uint64(len(owner.indexAccesses)) >= uint64(^uint32(0)) || position < 0 || uint64(position) > uint64(^uint32(0)) {
		return false
	}
	if _, ok := owner.source.Boundary().Values().Of(shard, base); !ok {
		return false
	}
	slot, ok := owner.indexSlot(shard, geometry, lens, keyTerm)
	if !ok {
		return false
	}
	payload := owner.addPayload(payloadRow{kind: payloadValues, shard: shard, values: values, index: uint32(position)})
	if payload == 0 || !owner.addGlobalPayload(payload, slot) {
		return false
	}
	owner.indexAccesses = append(owner.indexAccesses, indexAccessRow{shard: shard, write: write, base: base, keyTerm: keyTerm, values: values, position: position, lens: lens, slot: slot, payload: payload})
	return true
}

func (owner *schema) addExactSlot(exact linkproject.Key) uint32 {
	if owner == nil {
		return 0
	}
	if id := owner.exactSlots[exact]; id != 0 {
		return id
	}
	id := owner.addSlot(slotRow{kind: SlotExact, exact: exact})
	if id != 0 {
		owner.exactSlots[exact] = id
		owner.slotSupport[id-1].global = true
	}
	return id
}

func (owner *schema) addDynamicSlot(dynamic sourceCoordinate) uint32 {
	if owner == nil || dynamic.term == 0 {
		return 0
	}
	if _, ok := owner.source.Project().Mounts().Index(dynamic.shard); !ok {
		return 0
	}
	if id := owner.dynamicSlots[dynamic]; id != 0 {
		return id
	}
	id := owner.addSlot(slotRow{kind: SlotDynamic, dynamic: dynamic})
	if id != 0 {
		owner.dynamicSlots[dynamic] = id
	}
	return id
}

func (owner *schema) addSlot(row slotRow) uint32 {
	if owner == nil || uint64(len(owner.slots)) >= uint64(^uint32(0)) {
		return 0
	}
	owner.slots = append(owner.slots, row)
	owner.slotSupport = append(owner.slotSupport, slotSupport{global: row.kind == SlotUnknown})
	return uint32(len(owner.slots))
}

func (owner *schema) ensureUnknownSlot() uint32 {
	if owner == nil {
		return 0
	}
	if owner.unknownSlot != 0 {
		return owner.unknownSlot
	}
	owner.unknownSlot = owner.addSlot(slotRow{kind: SlotUnknown})
	return owner.unknownSlot
}

func (owner *schema) addPayload(row payloadRow) uint32 {
	if owner == nil || owner.source == nil {
		return 0
	}
	if id := owner.payloadIndex[row]; id != 0 {
		return id
	}
	switch row.kind {
	case payloadValues:
		p, ok := owner.source.Project().Mounts().Program(row.shard)
		if !ok || p == nil {
			return 0
		}
		if _, ok := p.Flow().Authored().Values().Position(row.values, int(row.index)); !ok {
			return 0
		}
	case payloadInitial:
		contract, ok := owner.source.Boundary().Target()
		if !ok || contract == nil {
			return 0
		}
		if kind, valid := contract.InitialValueKind(row.initial); !valid || kind == target.InitialValueInvalid {
			return 0
		}
	default:
		return 0
	}
	if uint64(len(owner.payloads)) >= uint64(^uint32(0)) {
		return 0
	}
	owner.payloads = append(owner.payloads, row)
	owner.payloadSupport = append(owner.payloadSupport, payloadSupport{})
	id := uint32(len(owner.payloads))
	owner.payloadIndex[row] = id
	return id
}

func (owner *schema) markGlobalSlot(id uint32) bool {
	if id == 0 || uint64(id) > uint64(len(owner.slotSupport)) {
		return false
	}
	owner.slotSupport[id-1].global = true
	return true
}

func (owner *schema) addLocalSlot(slot, root uint32) bool {
	if slot == 0 || root == 0 || uint64(slot) > uint64(len(owner.slotSupport)) || uint64(root) > owner.rootCount() {
		return false
	}
	owner.localSlots[rootSlot{root: root, slot: slot}] = struct{}{}
	return true
}

func (owner *schema) addGlobalPayload(payload, slot uint32) bool {
	if payload == 0 || slot == 0 || uint64(payload) > uint64(len(owner.payloadSupport)) || uint64(slot) > uint64(len(owner.slots)) {
		return false
	}
	support := &owner.payloadSupport[payload-1]
	if support.globalSlots == nil {
		support.globalSlots = make(map[uint32]struct{})
	}
	support.globalSlots[slot] = struct{}{}
	return true
}

func (owner *schema) addLocalPayload(payload, root, slot uint32) bool {
	if payload == 0 || root == 0 || slot == 0 || uint64(payload) > uint64(len(owner.payloadSupport)) || uint64(root) > owner.rootCount() || uint64(slot) > uint64(len(owner.slots)) {
		return false
	}
	support := &owner.payloadSupport[payload-1]
	if support.local == nil {
		support.local = make(map[rootSlot]struct{})
		support.roots = make(map[uint32]struct{})
	}
	support.local[rootSlot{root: root, slot: slot}] = struct{}{}
	support.roots[root] = struct{}{}
	return true
}

func (owner *schema) finish() bool {
	if owner == nil || owner.source == nil || len(owner.slots) == 0 {
		return false
	}
	if !owner.buildRootKinds() || !owner.buildAtomicKeys() {
		return false
	}
	// Build the exact inverse only after all Program roots and index geometry
	// rows are complete. A duplicate tuple is an ambiguity in the canonical
	// relation and therefore rejects the whole Heap seal.
	if !owner.sealOccurrenceInverses() {
		return false
	}
	owner.bootEntryOrder = owner.bootEntryOrder[:0]
	for entry := range owner.bootEntries {
		owner.bootEntryOrder = append(owner.bootEntryOrder, entry)
	}
	sort.Slice(owner.bootEntryOrder, func(left, right int) bool {
		if owner.bootEntryOrder[left].root != owner.bootEntryOrder[right].root {
			return owner.bootEntryOrder[left].root < owner.bootEntryOrder[right].root
		}
		return owner.bootEntryOrder[left].slot < owner.bootEntryOrder[right].slot
	})

	// A present tuple consists solely of sealed source/payload coordinates and
	// two owner-issued containment facts. Each fact is None, Unknown, or one of
	// the exact references. Its finite denominator proves canonical-widening
	// descent; no threshold decides whether a tuple is retained.
	references, ok := owner.referencePotential()
	if !ok {
		return false
	}
	owner.referenceCount = references
	containments, ok := safeAdd(references, 2)
	if !ok {
		return false
	}
	present, ok := safeMul(uint64(len(owner.slots)), uint64(len(owner.payloads)))
	if !ok {
		return false
	}
	present, ok = safeMul(present, containments)
	if !ok {
		return false
	}
	present, ok = safeMul(present, containments)
	if !ok {
		return false
	}
	if present == ^uint64(0) {
		return false
	}
	owner.presentPotential = present
	if !owner.sealWidenRankBounds() {
		return false
	}
	owner.id = heapContentID(owner.linkID)
	if !owner.id.Available() {
		return false
	}
	owner.bottom = Value{owner: owner}
	owner.top = Value{owner: owner, top: true}
	return true
}

// sealOccurrenceInverses derives the two exact occurrence indexes from the
// already-complete semantic rows. It deliberately runs once at the end of
// Heap sealing: callers can look up one supplied Program occurrence without
// walking the root or access denominators, while no new source row or identity
// is introduced.
func (owner *schema) sealOccurrenceInverses() bool {
	if owner == nil || owner.source == nil || owner.programAllocationOrdinals != nil || owner.indexAccessOrdinals != nil {
		return false
	}
	project := owner.source.Project()
	if project == nil {
		return false
	}
	mounts := project.Mounts()
	allocations := make(map[programAllocationOccurrence]uint32, owner.programRootCount)
	if uint64(owner.programRootCount) > uint64(len(owner.roots)) {
		return false
	}
	for index := uint32(0); index < owner.programRootCount; index++ {
		row := owner.roots[index]
		if row.kind != RootAllocation || row.allocation.shard == (linkproject.Shard{}) ||
			!validProgramAllocation(row.allocation.shard, mounts, row.allocation.term, row.allocation.kind) {
			return false
		}
		occurrence := programAllocationOccurrence{shard: row.allocation.shard, term: row.allocation.term}
		if allocations[occurrence] != 0 {
			return false
		}
		allocations[occurrence] = index + 1
	}

	accesses := make(map[indexAccessOccurrence]uint32, len(owner.indexAccesses))
	for index, row := range owner.indexAccesses {
		if row.shard == (linkproject.Shard{}) || !validIndexAccessRow(row, mounts) {
			return false
		}
		term := row.read
		if term == 0 {
			term = row.write
		}
		occurrence := indexAccessOccurrence{shard: row.shard, term: term}
		if accesses[occurrence] != 0 {
			return false
		}
		accesses[occurrence] = uint32(index + 1)
	}
	owner.programAllocationOrdinals = allocations
	owner.indexAccessOrdinals = accesses
	return true
}

func validProgramAllocation(shard linkproject.Shard, mounts linkproject.Mounts, term keyspace.Term, kind AllocationKind) bool {
	if shard == (linkproject.Shard{}) || !validProgramAllocationTermShape(term) {
		return false
	}
	p, ok := mounts.Program(shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(term) {
		return false
	}
	switch kind {
	case AllocationTable:
		_, ok = p.Flow().Authored().Tables().Get(term)
	case AllocationClosure:
		_, _, _, ok = p.Flow().Authored().Functions().Get(term)
	default:
		return false
	}
	return ok
}

func validProgramAllocationTermShape(term keyspace.Term) bool {
	if term == 0 || keyspace.TermOrdinal(term) == 0 {
		return false
	}
	family := keyspace.TermFamily(term)
	return family == keyspace.FamilyTable || family == keyspace.FamilyFunction
}

func validProgramAllocationTerm(owner *program.Program, term keyspace.Term) bool {
	if owner == nil || !validProgramAllocationTermShape(term) || !owner.Flow().Executable().Contains(term) {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyTable:
		_, ok := owner.Flow().Authored().Tables().Get(term)
		return ok
	case keyspace.FamilyFunction:
		_, _, _, ok := owner.Flow().Authored().Functions().Get(term)
		return ok
	default:
		return false
	}
}

func validProgramIndexAccessOccurrence(owner *program.Program, occurrence keyspace.Term) bool {
	if owner == nil || occurrence == 0 || keyspace.TermOrdinal(occurrence) == 0 || !owner.Flow().AccessGeometry().Available() || !owner.Flow().Executable().Contains(occurrence) {
		return false
	}
	geometry := owner.Flow().AccessGeometry().IndexAccesses()
	switch keyspace.TermFamily(occurrence) {
	case keyspace.FamilyRead:
		_, _, _, ok := geometry.Reads().Get(occurrence)
		return ok
	case keyspace.FamilyWrite:
		_, _, _, _, _, ok := geometry.Writes().Get(occurrence)
		return ok
	default:
		return false
	}
}

func validIndexAccessRow(row indexAccessRow, mounts linkproject.Mounts) bool {
	if row.read != 0 && row.write != 0 || row.read == 0 && row.write == 0 || row.base == 0 || row.keyTerm == 0 || row.lens == 0 {
		return false
	}
	p, ok := mounts.Program(row.shard)
	if !ok || p == nil || !p.Flow().AccessGeometry().Available() {
		return false
	}
	geometry := p.Flow().AccessGeometry().IndexAccesses()
	if row.read != 0 {
		if keyspace.TermFamily(row.read) != keyspace.FamilyRead || row.values != 0 || row.position != -1 || row.payload != 0 || !p.Flow().Executable().Contains(row.read) {
			return false
		}
		base, keyTerm, lens, rowOK := geometry.Reads().Get(row.read)
		return rowOK && base == row.base && keyTerm == row.keyTerm && lens == row.lens
	}
	if keyspace.TermFamily(row.write) != keyspace.FamilyWrite || row.values == 0 || row.position < 0 || row.payload == 0 || !p.Flow().Executable().Contains(row.write) {
		return false
	}
	base, keyTerm, values, position, lens, rowOK := geometry.Writes().Get(row.write)
	return rowOK && base == row.base && keyTerm == row.keyTerm && values == row.values && position == row.position && lens == row.lens
}

// sealWidenRankBounds proves that every fixed-coordinate Object score, and
// therefore every legal Value's at-most-three-object score, fits uint64.
// This is a representation admission proof performed once for the sealed
// finite schema; it is not a runtime convergence limit.  WidenRank may rely
// on these stored witnesses without repeating arithmetic or declining a
// valid Schema later.
func (owner *schema) sealWidenRankBounds() bool {
	if owner == nil || owner.presentPotential == ^uint64(0) || owner.referenceCount == ^uint64(0) {
		return false
	}
	// Every CellState scores at most presentPotential+1.  Object has one
	// residual coordinate for each legal Lua key kind and one fixed semantic
	// coordinate for each sealed exact/reference atom.  Its header score is
	// at most referenceCount+4: one missing shape, one missing frozen state,
	// and the referenceCount+2 exact/none/unknown metatable powerset capacity.
	cellBound, ok := safeAdd(owner.presentPotential, 1)
	if !ok {
		return false
	}
	coordinates, ok := safeAdd(uint64(legalKeyKindCount), uint64(len(owner.atomicKeys)))
	if !ok {
		return false
	}
	partitionBound, ok := safeMul(coordinates, cellBound)
	if !ok {
		return false
	}
	headerBound, ok := safeAdd(owner.referenceCount, 4)
	if !ok {
		return false
	}
	objectBound, ok := safeAdd(partitionBound, headerBound)
	if !ok || objectBound == 0 {
		return false
	}
	maxObjectSum, ok := safeMul(objectBound, 3)
	if !ok || maxObjectSum == 0 {
		return false
	}
	owner.fixedObjectRankBound = objectBound
	owner.maxObjectRankSum = maxObjectSum
	return true
}

// buildRootKinds records the closed, Link/Target-derived Lua-observable kind
// mask for each structural root. RootKind is deliberately insufficient: a
// Program closure is Function, and one fresh root can be Table or Function
// across sealed Candidate rows while retaining one canonical root identity.
func (owner *schema) buildRootKinds() bool {
	if owner == nil || owner.source == nil {
		return false
	}
	for index := uint64(0); index < owner.rootCount(); index++ {
		mask, ok := owner.rootRuntimeKinds(uint32(index + 1))
		if !ok || mask == 0 || !mask.Valid() || mask&^legalTableKeyKinds != 0 {
			return false
		}
	}
	return true
}

func (owner *schema) rootRuntimeKinds(root uint32) (runtimekind.Set, bool) {
	row, ok := owner.rootAt(root)
	if !ok {
		return 0, false
	}
	switch row.kind {
	case RootBoot:
		return runtimekind.Bit(runtimekind.Table), true
	case RootAllocation:
		if row.allocation.term != 0 {
			switch row.allocation.kind {
			case AllocationTable:
				return runtimekind.Bit(runtimekind.Table), true
			case AllocationClosure:
				return runtimekind.Bit(runtimekind.Function), true
			}
			return 0, false
		}
		return row.fresh.kinds, row.fresh.kinds.Valid() && row.fresh.kinds&^legalTableKeyKinds == 0
	default:
		return 0, false
	}
}

func freshRootKinds(kind target.FreshKind) (runtimekind.Set, bool) {
	switch kind {
	case target.FreshTable:
		return runtimekind.Bit(runtimekind.Table), true
	case target.FreshFunction:
		return runtimekind.Bit(runtimekind.Function), true
	case target.FreshThread:
		return runtimekind.Bit(runtimekind.Thread), true
	case target.FreshUserdata:
		return runtimekind.Bit(runtimekind.Userdata), true
	case target.FreshError, target.FreshReflection:
		// Error and reflection are fresh reference roots with no finer Heap
		// shape vocabulary. Preserve their production identity as the
		// conservative Userdata runtime family rather than dropping the root.
		return runtimekind.Bit(runtimekind.Userdata), true
	default:
		return 0, false
	}
}

func (owner *schema) buildAtomicKeys() bool {
	if owner == nil || owner.source == nil || owner.atomicKeys != nil {
		return false
	}
	keys := owner.source.Project().Keys()
	atoms := make([]keyAtom, 0, keys.Count()+int(owner.rootCount()))
	for index := 0; index < keys.Count(); index++ {
		key, ok := keys.At(index)
		if !ok {
			return false
		}
		atoms = append(atoms, keyAtom{kind: keyAtomExact, exact: key, exactOrdinal: uint32(index + 1)})
	}
	for index := uint64(0); index < owner.rootCount(); index++ {
		root, rootOK := owner.rootAt(uint32(index + 1))
		if !rootOK {
			return false
		}
		role := materialization.Invalid
		switch root.kind {
		case RootAllocation:
			role = materialization.Recent
		case RootBoot:
			role = materialization.Exact
		default:
			return false
		}
		atoms = append(atoms, keyAtom{kind: keyAtomReference, root: uint32(index + 1), role: role})
	}
	atoms = normalizeKeyAtoms(atoms)
	if len(atoms) != 0 && !validExactKeyAtoms(owner, atoms) {
		return false
	}
	for _, atom := range atoms {
		mask := keyAtomRuntimeKinds(owner, atom)
		index := int(mask)
		if index == 0 || index >= len(owner.atomMaskCounts) || owner.atomMaskCounts[index] == ^uint64(0) {
			return false
		}
		owner.atomMaskCounts[index]++
	}
	owner.atomicKeys = atoms
	return true
}

func safeMul(left, right uint64) (uint64, bool) {
	if left != 0 && right > ^uint64(0)/left {
		return 0, false
	}
	return left * right, true
}

func safeAdd(left, right uint64) (uint64, bool) {
	if left > ^uint64(0)-right {
		return 0, false
	}
	return left + right, true
}

func (owner *schema) referencePotential() (uint64, bool) {
	if owner == nil {
		return 0, false
	}
	var result uint64
	for index := uint64(0); index < owner.rootCount(); index++ {
		roles, ok := owner.referenceRolePotential(uint32(index + 1))
		if !ok || result > ^uint64(0)-roles {
			return 0, false
		}
		result += roles
	}
	return result, true
}

func (owner *schema) localRolePotential(root uint32) (uint64, bool) {
	if owner == nil || root == 0 {
		return 0, false
	}
	row, ok := owner.rootAt(root)
	if !ok {
		return 0, false
	}
	switch row.kind {
	case RootAllocation:
		return 3, true // one/recent and many/{recent,summary}
	case RootBoot:
		return 1, true // one/exact
	default:
		return 0, false
	}
}

func (owner *schema) referenceRolePotential(root uint32) (uint64, bool) {
	if owner == nil || root == 0 {
		return 0, false
	}
	row, ok := owner.rootAt(root)
	if !ok {
		return 0, false
	}
	switch row.kind {
	case RootAllocation:
		return 2, true // Recent and Summary only; Exact belongs solely to boot.
	case RootBoot:
		return 1, true
	default:
		return 0, false
	}
}

func (schema Schema) admitsReferenceRole(root uint32, role materialization.Role) bool {
	return schema.owner != nil && schema.owner.admitsReferenceRole(root, role)
}

func (owner *schema) admitsReferenceRole(root uint32, role materialization.Role) bool {
	if owner == nil || root == 0 || !role.Valid() {
		return false
	}
	row, ok := owner.rootAt(root)
	if !ok {
		return false
	}
	switch row.kind {
	case RootAllocation:
		return role == materialization.Recent || role == materialization.Summary
	case RootBoot:
		return role == materialization.Exact
	default:
		return false
	}
}

func heapContentID(linkID keyspace.ContentID) (id keyspace.ContentID) {
	// schemaFormat is the sole Heap schema/content identity discriminator.
	// Keeping one version word means a row-layout or projection-authority
	// change cannot accidentally leave a second, contradictory revision in
	// the identity hash.
	var payload [32 + 8]byte
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:], schemaFormat)
	digest := sha256.Sum256(payload[:])
	copy(id[:], digest[:])
	return id
}

func (schema Schema) valid() bool {
	return schema.owner != nil && schema.owner.source != nil && schema.owner.linkID.Available() && schema.owner.id.Available() &&
		(schema.owner.referenceCount != ^uint64(0) && schema.owner.presentPotential != ^uint64(0) &&
			schema.owner.fixedObjectRankBound != 0 && schema.owner.maxObjectRankSum != 0)
}

// Valid reports whether Schema is a fully sealed Heap authority.
func (schema Schema) Valid() bool { return schema.valid() }

// ContentID identifies the cold Heap declaration, never a recurrent Value.
func (schema Schema) ContentID() keyspace.ContentID {
	if !schema.valid() {
		return keyspace.ContentID{}
	}
	return schema.owner.id
}

// LinkContentID identifies the exact Link scope that admitted this family.
func (schema Schema) LinkContentID() keyspace.ContentID {
	if !schema.valid() {
		return keyspace.ContentID{}
	}
	return schema.owner.linkID
}

// Link returns the exact immutable structural authority behind this Heap
// schema. It is for typed Heap child declarations only, never recurrent fact
// state or an engine capability.
func (schema Schema) Link() *link.Link {
	if !schema.valid() {
		return nil
	}
	return schema.owner.source
}

// Rebind reconstructs the complete cold authority from an equivalent Link.
// State Values intentionally do not rebind or serialize.
func (schema Schema) Rebind(source *link.Link) (Schema, bool) {
	if !schema.valid() || source == nil || source.ContentID() != schema.owner.linkID {
		return Schema{}, false
	}
	rebound, ok := Seal(source)
	if !ok || rebound.ContentID() != schema.ContentID() {
		return Schema{}, false
	}
	return rebound, true
}

func (schema Schema) KeyCount() int {
	if !schema.valid() {
		return 0
	}
	count := schema.owner.rootCount()
	if count > uint64(^uint(0)>>1) {
		return 0
	}
	return int(count)
}

// KeyAt returns one owner-issued private dense selector for an exact root.
func (schema Schema) KeyAt(index int) (Key, bool) {
	if !schema.valid() || index < 0 || uint64(index) >= schema.owner.rootCount() || uint64(index) >= uint64(^uint32(0)) {
		return Key{}, false
	}
	return Key{owner: schema.owner, slot: uint32(index + 1)}, true
}

// KeyForProgramAllocation resolves one exact mounted Program allocation
// occurrence to Heap's already-issued aggregate Key. The Shard and Program
// must belong to this Schema's Link and the term must be an executable Table
// or Function occurrence. No root enumeration or source row is performed.
func (schema Schema) KeyForProgramAllocation(shard linkproject.Shard, owner *program.Program, term keyspace.Term) (Key, bool) {
	if !schema.valid() || schema.owner.programAllocationOrdinals == nil || owner == nil || term == 0 {
		return Key{}, false
	}
	mounts := schema.owner.source.Project().Mounts()
	mounted, mountedOK := mounts.Program(shard)
	if !mountedOK || mounted != owner || !validProgramAllocationTerm(owner, term) {
		return Key{}, false
	}
	slot := schema.owner.programAllocationOrdinals[programAllocationOccurrence{shard: shard, term: term}]
	if slot == 0 {
		return Key{}, false
	}
	key := Key{owner: schema.owner, slot: slot}
	return key, key.Valid() && key.Kind() == RootAllocation
}

// OwnsKey rejects a forged or foreign Heap coordinate without reconstructing
// its Program/Target source.
func (schema Schema) OwnsKey(key Key) bool {
	return schema.valid() && key.valid() && key.owner == schema.owner
}

// KeyID is Heap's stable identity for an already owner-issued coordinate.
// It is not a second root representation and it never serializes a Link
// allocation handle.
func (schema Schema) KeyID(key Key) (keyspace.ContentID, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner {
		return keyspace.ContentID{}, false
	}
	var payload [32 + 2*8]byte
	id := schema.ContentID()
	copy(payload[:32], id[:])
	binary.BigEndian.PutUint64(payload[32:40], 0x686561702d6b6579) // "heap-key"
	binary.BigEndian.PutUint64(payload[40:48], uint64(key.slot))
	return sha256.Sum256(payload[:]), true
}

// KeyForBootRoot admits only an existing actor-local BootRoot from this Link
// into the same Heap Key family as allocation roots.
func (schema Schema) KeyForBootRoot(root linkhost.BootRoot) (Key, bool) {
	if !schema.valid() {
		return Key{}, false
	}
	id := schema.owner.bootIndex[root]
	if id == 0 {
		return Key{}, false
	}
	return Key{owner: schema.owner, slot: id}, true
}

// BootFrozen projects Target's canonical whole-object boot header through the
// actor-local Link root.  It is deliberately separate from InitialFrozen,
// which constrains only one exact entry.  Callers receive the Heap header
// value rather than Target vocabulary, so target policy cannot leak into
// recurrent Heap transfer APIs.
func (schema Schema) BootFrozen(key Key) (Frozen, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootBoot {
		return FrozenInvalid, false
	}
	row, ok := schema.owner.rootAt(key.slot)
	if !ok {
		return FrozenInvalid, false
	}
	if row.bootImmutable {
		return FrozenFrozen, true
	}
	return FrozenMutable, true
}

func (schema Schema) SlotCount() int {
	if !schema.valid() {
		return 0
	}
	return len(schema.owner.slots)
}

func (schema Schema) SlotAt(index int) (Slot, bool) {
	if !schema.valid() || index < 0 || index >= len(schema.owner.slots) {
		return Slot{}, false
	}
	return Slot{owner: schema.owner, id: uint32(index + 1)}, true
}

// UnknownSlot returns the one sealed Heap-owned fallback partition.
func (schema Schema) UnknownSlot() (Slot, bool) {
	if !schema.valid() || schema.owner.unknownSlot == 0 {
		return Slot{}, false
	}
	return Slot{owner: schema.owner, id: schema.owner.unknownSlot}, true
}

// FieldCount and FieldAt enumerate the dense field range sealed for one
// allocation Key. Fresh and Boot roots have no constructor fields.
func (schema Schema) FieldCount(key Key) int {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootAllocation {
		return 0
	}
	root, ok := schema.owner.rootAt(key.slot)
	if !ok || uint64(root.fieldCount) > uint64(^uint(0)>>1) {
		return 0
	}
	return int(root.fieldCount)
}

func (schema Schema) FieldAt(key Key, index int) (Field, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || index < 0 || key.Kind() != RootAllocation {
		return Field{}, false
	}
	root, ok := schema.owner.rootAt(key.slot)
	if !ok || index >= int(root.fieldCount) || root.fieldStart == 0 {
		return Field{}, false
	}
	return Field{owner: schema.owner, index: root.fieldStart + uint32(index)}, true
}

// FieldOrigin returns the exact table Key and direct Program TableField term.
func (schema Schema) FieldOrigin(field Field) (Key, linkproject.Shard, keyspace.Term, bool) {
	if !schema.ownsField(field) {
		return Key{}, linkproject.Shard{}, 0, false
	}
	row := schema.owner.fields[field.index-1]
	root := Key{owner: schema.owner, slot: row.root}
	physical, physicalOK := schema.owner.rootAt(row.root)
	if !physicalOK {
		return Key{}, linkproject.Shard{}, 0, false
	}
	allocation := physical.allocation
	return root, allocation.shard, row.term, allocation.term != 0
}

func (schema Schema) SlotForField(field Field) (Slot, bool) {
	if !schema.ownsField(field) {
		return Slot{}, false
	}
	return schema.slot(schema.owner.fields[field.index-1].slot)
}

func (schema Schema) SlotForOpenFieldTail(field Field) (Slot, bool) {
	if !schema.ownsField(field) {
		return Slot{}, false
	}
	return schema.slot(schema.owner.fields[field.index-1].openTail)
}

// IndexAccessCount and IndexAccessAt enumerate Heap's direct sealed candidate
// rows. Candidate order is Reads first, followed by Writes, as supplied by
// Flow AccessGeometry.
func (schema Schema) IndexAccessCount() int {
	if !schema.valid() {
		return 0
	}
	return len(schema.owner.indexAccesses)
}

func (schema Schema) IndexAccessAt(index int) (IndexAccess, bool) {
	if !schema.valid() || index < 0 || index >= len(schema.owner.indexAccesses) {
		return IndexAccess{}, false
	}
	access := IndexAccess{owner: schema.owner, index: uint32(index + 1)}
	return access, schema.ownsIndexAccess(access)
}

// IndexAccessFor resolves one exact mounted Program index-access occurrence
// to Heap's canonical IndexAccess row. Both Read and Write terms are
// accepted; the supplied Program, owner-fenced Shard, executable occurrence,
// and sealed geometry must all agree before the inverse is observed.
func (schema Schema) IndexAccessFor(shard linkproject.Shard, owner *program.Program, occurrence keyspace.Term) (IndexAccess, bool) {
	if !schema.valid() || schema.owner.indexAccessOrdinals == nil || owner == nil || occurrence == 0 {
		return IndexAccess{}, false
	}
	mounts := schema.owner.source.Project().Mounts()
	mounted, mountedOK := mounts.Program(shard)
	if !mountedOK || mounted != owner || !validProgramIndexAccessOccurrence(owner, occurrence) {
		return IndexAccess{}, false
	}
	index := schema.owner.indexAccessOrdinals[indexAccessOccurrence{shard: shard, term: occurrence}]
	if index == 0 || uint64(index) > uint64(len(schema.owner.indexAccesses)) {
		return IndexAccess{}, false
	}
	access := IndexAccess{owner: schema.owner, index: index}
	return access, schema.ownsIndexAccess(access)
}

func (schema Schema) PayloadForField(field Field) (Payload, bool) {
	if !schema.ownsField(field) {
		return Payload{}, false
	}
	return schema.payload(schema.owner.fields[field.index-1].payload)
}

// IndexAccessGeometry returns the direct typed geometry retained by Heap.
// Exactly one of read and write is nonzero; read rows have position -1,
// values zero, and write rows carry their authored Values and position.
func (schema Schema) IndexAccessGeometry(access IndexAccess) (IndexGeometry, bool) {
	if !schema.ownsIndexAccess(access) {
		return IndexGeometry{}, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	return IndexGeometry{Shard: row.shard, ReadTerm: row.read, WriteTerm: row.write, Base: row.base, KeyTerm: row.keyTerm, Values: row.values, Position: row.position, Lens: row.lens}, true
}

// SlotForIndexAccess returns the exact slot sealed on the Heap row.
func (schema Schema) SlotForIndexAccess(access IndexAccess) (Slot, bool) {
	if !schema.ownsIndexAccess(access) {
		return Slot{}, false
	}
	return schema.slot(schema.owner.indexAccesses[access.index-1].slot)
}

// PayloadForIndexAccess returns the exact sealed RHS payload. Read rows have
// no payload.
func (schema Schema) PayloadForIndexAccess(access IndexAccess) (Payload, bool) {
	if !schema.ownsIndexAccess(access) {
		return Payload{}, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	if row.read != 0 || row.payload == 0 {
		return Payload{}, false
	}
	return schema.payload(row.payload)
}

func (schema Schema) IndexAccessResult(access IndexAccess) (linkboundary.Value, bool) {
	if !schema.ownsIndexAccess(access) {
		return linkboundary.Value{}, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	if row.read == 0 {
		return linkboundary.Value{}, false
	}
	return schema.owner.source.Boundary().Values().Of(row.shard, row.read)
}

func (schema Schema) IndexAccessID(access IndexAccess) (keyspace.ContentID, bool) {
	if !schema.ownsIndexAccess(access) {
		return keyspace.ContentID{}, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	sourceTerm := row.read
	if sourceTerm == 0 {
		sourceTerm = row.write
	}
	var payload [32 + 2*8]byte
	id := schema.ContentID()
	copy(payload[:32], id[:])
	shardIndex, ok := schema.owner.source.Project().Mounts().Index(row.shard)
	if !ok {
		return keyspace.ContentID{}, false
	}
	binary.BigEndian.PutUint64(payload[32:40], uint64(shardIndex+1))
	binary.BigEndian.PutUint64(payload[40:48], uint64(sourceTerm))
	return sha256.Sum256(payload[:]), true
}

// BootEntry is one typed immutable bootstrap entry projection. It separates
// Target's raw-absence row from a present initial-value payload so callers can
// never reconstruct absence as a forged present payload.
type BootEntry struct {
	owner *schema
	root  uint32
	slot  uint32
}

// BootEntryCount is the finite, Link/Target-derived bootstrap raw-slot
// denominator.  It is deliberately exposed only as typed BootEntry values:
// callers never receive the owner-private root/slot pair used to store it.
func (schema Schema) BootEntryCount() int {
	if !schema.valid() {
		return 0
	}
	return len(schema.owner.bootEntryOrder)
}

// BootEntryAt returns one canonical bootstrap raw-slot relation.  The order
// is an implementation traversal only; BootEntry.ID is its stable semantic
// identity and callers must not retain this ordinal.
func (schema Schema) BootEntryAt(index int) (BootEntry, bool) {
	if !schema.valid() || index < 0 || index >= len(schema.owner.bootEntryOrder) {
		return BootEntry{}, false
	}
	pair := schema.owner.bootEntryOrder[index]
	entry := BootEntry{owner: schema.owner, root: pair.root, slot: pair.slot}
	return entry, entry.valid()
}

func (entry BootEntry) valid() bool {
	if entry.owner == nil || entry.root == 0 || entry.slot == 0 || uint64(entry.root) > entry.owner.rootCount() || int(entry.slot) > len(entry.owner.slots) {
		return false
	}
	_, ok := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	return ok
}

// BootEntry projects one exact Target-owned bootstrap row.
func (schema Schema) BootEntry(key Key, slot Slot) (BootEntry, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootBoot || !slot.valid() || slot.owner != schema.owner {
		return BootEntry{}, false
	}
	entry := BootEntry{owner: schema.owner, root: key.slot, slot: slot.id}
	return entry, entry.valid()
}

// Key returns the exact actor-local bootstrap root selected by this entry.
func (entry BootEntry) Key() (Key, bool) {
	if !entry.valid() {
		return Key{}, false
	}
	return Key{owner: entry.owner, slot: entry.root}, true
}

// Slot returns the exact canonical key partition selected by this entry.
func (entry BootEntry) Slot() (Slot, bool) {
	if !entry.valid() {
		return Slot{}, false
	}
	return Slot{owner: entry.owner, id: entry.slot}, true
}

// ID is the stable identity of this exact Link/Target bootstrap raw-slot
// projection. root and slot stay private carrier selectors; their canonical
// pair is hashed under the sealed Heap schema instead of being exposed as a
// second key plane.
func (entry BootEntry) ID() (keyspace.ContentID, bool) {
	if !entry.valid() || !entry.owner.id.Available() {
		return keyspace.ContentID{}, false
	}
	var payload [32 + 4*8]byte
	copy(payload[:32], entry.owner.id[:])
	words := payload[32:]
	binary.BigEndian.PutUint64(words[0:8], 0x686561702d626f6f) // "heap-boo"
	binary.BigEndian.PutUint64(words[8:16], 1)
	binary.BigEndian.PutUint64(words[16:24], uint64(entry.root))
	binary.BigEndian.PutUint64(words[24:32], uint64(entry.slot))
	return sha256.Sum256(payload[:]), true
}

// Projection returns RawAbsent with no Payload for Target InitialValueNil and
// InitialValueAbsent. Target retains those distinct contract identities; Heap
// deliberately projects both to the one Lua raw-slot absence relation. A
// RawPresent entry returns its exact non-nil initial-value Payload.
func (entry BootEntry) Projection() (RawPresence, Payload, bool) {
	if !entry.valid() {
		return RawInvalid, Payload{}, false
	}
	row := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	if row.raw == RawAbsent {
		return RawAbsent, Payload{}, true
	}
	if row.raw != RawPresent || row.payload == 0 {
		return RawInvalid, Payload{}, false
	}
	return RawPresent, Payload{owner: entry.owner, id: row.payload}, true
}

// Mutability returns the immutable Target policy for this entry. It is not a
// current Heap frozen-state conclusion.
func (entry BootEntry) Mutability() (target.InitialMutability, bool) {
	if !entry.valid() {
		return target.InitialMutabilityInvalid, false
	}
	row := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	return row.mutability, row.mutability == target.InitialMutable || row.mutability == target.InitialFrozen
}

// Reference makes one exact Link structural-root/role operand available to a
// correlated containment fact. Bootstrap roots have one stable Exact role;
// allocation roots retain Recent/Summary materialization roles.
func (schema Schema) Reference(key Key, role materialization.Role) (Reference, bool) {
	if !schema.valid() || key.owner != schema.owner || !key.valid() || !schema.admitsReferenceRole(key.slot, role) {
		return Reference{}, false
	}
	return Reference{owner: schema.owner, root: key.slot, role: role}, true
}

// ValueChild resolves a Root-valued present entry to its exact actor-local
// BootRoot image. Raw-absent and primitive rows intentionally have no child.
func (entry BootEntry) ValueChild() (Reference, bool) {
	if !entry.valid() {
		return Reference{}, false
	}
	row := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	if row.raw != RawPresent || row.payload == 0 || row.valueChild == 0 {
		return Reference{}, false
	}
	payload := entry.owner.payloads[row.payload-1]
	if payload.kind != payloadInitial || payload.initial != row.initial || uint64(row.valueChild) > entry.owner.rootCount() {
		return Reference{}, false
	}
	return Reference{owner: entry.owner, root: row.valueChild, role: materialization.Exact}, true
}

// ValueContainment classifies the value-edge of one raw-present Target boot
// payload. The Target initial-value kind is the sole authority: roots retain
// their exact actor-local child, callable payloads retain an opaque edge, and
// scalar payloads prove no reference edge. Raw-absent entries have no value
// containment tuple and therefore fail closed here.
func (entry BootEntry) ValueContainment() (Containment, bool) {
	if !entry.valid() {
		return Containment{}, false
	}
	row := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	if row.raw != RawPresent || row.payload == 0 || int(row.payload) > len(entry.owner.payloads) || entry.owner.source == nil {
		return Containment{}, false
	}
	payload := entry.owner.payloads[row.payload-1]
	contract, contractOK := entry.owner.source.Boundary().Target()
	if payload.kind != payloadInitial || payload.initial != row.initial || !contractOK || contract == nil {
		return Containment{}, false
	}
	kind, kindOK := contract.InitialValueKind(payload.initial)
	if !kindOK {
		return Containment{}, false
	}
	schema := Schema{owner: entry.owner}
	switch kind {
	case target.InitialValueRoot:
		child, childOK := entry.ValueChild()
		if !childOK {
			return Containment{}, false
		}
		return schema.ContainmentExact(child)
	case target.InitialValueOperation, target.InitialValueDeniedOperation:
		if row.valueChild != 0 {
			return Containment{}, false
		}
		return schema.ContainmentUnknown()
	case target.InitialValueBoolean, target.InitialValueInteger, target.InitialValueFloat, target.InitialValueString:
		if row.valueChild != 0 {
			return Containment{}, false
		}
		return schema.ContainmentNone()
	default:
		return Containment{}, false
	}
}

// BootMetatableRoute returns the existing immutable Link bootstrap route for
// one primitive base class. It does not imply ordinary dispatch selection.
func (schema Schema) BootMetatableRoute(attachment linkhost.BootMetatableAttachment) (MetatableRoute, bool) {
	if !schema.valid() {
		return MetatableRoute{}, false
	}
	id := schema.owner.bootMetatableRoutes[attachment]
	if id == 0 {
		return MetatableRoute{}, false
	}
	return MetatableRoute{owner: schema.owner, id: id}, true
}

// Admits is Heap's sole coordinate-specific carrier fence. The outer Value
// remains homogeneous across the Factor; this fence proves that each complete
// World is meaningful at the selected structural root and that every present
// tuple retains its Link/Target provenance.
func (schema Schema) Admits(key Key, value Value) bool {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || !schema.owns(value) {
		return false
	}
	if value.top || value.IsBottom() {
		return true
	}
	for _, world := range value.worlds {
		if !schema.admitsWorld(key, world) {
			return false
		}
	}
	return true
}

func (schema Schema) admitsWorld(key Key, world World) bool {
	if !world.valid() || world.owner != schema.owner {
		return false
	}
	switch key.Kind() {
	case RootAllocation:
		switch world.kind {
		case WorldZero:
			return true
		case WorldOne:
			return schema.admitsObject(key, world.recent)
		case WorldMany:
			return schema.admitsObject(key, world.recent) && schema.admitsObject(key, world.summary)
		default:
			return false
		}
	case RootBoot:
		return world.kind == WorldExact && schema.admitsObject(key, world.exact)
	default:
		return false
	}
}

func (schema Schema) admitsObject(key Key, object Object) bool {
	if !object.valid() || object.owner != schema.owner {
		return false
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !schema.admitsPartitionState(key, object.partition, kind, nil, object.partition.rest[kind]) {
			return false
		}
	}
	for _, exception := range object.partition.exceptions {
		if !schema.admitsPartitionState(key, object.partition, runtimekind.Invalid, &exception.atom, exception.state) {
			return false
		}
	}
	return true
}

func (schema Schema) admitsPartitionState(key Key, partition Partition, kind runtimekind.Kind, atom *keyAtom, state CellState) bool {
	if !partition.valid() || partition.owner != schema.owner || !state.valid() || state.owner != schema.owner || atom == nil && !legalKeyKind(kind) ||
		atom != nil && !validExactKeyAtom(schema.owner, *atom) {
		return false
	}
	for _, present := range state.presents {
		if !schema.admitsSlot(key.slot, present.slotID) || !schema.partitionAdmitsSlot(partition, kind, atom, present.slotID) ||
			!schema.admitsPayload(key.slot, present.slotID, present.payloadID) || !schema.admitsInitialPresent(key.slot, present) {
			return false
		}
	}
	return true
}

// partitionAdmitsSlot verifies only cold source-to-semantic-key incidence. A
// dynamic source proves no equality and may inhabit any complete partition
// coordinate. An exact source must inhabit its exact exception or a residual
// coordinate that still contains that atom; source occurrences never become a
// partition identity.
func (schema Schema) partitionAdmitsSlot(partition Partition, kind runtimekind.Kind, atom *keyAtom, slot uint32) bool {
	if schema.owner == nil || !partition.valid() || partition.owner != schema.owner || slot == 0 || int(slot) > len(schema.owner.slots) {
		return false
	}
	row := schema.owner.slots[slot-1]
	if row.kind != SlotExact {
		return true
	}
	keyIndex, keyOK := schema.owner.source.Project().Keys().Index(row.exact)
	if !keyOK {
		return false
	}
	exact := keyAtom{kind: keyAtomExact, exact: row.exact, exactOrdinal: uint32(keyIndex + 1)}
	if !validExactKeyAtom(schema.owner, exact) {
		return false
	}
	if atom != nil {
		return compareKeyAtom(*atom, exact) == 0
	}
	if !legalKeyKind(kind) || !keyAtomRuntimeKinds(schema.owner, exact).Contains(kind) {
		return false
	}
	_, excluded := partition.exceptionIndex(exact)
	return !excluded
}

// admitsInitialPresent preserves Target's immutable bootstrap row law. A
// Root-valued entry has one exact actor-local child; primitives have none.
// Coarsening a semantic cover never drops the tuple's original source slot.
func (schema Schema) admitsInitialPresent(root uint32, present Present) bool {
	if schema.owner == nil || present.payloadID == 0 || int(present.payloadID) > len(schema.owner.payloads) {
		return false
	}
	payload := schema.owner.payloads[present.payloadID-1]
	if payload.kind != payloadInitial {
		return true
	}
	entry, found := schema.owner.bootEntries[rootSlot{root: root, slot: present.slotID}]
	if !found || entry.raw != RawPresent || entry.payload != present.payloadID || entry.initial != payload.initial {
		return false
	}
	expected, expectedOK := (BootEntry{owner: schema.owner, root: root, slot: present.slotID}).ValueContainment()
	return expectedOK && present.valueContainment == expected && present.keyContainment.kind == ContainmentNone
}

func (schema Schema) admitsSlot(root, slot uint32) bool {
	return schema.owner != nil && schema.owner.admitsSlot(root, slot)
}

func (owner *schema) admitsSlot(root, slot uint32) bool {
	if owner == nil || root == 0 || slot == 0 || uint64(root) > owner.rootCount() || uint64(slot) > uint64(len(owner.slotSupport)) {
		return false
	}
	if owner.slotSupport[slot-1].global {
		return true
	}
	_, ok := owner.localSlots[rootSlot{root: root, slot: slot}]
	return ok
}

func (schema Schema) admitsPayload(root, slot, payload uint32) bool {
	return schema.owner != nil && schema.owner.admitsPayload(root, slot, payload)
}

func (owner *schema) admitsPayload(root, slot, payload uint32) bool {
	if owner == nil || root == 0 || slot == 0 || payload == 0 || uint64(payload) > uint64(len(owner.payloadSupport)) {
		return false
	}
	support := owner.payloadSupport[payload-1]
	if _, ok := support.globalSlots[slot]; ok {
		return true
	}
	_, ok := support.local[rootSlot{root: root, slot: slot}]
	return ok
}

// admitsUnknownPayload preserves source provenance after canonical slot
// coarsening. Assignment payloads apply at every root; constructor payloads
// remain confined to the roots at which they were structurally introduced.
func (schema Schema) admitsUnknownPayload(root, payload uint32) bool {
	return schema.owner != nil && schema.owner.admitsUnknownPayload(root, payload)
}

func (owner *schema) admitsUnknownPayload(root, payload uint32) bool {
	if owner == nil || root == 0 || payload == 0 || uint64(root) > owner.rootCount() || int(payload) > len(owner.payloadSupport) {
		return false
	}
	support := owner.payloadSupport[payload-1]
	if len(support.globalSlots) != 0 {
		return true
	}
	_, ok := support.roots[root]
	return ok
}

func (schema Schema) slot(id uint32) (Slot, bool) {
	if id == 0 || int(id) > len(schema.owner.slots) {
		return Slot{}, false
	}
	return Slot{owner: schema.owner, id: id}, true
}

func (schema Schema) payload(id uint32) (Payload, bool) {
	if id == 0 || int(id) > len(schema.owner.payloads) {
		return Payload{}, false
	}
	return Payload{owner: schema.owner, id: id}, true
}

func (schema Schema) ownsField(field Field) bool {
	if !schema.valid() || field.owner != schema.owner || field.index == 0 || int(field.index) > len(schema.owner.fields) {
		return false
	}
	row := schema.owner.fields[field.index-1]
	if row.root == 0 || uint64(row.root) > schema.owner.rootCount() || row.slot == 0 || row.payload == 0 || row.term == 0 || row.keyTerm == 0 {
		return false
	}
	root, ok := schema.owner.rootAt(row.root)
	if !ok || root.kind != RootAllocation || root.allocation.kind != AllocationTable || root.fieldStart == 0 || field.index < root.fieldStart || uint64(field.index-root.fieldStart) >= uint64(root.fieldCount) {
		return false
	}
	return uint64(row.slot) <= uint64(len(schema.owner.slots)) && uint64(row.payload) <= uint64(len(schema.owner.payloads))
}

func (schema Schema) ownsIndexAccess(access IndexAccess) bool {
	if !schema.valid() || access.owner != schema.owner || access.index == 0 || int(access.index) > len(schema.owner.indexAccesses) {
		return false
	}
	row := schema.owner.indexAccesses[access.index-1]
	if _, shardOK := schema.owner.source.Project().Mounts().Index(row.shard); !shardOK || row.base == 0 || row.keyTerm == 0 || row.lens == 0 || row.slot == 0 || uint64(row.slot) > uint64(len(schema.owner.slots)) {
		return false
	}
	if (row.read == 0) == (row.write == 0) {
		return false
	}
	if keyspace.TermFamily(row.lens) != keyspace.FamilyLensExact && keyspace.TermFamily(row.lens) != keyspace.FamilyLensKey {
		return false
	}
	if row.read != 0 {
		return keyspace.TermFamily(row.read) == keyspace.FamilyRead && row.values == 0 && row.position == -1 && row.payload == 0
	}
	return keyspace.TermFamily(row.write) == keyspace.FamilyWrite && row.values != 0 && row.position >= 0 && row.payload != 0 && uint64(row.payload) <= uint64(len(schema.owner.payloads))
}
