package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
)

// ModuleLoadCall is Value's sealed interpretation of one mounted call-result
// coordinate. The call and argument are exact existing factors; the write is
// the already-issued Value coordinate for the call result. Target's explicit
// module-path relation and Host's actor-local boot mapping are resolved while
// sealing, so no hot rule derives a root from a string or a name convention.
type ModuleLoadCall struct {
	schema    *Schema
	key       computationKey
	content   identity.ContentID
	result    Coordinate
	argument  Coordinate
	expected  Value
	fact      Value
	endpoints uint32
	require   vocabulary.Operation
	// call is Call's own coordinate for this mounted occurrence, copied at
	// seal. Call's algebra is the earliest owner of that coordinate; a
	// consumer of this row reads it here instead of resolving the occurrence
	// against Call again.
	call calldomain.CallCoordinate
	// composed is true only when Program's authored Import term was resolved
	// through Module composition. Path/boot projection is deliberately not an
	// export proof for dynamic fresh-result routing.
	composed bool
}

// moduleLoadFactKey is a cold-only factor key. Repeated require calls for the
// same mounted module and authored path reuse one actor-local root reduction
// instead of rescanning Module roots and rejoining the same Value atoms.
type moduleLoadFactKey struct {
	module     identity.ContentID
	path       string
	importTerm keyspace.Term
}

func (schema *Schema) ModuleLoadCall(module, occurrence identity.ContentID) (ModuleLoadCall, bool) {
	if schema == nil || schema.moduleLoadCalls == nil {
		return ModuleLoadCall{}, false
	}
	row, ok := schema.moduleLoadCalls[computationKey{module: module, occurrence: occurrence}]
	return row, ok && row.valid()
}

func (row ModuleLoadCall) valid() bool {
	return row.schema != nil && row.key.module.Available() && row.key.occurrence.Available() &&
		row.content.Available() && row.result.Valid() && row.argument.Valid() && row.require != 0 &&
		row.call.Valid()
}

// Call is the mounted-call coordinate Call published for this occurrence.
func (row ModuleLoadCall) Call() (calldomain.CallCoordinate, bool) {
	if !row.valid() {
		return calldomain.CallCoordinate{}, false
	}
	return row.call, true
}

func (schema *Schema) OwnsModuleLoadCall(row ModuleLoadCall) bool {
	return schema != nil && row.schema == schema && row.valid()
}

func (row ModuleLoadCall) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

func (row ModuleLoadCall) Endpoints() (result, argument Coordinate, ok bool) {
	if !row.valid() {
		return Coordinate{}, Coordinate{}, false
	}
	return row.result, row.argument, true
}

func (row ModuleLoadCall) ExpectedArgument() (Value, bool) {
	if !row.valid() || !row.expected.valid() {
		return Value{}, false
	}
	return row.expected, true
}

func (row ModuleLoadCall) ResultFact() (Value, bool) {
	if !row.valid() || !row.fact.valid() {
		return Value{}, false
	}
	return row.fact, true
}

func (row ModuleLoadCall) RequireOperation() (vocabulary.Operation, bool) {
	if !row.valid() {
		return 0, false
	}
	return row.require, true
}

func (row ModuleLoadCall) CallOccurrence() (module, occurrence identity.ContentID, ok bool) {
	if !row.valid() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	return row.key.module, row.key.occurrence, true
}

