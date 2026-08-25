// Package fresh owns Heap's Target-derived fresh-root denominator.
//
// The catalog is deliberately independent of Heap. It consumes the exact
// sealed Link Project/Boundary/Target authorities once, then publishes only
// scalar fresh-root rows. Heap remains the issuer of the Key that addresses a
// row in this catalog.
package fresh

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	targetcontract "github.com/wippyai/go-lua/analysis/program/target/contract"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

const maxUint32 = uint64(^uint32(0))

// Root is one exact Target fresh-result coordinate selected by a mounted
// ordinary Call application. ApplicationID is the existing Project
// application identity; OutcomeResultID is the exact Target identity joined
// to the canonical Program CallResult admission. Ordinal is the fresh
// occurrence within that identity. Kinds is the Target fresh-class projection
// used by Heap's runtime-kind lattice.
//
// Root contains no Heap key or dense Heap selector. The parent Heap package
// issues those coordinates and owns their content identity.
type Root struct {
	ApplicationID   identity.ContentID
	OutcomeResultID identity.ContentID
	Ordinal         uint32
	Kinds           runtimekind.Set
}

// Available reports whether the row is a usable fresh-root projection.
func (root Root) Available() bool {
	return root.ApplicationID.Available() && root.OutcomeResultID.Available() && root.Kinds != 0 && root.Kinds.Valid() && root.Kinds&^runtimekind.NonNil == 0
}

type templateKey struct {
	outcomeResultID identity.ContentID
	ordinal         uint32
}

type template struct {
	templateKey
}

type set struct {
	start uint32
	end   uint32
}

type setValue struct {
	template uint32
	kinds    runtimekind.Set
}

// MountedProgram is the neutral seal-time projection Heap supplies from its
// artifact mounts. It carries the canonical immutable Program publication and
// the concrete module key that places it in the Link. The projection contains
// no Heap mount authority and no copied CallResult rows; Build indexes the
// Program's existing CallResult family once for its cold admission walk.
type MountedProgram struct {
	Module  identity.ContentID
	Program programschema.Program
}

func (mounted MountedProgram) available() bool {
	return mounted.Module.Available() && mounted.Program.Available()
}

type mountedCallKey struct {
	module identity.ContentID
	call   identity.ContentID
}

type callResultAdmissionShape struct {
	form         programschema.CallResultForm
	multiplicity programschema.CallResultMultiplicity
	count        uint32
}

// Catalog is the immutable, factorized fresh-root denominator. Templates are
// interned by Target OutcomeResultID plus fresh ordinal. Each admitted
// Project application points at one sorted, interned set of templates, and
// offsets turn that relation into one dense root order without retaining an
// Application×template product.
type Catalog struct {
	templates       []template
	templateBy      map[templateKey]uint32
	sets            []set
	setValues       []setValue
	applications    []identity.ContentID
	applicationSets []uint32
	offsets         []uint64
	count           uint64
}

// Build seals the fresh-root catalog from one exact Link and its owner-issued
// mounted Program projections. A nil or malformed source fails closed.
// CallResult is the only admission path for an ordinary Call's Target result
// identity: Project application membership and Target OutcomeResultID
// coordinates are joined with the canonical Program CallResult row, whose
// AdmitsResult law decides the admitted ordinal. The row is read from the
// mounted Program publication and is not rebuilt as a Link-level relation.
func Build(source *link.Link, mounted []MountedProgram) (*Catalog, bool) {
	if source == nil || source.Project() == nil || source.Boundary() == nil {
		return nil, false
	}
	project := source.Project()
	boundary := source.Boundary()
	contract, contractOK := boundary.Target()
	if !contractOK || contract == nil {
		return nil, false
	}
	callResults, mountsOK := indexMountedCallResults(project, mounted)
	if !mountsOK {
		return nil, false
	}

	catalog := &Catalog{templateBy: make(map[templateKey]uint32)}
	if !catalog.internTargetTemplates(contract) {
		return nil, false
	}
	if len(catalog.templates) == 0 {
		return catalog, true
	}

	applications := project.Applications().Calls()
	selectedByShape := make(map[callResultAdmissionShape][]setValue)
	for applicationIndex := 0; applicationIndex < applications.Count(); applicationIndex++ {
		application, applicationOK := applications.At(applicationIndex)
		applicationID, moduleID, callID, mountedOK := applications.MountedIdentity(application)
		if !applicationOK || !mountedOK || !applicationID.Available() || !moduleID.Available() || !callID.Available() {
			return nil, false
		}

		selected, selectedOK := catalog.authenticatedApplication(contract, callResults, selectedByShape, applicationID, moduleID, callID)
		if !selectedOK {
			return nil, false
		}
		if len(selected) == 0 {
			continue
		}
		setID, setOK := catalog.internSet(selected)
		if !setOK || setID == 0 || uint64(len(catalog.applications)) >= maxUint32 {
			return nil, false
		}
		catalog.applications = append(catalog.applications, applicationID)
		catalog.applicationSets = append(catalog.applicationSets, setID)
		if catalog.count > ^uint64(0)-uint64(len(selected)) {
			return nil, false
		}
		catalog.count += uint64(len(selected))
		catalog.offsets = append(catalog.offsets, catalog.count)
	}
	// Every counted index is proved here, once, so membership below is the
	// range test and no reader has to materialize a Root to learn that one
	// exists.
	for index := uint64(0); index < catalog.count; index++ {
		if _, ok := catalog.At(index); !ok {
			return nil, false
		}
	}
	return catalog, true
}

