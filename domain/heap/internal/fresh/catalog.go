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
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	targetcontract "github.com/wippyai/go-lua/analysis/program/target/contract"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

const maxUint32 = uint64(^uint32(0))

// Root is one exact Target fresh-result coordinate selected by a mounted
// ordinary Call application. ApplicationID is the existing Project
// application identity; OutcomeResultID is the exact Target identity already
// authenticated by Boundary.CallResult. Ordinal is the fresh occurrence
// within that identity. Kinds is the Target fresh-class projection used by
// Heap's runtime-kind lattice.
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

// Build seals the fresh-root catalog from one exact Link. A nil or malformed
// source fails closed. CallResult is the only admission path for an ordinary
// Call's Target result identity: ApplicationOperationAvailable alone is not
// sufficient because statement calls and unconsumed fixed/open result tails
// do not issue Heap fresh roots.
func Build(source *link.Link) (*Catalog, bool) {
	if source == nil || source.Project() == nil || source.Boundary() == nil {
		return nil, false
	}
	project := source.Project()
	boundary := source.Boundary()
	contract, contractOK := boundary.Target()
	if !contractOK || contract == nil {
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
	callResults := boundary.Calls()
	for applicationIndex := 0; applicationIndex < applications.Count(); applicationIndex++ {
		application, applicationOK := applications.At(applicationIndex)
		applicationID, moduleID, callID, mountedOK := applications.MountedIdentity(application)
		if !applicationOK || !mountedOK || !applicationID.Available() || !moduleID.Available() || !callID.Available() {
			return nil, false
		}

		selected, selectedOK := catalog.authenticatedApplication(contract, callResults, applicationID, moduleID, callID)
		if !selectedOK {
			return nil, false
		}
		if len(selected) == 0 {
			continue
		}
		values := make([]setValue, 0, len(selected))
		for templateID, kinds := range selected {
			if templateID == 0 || uint64(templateID) > uint64(len(catalog.templates)) || kinds == 0 || !kinds.Valid() || kinds&^runtimekind.NonNil != 0 {
				return nil, false
			}
			values = append(values, setValue{template: templateID, kinds: kinds})
		}
		sort.Slice(values, func(left, right int) bool { return values[left].template < values[right].template })
		setID, setOK := catalog.internSet(values)
		if !setOK || setID == 0 || uint64(len(catalog.applications)) >= maxUint32 {
			return nil, false
		}
		catalog.applications = append(catalog.applications, applicationID)
		catalog.applicationSets = append(catalog.applicationSets, setID)
		if catalog.count > ^uint64(0)-uint64(len(values)) {
			return nil, false
		}
		catalog.count += uint64(len(values))
		catalog.offsets = append(catalog.offsets, catalog.count)
	}
	return catalog, true
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

// authenticatedApplication visits the Target fresh rows and asks Boundary to
// authenticate each corresponding mounted CallResult. A valid CallResult
// proves the exact application, module, call, and Target result identity; no
// Project/Target membership approximation is accepted here.
func (catalog *Catalog) authenticatedApplication(contract *targetcontract.Contract, callResults linkboundary.Calls, applicationID, moduleID, callID identity.ContentID) (map[uint32]runtimekind.Set, bool) {
	if catalog == nil || contract == nil || !applicationID.Available() || !moduleID.Available() || !callID.Available() {
		return nil, false
	}
	selected := make(map[uint32]runtimekind.Set)
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
				callResult, callResultOK := callResults.CallResult(moduleID, callID, outcomeResultID)
				if !callResultOK {
					continue
				}
				rowApplicationID, applicationOK := callResult.ApplicationID()
				rowModuleID, moduleOK := callResult.ModuleID()
				rowCallID, callOK := callResult.CallID()
				rowOutcomeResultID, outcomeResultIDOK := callResult.OutcomeResultID()
				rowOperation, rowOutcome, rowResult, coordinatesOK := callResult.OutcomeResult()
				if !applicationOK || !moduleOK || !callOK || !outcomeResultIDOK || !coordinatesOK || rowApplicationID != applicationID || rowModuleID != moduleID || rowCallID != callID || rowOutcomeResultID != outcomeResultID || rowOperation != operation || rowOutcome != outcome || rowResult != int(result) {
					return nil, false
				}
				templateID := catalog.templateBy[templateKey{outcomeResultID: outcomeResultID, ordinal: ordinal}]
				if templateID == 0 {
					return nil, false
				}
				selected[templateID] |= mask
			}
		}
	}
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