// sealModuleLoadRows joins only the existing mounted Call and Value factors.
// Program owns the reusable call-result geometry; the Target normal outcome
// selects result zero and Project/Boundary authenticate the exact require
// operation for each mounted call. The pass never forms a call/result product.
// It runs after literal and exact stored-reference atoms are sealed. The cold
// literal directory is still available, and actor-local module roots can now
// resolve their presealed exact Boot atoms. Both facts are copied into the
// operand so no hot rule reopens Link or Program state.
func (schema *valueBuilder) sealModuleLoadRows() bool {
	if schema == nil || schema.sealProject() == nil || schema.sealBoundary() == nil || schema.sealModule() == nil || schema.moduleLoadCalls == nil {
		return false
	}
	if len(schema.moduleLoadCalls) != 0 {
		return false
	}
	require, hasRequire := schema.sealBoundary().RequireOperation()
	if !hasRequire {
		return true
	}
	contract, contractOK := schema.sealBoundary().Target()
	if !contractOK || contract == nil {
		return false
	}
	if !requireOutcomeResultAvailable(contract, require) {
		return false
	}
	applications := schema.sealProject().Applications().Calls()
	applicationByCall := make(map[computationKey]linkproject.Application, applications.Count())
	for index := 0; index < applications.Count(); index++ {
		application, applicationOK := applications.At(index)
		_, module, call, mountedOK := applications.MountedIdentity(application)
		if !applicationOK || !mountedOK || !module.Available() || !call.Available() {
			return false
		}
		key := computationKey{module: module, occurrence: call}
		if _, duplicate := applicationByCall[key]; duplicate {
			return false
		}
		applicationByCall[key] = application
	}

	// coordinates is keyed by Boundary's portable Value identity, while the
	// mounted directory above is keyed by Program semantic identity. Build the
	// one cold reverse view needed to recover a presealed atom and its literal
	// path from a CoordinateForMountedSemantic result. This is a single pass,
	// never a per-call scan.
	type coordinateProjection struct {
		id  identity.ContentID
		row coordinateRow
	}
	coordinateByIndex := make(map[uint32]coordinateProjection, len(schema.coordinates))
	for id, row := range schema.coordinates {
		if !id.Available() || row.coordinate == 0 {
			return false
		}
		if _, duplicate := coordinateByIndex[row.coordinate]; duplicate {
			return false
		}
		coordinateByIndex[row.coordinate] = coordinateProjection{id: id, row: row}
	}
	for module, mount := range schema.artifacts {
		program := mount.Program.Program
		if !program.Available() {
			return false
		}
		count, countOK := program.CallCount()
		if !countOK {
			return false
		}
		for index := 0; index < count; index++ {
			call, callOK := program.CallAt(index)
			if !callOK || call.Form() != programschema.CallFormPlain || call.ArgumentCount() != 1 {
				continue
			}
			if _, receiver := call.ReceiverID(); receiver {
				continue
			}
			if _, tail := call.TailID(); tail {
				continue
			}
			argument, argumentOK := program.CallArgumentFor(index, 0)
			if !argumentOK || !argument.Available() || argument.CallID() != call.ID() || argument.Index() != 0 || !argument.MemberID().Available() {
				return false
			}
			application, applicationOK := applicationByCall[computationKey{module: module, occurrence: call.ID()}]
			if !applicationOK || !schema.sealBoundary().ApplicationOperationAvailable(contract, application, require) {
				continue
			}
			argumentCoordinate, argumentCoordinateOK := schema.CoordinateForMountedSemantic(module, argument.MemberID())
			if !argumentCoordinateOK {
				return false
			}
			result, resultOK := schema.MountedCallResultSlotFor(module, call.ID(), 0)
			// Statement, open, and multi-result calls have no admitted ordinal-0
			// Value slot this rule may write. The slot projection admits only an
			// existing fixed member or bounded consumer Cell coordinate; it never
			// collapses a producer tail or invents a result coordinate.
			if !resultOK || !schema.OwnsMountedCallResultSlot(result) {
				continue
			}
			resultCoordinate, resultCoordinateOK := result.Coordinate()
			if !resultCoordinateOK {
				return false
			}
			expected := Value{}
			argumentIndex, argumentIndexOK := schema.CoordinateIndex(argumentCoordinate)
			projection, projectionOK := coordinateByIndex[argumentIndex+1]
			if !argumentIndexOK || !projectionOK {
				return false
			}
			if projection.row.atom != 0 {
				expected, _ = schema.Singleton(Atom{schema: schema.Schema, id: projection.row.atom})
			}
			path := ""
			if family, literal, literalOK := schema.sourceLiteralID(projection.id); literalOK && family == keyspace.FamilyString && literal.Kind == keyspace.LiteralString {
				path = literal.String
			}
			fact := Value{}
			composed := false
			importTerm, importFound, importGeometryOK := moduleImportTermForCall(program, call.ID())
			if !importGeometryOK {
				return false
			}
			if importFound {
				// A Program Import has two canonical target authorities. Module
				// composition owns mounted sibling exports; a host-module Import
				// has no composition edge and is resolved by Target's exact
				// initial-root/Host boot relation. Select the authority by the
				// sealed composition edge, never by a widened value or a name
				// fallback after a sibling edge has been admitted. A mounted
				// sibling without that edge is malformed composition, not a host
				// request that may borrow Target by spelling.
				_, composed = schema.sealModule().TargetModuleForImport(module, importTerm)
				if composed {
					fact, _ = schema.moduleLoadFactForImport(module, importTerm)
				} else {
					fact, _ = schema.moduleLoadFact(module, path)
				}
			}
			if !importFound {
				fact, _ = schema.moduleLoadFact(module, path)
			}
			// This pass has already joined the call to Project's own
			// application row, which is exactly what Call's mounted-call
			// directory is built from. An absent coordinate here is a broken
			// join between two sealed authorities, not an operand this rule
			// may interpret with a default.
			coordinate, coordinateOK := schema.callCoordinateForOccurrence(module, call.ID())
			if !coordinateOK {
				return false
			}
			content := computationContent(schema.linkID, "val-callresult-moduleload!", module, call.ID(), uint64(require))
			row := ModuleLoadCall{
				schema: schema.Schema, key: computationKey{module: module, occurrence: call.ID()}, content: content,
				result: resultCoordinate, argument: argumentCoordinate, expected: expected, fact: fact, require: require, composed: composed,
				call: coordinate,
			}
			if !row.valid() {
				return false
			}
			if _, duplicate := schema.moduleLoadCalls[row.key]; duplicate {
				return false
			}
			schema.moduleLoadCalls[row.key] = row
		}
	}
	return true
}