// Has reports whether index addresses a fresh root. Build proved every index
// it counted, so membership is the count.
func (catalog *Catalog) Has(index uint64) bool {
	return catalog != nil && index < catalog.count
}

func (catalog *Catalog) internTargetTemplates(contract *targetcontract.Contract) bool {
	if catalog == nil || contract == nil || catalog.templateBy == nil {
		return false
	}
	operations := contract.Operations.OperationCount()
	if operations < 0 || uint64(operations) > maxUint32 {
		return false
	}
	for operationIndex := 0; operationIndex < operations; operationIndex++ {
		operation, operationOK := contract.Operations.OperationAt(operationIndex)
		if !operationOK {
			return false
		}
		outcomes := contract.Operations.OutcomeCount(operation)
		if outcomes < 0 || uint64(outcomes) > maxUint32 {
			return false
		}
		for outcome := 0; outcome < outcomes; outcome++ {
			freshCount := contract.Operations.FreshResultCount(operation, outcome)
			if freshCount < 0 || uint64(freshCount) > maxUint32 {
				return false
			}
			for freshIndex := 0; freshIndex < freshCount; freshIndex++ {
				result, ordinal, kind, freshOK := contract.Operations.FreshResultAt(operation, outcome, freshIndex)
				if !freshOK || uint64(outcome) > maxUint32 {
					return false
				}
				if _, heapKind := freshRootKinds(kind); !heapKind {
					continue
				}
				outcomeResultID, outcomeResultOK := contract.OutcomeResultID(operation, outcome, int(result))
				if !outcomeResultOK || !outcomeResultID.Available() {
					return false
				}
				key := templateKey{outcomeResultID: outcomeResultID, ordinal: ordinal}
				if catalog.templateBy[key] != 0 {
					continue
				}
				if uint64(len(catalog.templates)) >= maxUint32 {
					return false
				}
				catalog.templates = append(catalog.templates, template{templateKey: key})
				catalog.templateBy[key] = uint32(len(catalog.templates))
			}
		}
	}
	return true
}

