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
)

// ModuleLoadCall is Value's sealed interpretation of one mounted call-result
// coordinate. The call and argument are exact existing factors; the write is
// the already-issued Value coordinate for the call result. Target's explicit
// module-path relation and Host's actor-local boot mapping are resolved while
// sealing, so no hot rule derives a root from a string or a name convention.
type ModuleLoadCall struct {
	schema   *Schema
	key      computationKey
	content  identity.ContentID
	result   Coordinate
	argument Coordinate
	expected Value
	fact     Value
	require  vocabulary.Operation
}

// moduleLoadFactKey is a cold-only factor key. Repeated require calls for the
// same mounted module and authored path reuse one actor-local root reduction
// instead of rescanning Module roots and rejoining the same Value atoms.
type moduleLoadFactKey struct {
	module identity.ContentID
	path   string
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
		row.content.Available() && row.result.Valid() && row.argument.Valid() && row.require != 0
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
	outcomeResult, outcomeResultOK := normalResultID(contract, require)
	if !outcomeResultOK || !outcomeResult.Available() {
		// A malformed/empty require outcome cannot produce a bounded result
		// coordinate. Leave the vertical absent rather than fabricating one.
		return true
	}
	operation, outcome, resultIndex, resultIdentityOK := contract.FindOutcomeResultID(outcomeResult)
	kind, values, outcomeOK := contract.Operations.OutcomeAt(require, outcome)
	slots, slotsOK := contract.Operations.OutcomeValueSlots(require, outcome)
	if !resultIdentityOK || operation != require || resultIndex != 0 || !outcomeOK || kind != flowkind.OutcomeNormal || values == 0 || !slotsOK || slots == 0 {
		return true
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
		snapshot := mount.Snapshot()
		if snapshot == nil || !snapshot.Available() {
			return false
		}
		program := snapshot.Program()
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
			fact, _ := schema.moduleLoadFact(module, path)
			content := computationContent(schema.linkID, "val-callresult-moduleload!", module, call.ID(), uint64(require))
			row := ModuleLoadCall{
				schema: schema.Schema, key: computationKey{module: module, occurrence: call.ID()}, content: content,
				result: resultCoordinate, argument: argumentCoordinate, expected: expected, fact: fact, require: require,
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