// mountedModuleForPath is the sealed Project-name partition used only to
// distinguish a host-module Import from an unresolved mounted sibling. It
// does not resolve a target; Module composition remains the sole sibling
// authority and Target's initial-root relation remains the sole host
// authority.
func (schema *valueBuilder) mountedModuleForPath(path string) (identity.ContentID, bool) {
	if schema == nil || schema.sealProject() == nil || path == "" {
		return identity.ContentID{}, false
	}
	mounts := schema.sealProject().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		name, nameOK := mounts.Name(shard)
		module, moduleOK := schema.sealProject().ModuleKey(shard)
		if !shardOK || !nameOK || !moduleOK || !module.Available() {
			return identity.ContentID{}, false
		}
		if name == path {
			return module, true
		}
	}
	return identity.ContentID{}, false
}

// moduleImportTermForCall returns the exact Program Import term whose scoped
// require call produces this result. Import order is the canonical Program
// term ordinal used by Module's composition relation; no request string or
// module name is reconstructed here.
func moduleImportTermForCall(program programschema.Program, callID identity.ContentID) (keyspace.Term, bool, bool) {
	if !program.Available() || !callID.Available() {
		return 0, false, false
	}
	count, published := program.ModuleImportCount()
	if !published {
		return 0, false, false
	}
	var term keyspace.Term
	for index := 0; index < count; index++ {
		row, rowOK := program.ModuleImportAt(index)
		candidate := keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1))
		if !rowOK || !row.Available() || candidate == 0 {
			return 0, false, false
		}
		if row.CallID() != callID {
			continue
		}
		if term != 0 {
			// Duplicate Import ownership is malformed canonical geometry. A
			// caller must not turn the ambiguity into a widened module value.
			return 0, false, false
		}
		term = candidate
	}
	return term, term != 0, true
}