// indexMountedCallResults validates the exact Project mount substitution and
// indexes each canonical Program CallResult once by its concrete module and
// authored Call identity. The index is a cold-build accelerator, not a second
// schema plane: every value is the immutable row already owned by Program.
func indexMountedCallResults(project *linkproject.Component, mounted []MountedProgram) (map[mountedCallKey]programschema.CallResult, bool) {
	if project == nil || len(mounted) != project.Mounts().Count() {
		return nil, false
	}
	callResults := make(map[mountedCallKey]programschema.CallResult)
	seenModules := make(map[identity.ContentID]struct{}, len(mounted))
	for index, mountedProgram := range mounted {
		if !mountedProgram.available() {
			return nil, false
		}
		if _, duplicate := seenModules[mountedProgram.Module]; duplicate {
			return nil, false
		}
		seenModules[mountedProgram.Module] = struct{}{}
		shard, shardOK := project.Mounts().At(index)
		module, moduleOK := project.ModuleKey(shard)
		programID, programIDOK := project.Mounts().ProgramID(shard)
		if !shardOK || !moduleOK || !programIDOK || module != mountedProgram.Module || programID != mountedProgram.Program.ProgramID {
			return nil, false
		}

		program := mountedProgram.Program
		callCount, callsPublished := program.CallCount()
		if !callsPublished || callCount < 0 {
			return nil, false
		}
		callIDs := make(map[identity.ContentID]struct{}, callCount)
		for callIndex := 0; callIndex < callCount; callIndex++ {
			call, callOK := program.CallAt(callIndex)
			callID := call.ID()
			if !callOK || !call.Available() || !callID.Available() {
				return nil, false
			}
			if _, duplicate := callIDs[callID]; duplicate {
				return nil, false
			}
			callIDs[callID] = struct{}{}
		}

		resultCount, resultsPublished := program.CallResultCount()
		if !resultsPublished || resultCount < 0 {
			return nil, false
		}
		for resultIndex := 0; resultIndex < resultCount; resultIndex++ {
			result, resultOK := program.CallResultAt(resultIndex)
			callID := result.CallID()
			if !resultOK || !result.Available() || !callID.Available() {
				return nil, false
			}
			if _, callExists := callIDs[callID]; !callExists {
				return nil, false
			}
			key := mountedCallKey{module: mountedProgram.Module, call: callID}
			if _, duplicate := callResults[key]; duplicate {
				return nil, false
			}
			callResults[key] = result
		}
	}
	return callResults, true
}

// authenticatedApplication visits Target fresh rows and authenticates each
// corresponding canonical Program CallResult. Project's exact ordinary-call
// application supplies the module/call key; Target supplies the exact
// OutcomeResultID and coordinates; CallResult.AdmitsResult supplies the only
// output-ordinal admission law. Selected template sets are cached by their
// target-independent CallResult shape, so repeated applications do not
// rescan the Target rows.
func (catalog *Catalog) authenticatedApplication(contract *targetcontract.Contract, callResults map[mountedCallKey]programschema.CallResult, selectedByShape map[callResultAdmissionShape][]setValue, applicationID, moduleID, callID identity.ContentID) ([]setValue, bool) {
	if catalog == nil || contract == nil || !applicationID.Available() || !moduleID.Available() || !callID.Available() {
		return nil, false
	}
	callResult, callResultOK := callResults[mountedCallKey{module: moduleID, call: callID}]
	if !callResultOK {
		return nil, true
	}
	if !callResult.Available() || callResult.CallID() != callID {
		return nil, false
	}
	shape := callResultAdmissionShape{form: callResult.Form(), multiplicity: callResult.Multiplicity()}
	if count, countOK := callResult.ResultCount(); countOK {
		shape.count = count
	}
	if selected, cached := selectedByShape[shape]; cached {
		return selected, true
	}
	selectedByTemplate := make(map[uint32]runtimekind.Set)
	operations := contract.Operations.OperationCount()
	for operationIndex := 0; operationIndex < operations; operationIndex++ {
		operation, operationOK := contract.Operations.OperationAt(operationIndex)
		if !operationOK {
			return nil, false
		}
		outcomes := contract.Operations.OutcomeCount(operation)
		for outcome := 0; outcome < outcomes; outcome++ {
			freshCount := contract.Operations.FreshResultCount(operation, outcome)
			for freshIndex := 0; freshIndex < freshCount; freshIndex++ {
				result, ordinal, kind, freshOK := contract.Operations.FreshResultAt(operation, outcome, freshIndex)
				if !freshOK {
					return nil, false
				}
				mask, heapKind := freshRootKinds(kind)
				if !heapKind {
					continue
				}
				outcomeResultID, outcomeResultOK := contract.OutcomeResultID(operation, outcome, int(result))
				if !outcomeResultOK || !outcomeResultID.Available() {
					return nil, false
				}
				rowOperation, rowOutcome, rowResult, targetOK := contract.FindOutcomeResultID(outcomeResultID)
				if !targetOK || rowOperation != operation || rowOutcome != outcome || rowResult != int(result) {
					return nil, false
				}
				if !callResult.AdmitsResult(uint32(result)) {
					continue
				}
				templateID := catalog.templateBy[templateKey{outcomeResultID: outcomeResultID, ordinal: ordinal}]
				if templateID == 0 {
					return nil, false
				}
				selectedByTemplate[templateID] |= mask
			}
		}
	}
	selected := make([]setValue, 0, len(selectedByTemplate))
	for templateID, kinds := range selectedByTemplate {
		if templateID == 0 || uint64(templateID) > uint64(len(catalog.templates)) || kinds == 0 || !kinds.Valid() || kinds&^runtimekind.NonNil != 0 {
			return nil, false
		}
		selected = append(selected, setValue{template: templateID, kinds: kinds})
	}
	sort.Slice(selected, func(left, right int) bool { return selected[left].template < selected[right].template })
	selectedByShape[shape] = selected
	return selected, true
}