// requireOutcomeResultAvailable reports whether the declared required
// operation actually names a bounded normal result at ordinal zero.
//
// A Link that declares no scoped loader has no module-load vertical at all,
// and sealing simply produces no rows. A Link that DOES declare one but whose
// contract cannot produce its result is a different thing: leaving the
// vertical absent there does not leave the program unanalysed, it hands every
// require call to the generic call-result path, which has no module evidence
// and widens to Top. Refusing the seal by name keeps that fabricated Top out
// of the lattice; the malformed contract is reported as a Value seal failure
// instead of being silently absorbed.
func requireOutcomeResultAvailable(target *contract.Contract, require vocabulary.Operation) bool {
	if target == nil || require == 0 {
		return false
	}
	outcomeResult, outcomeResultOK := normalResultID(target, require)
	if !outcomeResultOK || !outcomeResult.Available() {
		return false
	}
	operation, outcome, resultIndex, resultIdentityOK := target.FindOutcomeResultID(outcomeResult)
	kind, values, outcomeOK := target.Operations.OutcomeAt(require, outcome)
	slots, slotsOK := target.Operations.OutcomeValueSlots(require, outcome)
	return resultIdentityOK && operation == require && resultIndex == 0 &&
		outcomeOK && kind == flowkind.OutcomeNormal && values != 0 && slotsOK && slots != 0
}

func normalResultID(target *contract.Contract, require vocabulary.Operation) (identity.ContentID, bool) {
	if target == nil || require == 0 {
		return identity.ContentID{}, false
	}
	for outcome := 0; outcome < target.Operations.OutcomeCount(require); outcome++ {
		kind, values, ok := target.Operations.OutcomeAt(require, outcome)
		if !ok || kind != flowkind.OutcomeNormal {
			continue
		}
		slots, slotsOK := target.Operations.OutcomeValueSlots(require, outcome)
		if !slotsOK || slots == 0 || values == 0 {
			continue
		}
		return target.OutcomeResultID(require, outcome, 0)
	}
	return identity.ContentID{}, false
}

// moduleLoadFact projects one exact Target module root into every actor-local
// Host boot root that can execute the mounted module. Multiple actor contexts
// are joined in Value's existing finite relation; no new actor/product axis is
// introduced.
func (schema *valueBuilder) moduleLoadFact(moduleID identity.ContentID, path string) (Value, bool) {
	if schema == nil || !moduleID.Available() || path == "" || schema.sealModule() == nil || schema.sealHost() == nil || schema.sealBoundary() == nil || schema.moduleFacts == nil {
		return Value{}, false
	}
	// A mounted Project name is a sibling namespace. Its only admissible
	// authority is the exact Module composition row consumed above; Target's
	// initial-root relation cannot be used as a spelling-based fallback for an
	// unresolved sibling Import.
	if _, mountedSibling := schema.mountedModuleForPath(path); mountedSibling {
		return Value{}, false
	}
	cacheKey := moduleLoadFactKey{module: moduleID, path: path}
	if fact, cached := schema.moduleFacts[cacheKey]; cached {
		return fact, fact.valid()
	}
	contract, contractOK := schema.sealBoundary().Target()
	if !contractOK || contract == nil {
		return Value{}, false
	}
	targetRoot, targetRootOK := contract.InitialRootByModulePath(path)
	if !targetRootOK {
		return Value{}, false
	}
	shard, shardOK := schema.sealProject().Mounts().ForModuleKey(moduleID)
	if !shardOK {
		return Value{}, false
	}
	rootCount := schema.sealModule().Roots().ForShardCount(shard)
	if rootCount == 0 {
		return Value{}, false
	}
	shape, shapeOK := contract.InitialRootBootShape(targetRoot)
	initial, initialOK := contract.BootShapeValue(shape)
	if !shapeOK || !initialOK || initial == 0 {
		return Value{}, false
	}
	actors := make(map[linkmodule.Actor]struct{}, rootCount)
	for index := 0; index < rootCount; index++ {
		root, rootOK := schema.sealModule().Roots().ForShardAt(shard, index)
		_, actor, _, mappingOK := schema.sealModule().Roots().Mapping(root)
		if !rootOK || !mappingOK {
			return Value{}, false
		}
		actors[actor] = struct{}{}
	}
	var projected Value
	first := true
	for actor := range actors {
		boot, bootOK := schema.sealHost().BootRoots().For(actor, targetRoot)
		if !bootOK {
			return Value{}, false
		}
		fact, factOK := schema.targetInitialCold(boot, initial)
		if !factOK {
			return Value{}, false
		}
		if first {
			projected, first = fact, false
			continue
		}
		var joinOK bool
		projected, joinOK = schema.Join(projected, fact)
		if !joinOK {
			return Value{}, false
		}
	}
	if first || !projected.valid() {
		return Value{}, false
	}
	schema.moduleFacts[cacheKey] = projected
	return projected, true
}

// moduleLoadFactForImport projects one mounted sibling module's exported root
// table into the existing Value reference atoms. Module's sealed composition
// relation resolves the source Import to the target module; Program's
// ModuleEntryMember.TableID then identifies the exact target table allocation.
// No path lookup, export-name heuristic, or finite fallback is admitted.
func (schema *valueBuilder) moduleLoadFactForImport(moduleID identity.ContentID, importTerm keyspace.Term) (Value, bool) {
	if schema == nil || !moduleID.Available() || keyspace.TermFamily(importTerm) != keyspace.FamilyImport || keyspace.TermOrdinal(importTerm) == 0 ||
		schema.sealModule() == nil || schema.module == nil || schema.heap.LinkContentID() != schema.linkID || schema.moduleFacts == nil {
		return Value{}, false
	}
	cacheKey := moduleLoadFactKey{module: moduleID, importTerm: importTerm}
	if fact, cached := schema.moduleFacts[cacheKey]; cached {
		return fact, fact.valid()
	}
	targetModule, targetOK := schema.module.TargetModuleForImport(moduleID, importTerm)
	if !targetOK {
		return Value{}, false
	}
	mount, mountOK := schema.artifacts[targetModule]
	if !mountOK || !mount.Available() || mount.ModuleKey != targetModule {
		return Value{}, false
	}
	program := mount.Program.Program
	entryCount, entriesOK := program.ModuleEntryCount()
	if !entriesOK {
		return Value{}, false
	}
	issuer, issuerOK := schema.heap.OccurrenceMountForModule(targetModule)
	if !issuerOK {
		return Value{}, false
	}

	// A module can have multiple executable Return entries. Each selected
	// first-level member contributes the same root table allocation for that
	// entry; joining distinct table allocations preserves legitimate return
	// unions without admitting nested table children.
	keys := make(map[heap.Key]struct{})
	for entryIndex := 0; entryIndex < entryCount; entryIndex++ {
		entry, entryOK := program.ModuleEntryAt(entryIndex)
		rootCellOffset, rootCellCount, rootCellSpanOK := entry.RootCellSpan()
		memberOffset, memberCount, spanOK := entry.MemberSpan()
		if !entryOK || !rootCellSpanOK || !spanOK ||
			uint64(rootCellOffset)+uint64(rootCellCount) > uint64(^uint32(0)) ||
			uint64(memberOffset)+uint64(memberCount) > uint64(^uint32(0)) {
			return Value{}, false
		}
		for childIndex := uint32(0); childIndex < rootCellCount; childIndex++ {
			cell, cellOK := program.ModuleEntryRootCellAt(int(rootCellOffset + childIndex))
			if !cellOK || !cell.Available() || cell.EntryID() != entry.ID() {
				return Value{}, false
			}
			// Only the first returned Values position is the module export
			// object. Other root cells are legitimate return values, but cannot
			// establish the require result's table receiver.
			if cell.Position() != 0 {
				continue
			}
			cellKeys, cellProjectionOK := schema.moduleRootTableKeysForCell(targetModule, cell.CellID())
			if !cellProjectionOK {
				return Value{}, false
			}
			for key := range cellKeys {
				keys[key] = struct{}{}
			}
		}
		for childIndex := uint32(0); childIndex < memberCount; childIndex++ {
			member, memberOK := program.ModuleEntryMemberAt(int(memberOffset + childIndex))
			if !memberOK || !member.Available() || member.EntryID() != entry.ID() {
				return Value{}, false
			}
			// Only the first returned Values position is the module export
			// object. Other positions remain valid ModuleEntry geometry, but
			// cannot establish the require result's table receiver.
			if member.Position() != 0 {
				continue
			}
			parentID := member.ParentID()
			tableID := member.TableID()
			// Root members name the returned table itself as both ParentID and
			// TableID. Nested members retain their parent FieldID and are not
			// eligible to establish the module result root.
			if parentID != tableID {
				continue
			}
			key, keyOK := issuer.AllocationRootForOccurrence(tableID)
			module, _, _, kind, _, originOK := schema.heap.AllocationOriginForKey(key)
			if !keyOK || !originOK || module != targetModule || kind != heap.AllocationTable {
				return Value{}, false
			}
			keys[key] = struct{}{}
		}
	}
	if len(keys) == 0 {
		return Value{}, false
	}

	var projected Value
	first := true
	for key := range keys {
		reference := schema.allocRefs[key]
		if reference == 0 {
			return Value{}, false
		}
		for _, role := range []materialization.Role{materialization.Recent, materialization.Summary} {
			atomID, atomOK := schema.referenceAtom(reference, role)
			atom, atomValid := schema.Singleton(Atom{schema: schema.Schema, id: atomID})
			if !atomOK || !atomValid {
				return Value{}, false
			}
			if first {
				projected, first = atom, false
				continue
			}
			var joinOK bool
			projected, joinOK = schema.Join(projected, atom)
			if !joinOK {
				return Value{}, false
			}
		}
	}
	if first || !projected.valid() {
		return Value{}, false
	}
	schema.moduleFacts[cacheKey] = projected
	return projected, true
}