// Count returns the exact dense fresh-root denominator.
func (catalog *Catalog) Count() uint64 {
	if catalog == nil {
		return 0
	}
	return catalog.count
}

// At returns the fresh root at its stable catalog order. The index is local
// to the fresh interval; Heap adds its physical Program-root offset when it
// issues a Key.
func (catalog *Catalog) At(index uint64) (Root, bool) {
	if catalog == nil || index >= catalog.count || len(catalog.offsets) != len(catalog.applications) || len(catalog.offsets) != len(catalog.applicationSets) {
		return Root{}, false
	}
	applicationIndex := sort.Search(len(catalog.offsets), func(position int) bool { return catalog.offsets[position] > index })
	if applicationIndex >= len(catalog.applications) || applicationIndex >= len(catalog.applicationSets) {
		return Root{}, false
	}
	start := uint64(0)
	if applicationIndex > 0 {
		start = catalog.offsets[applicationIndex-1]
	}
	setID := catalog.applicationSets[applicationIndex]
	if setID == 0 || uint64(setID) > uint64(len(catalog.sets)) {
		return Root{}, false
	}
	setRow := catalog.sets[setID-1]
	local := index - start
	if local >= uint64(setRow.end-setRow.start) || uint64(setRow.start)+local >= uint64(len(catalog.setValues)) {
		return Root{}, false
	}
	value := catalog.setValues[setRow.start+uint32(local)]
	if value.template == 0 || uint64(value.template) > uint64(len(catalog.templates)) {
		return Root{}, false
	}
	templateRow := catalog.templates[value.template-1]
	root := Root{ApplicationID: catalog.applications[applicationIndex], OutcomeResultID: templateRow.outcomeResultID, Ordinal: templateRow.ordinal, Kinds: value.kinds}
	return root, root.Available()
}

func (catalog *Catalog) internSet(values []setValue) (uint32, bool) {
	if catalog == nil || len(values) == 0 || uint64(len(values)) > maxUint32 {
		return 0, false
	}
	for index, candidate := range catalog.sets {
		if uint64(candidate.end-candidate.start) != uint64(len(values)) || uint64(candidate.end) > uint64(len(catalog.setValues)) {
			continue
		}
		equal := true
		for offset, value := range values {
			if catalog.setValues[candidate.start+uint32(offset)] != value {
				equal = false
				break
			}
		}
		if equal {
			return uint32(index + 1), true
		}
	}
	start := uint64(len(catalog.setValues))
	end := start + uint64(len(values))
	if end > maxUint32 || uint64(len(catalog.sets)) >= maxUint32 {
		return 0, false
	}
	catalog.setValues = append(catalog.setValues, values...)
	catalog.sets = append(catalog.sets, set{start: uint32(start), end: uint32(end)})
	return uint32(len(catalog.sets)), true
}

// KindsFor maps Target's closed portable FreshClass vocabulary to the
// runtime-kind set Heap needs. Error and reflection intentionally share the
// conservative Userdata family; an unavailable class is never admitted.
func KindsFor(kind schematype.FreshClass) (runtimekind.Set, bool) {
	return freshRootKinds(kind)
}

func freshRootKinds(kind schematype.FreshClass) (runtimekind.Set, bool) {
	switch kind {
	case schematype.FreshClassTable:
		return runtimekind.Bit(runtimekind.Table), true
	case schematype.FreshClassFunction:
		return runtimekind.Bit(runtimekind.Function), true
	case schematype.FreshClassThread:
		return runtimekind.Bit(runtimekind.Thread), true
	case schematype.FreshClassUserdata:
		return runtimekind.Bit(runtimekind.Userdata), true
	case schematype.FreshClassError, schematype.FreshClassReflection:
		return runtimekind.Bit(runtimekind.Userdata), true
	default:
		return 0, false
	}
}