// moduleRootTableKeysForCell is the bounded Value/Heap composition seam for
// a returned module root held in a storage Cell. Program's ModuleEntry only
// authenticates the cell; Value's sealed StorageTransfer names the exact
// persistent write into it, and Heap's AllocationRootValueID names the
// existing Value coordinate of each table allocation. Joining those two
// coordinates recovers only table roots written to this cell. No fresh-root
// application, operation, path, or handwritten arity is consulted.
func (schema *valueBuilder) moduleRootTableKeysForCell(moduleID, cellID identity.ContentID) (map[heap.Key]struct{}, bool) {
	if schema == nil || !moduleID.Available() || !cellID.Available() || schema.heap.LinkContentID() != schema.linkID {
		return nil, false
	}
	cell, cellOK := schema.CoordinateForMountedSemantic(moduleID, cellID)
	cellIndex, cellIndexOK := schema.CoordinateIndex(cell)
	if !cellOK || !cellIndexOK {
		return nil, false
	}

	// Build the existing allocation-root Value-coordinate inverse once for
	// this module. AllocationRootValueID is Heap's canonical root projection;
	// CoordinateForID is the Value owner handoff for that same Boundary Value.
	allocationByCoordinate := make(map[uint32]heap.Key)
	for index := 0; index < schema.heap.AllocationKeyCount(); index++ {
		key, keyOK := schema.heap.AllocationKeyAt(index)
		module, _, _, kind, _, originOK := schema.heap.AllocationOriginForKey(key)
		if !keyOK || !originOK || module != moduleID || kind != heap.AllocationTable {
			continue
		}
		rootID, rootOK := schema.heap.AllocationRootValueID(key)
		rootCoordinate, coordinateOK := schema.CoordinateForID(rootID)
		rootIndex, indexOK := schema.CoordinateIndex(rootCoordinate)
		if !rootOK || !coordinateOK || !indexOK {
			return nil, false
		}
		if prior, duplicate := allocationByCoordinate[rootIndex]; duplicate && prior != key {
			return nil, false
		}
		allocationByCoordinate[rootIndex] = key
	}

	keys := make(map[heap.Key]struct{})
	for index := 0; index < schema.StorageTransferCount(); index++ {
		transfer, transferOK := schema.StorageTransferAt(index)
		module, _, occurrenceOK := transfer.Occurrence()
		if !transferOK || !occurrenceOK || module != moduleID || !transfer.Persistent() {
			continue
		}
		from, to, endpointsOK := transfer.Endpoints()
		toIndex, toIndexOK := schema.CoordinateIndex(to)
		fromIndex, fromIndexOK := schema.CoordinateIndex(from)
		if !endpointsOK || !toIndexOK || !fromIndexOK || toIndex != cellIndex {
			continue
		}
		if key, found := allocationByCoordinate[fromIndex]; found {
			keys[key] = struct{}{}
		}
	}
	return keys, true
}
