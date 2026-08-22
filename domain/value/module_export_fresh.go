package value

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/heap"
)

// moduleExportFreshRow is Value's owner-issued proof that one Target fresh
// result is the exact result of a composed module Import. The operation is
// the authenticated Call seed; roots are the finite union of exported table
// allocations proved by ModuleEntry and the sealed Value storage transfer.
type moduleExportFreshRow struct {
	operation vocabulary.Operation
	roots     []heap.Key
}

// ModuleExportFreshOperation returns the exact operation authenticated for a
// composed module export fresh result. Missing rows are intentional: generic
// fresh results retain Heap's conservative unknown route.
func (schema *Schema) ModuleExportFreshOperation(key heap.Key) (vocabulary.Operation, bool) {
	if schema == nil || !schema.Valid() || !schema.heap.OwnsKey(key) || schema.moduleExportFresh == nil {
		return 0, false
	}
	row, ok := schema.moduleExportFresh[key]
	return row.operation, ok && row.operation != 0 && len(row.roots) != 0
}

// ModuleExportFreshRootCount reports the exact finite table-root union for a
// composed module export fresh result. It never reports roots for ordinary or
// unresolved fresh calls.
func (schema *Schema) ModuleExportFreshRootCount(key heap.Key) int {
	if schema == nil || !schema.Valid() || !schema.heap.OwnsKey(key) || schema.moduleExportFresh == nil {
		return 0
	}
	row, ok := schema.moduleExportFresh[key]
	if !ok || row.operation == 0 {
		return 0
	}
	return len(row.roots)
}

// ModuleExportFreshRootAt returns one owner-fenced exported table root in the
// canonical content order retained by Value. The Heap key remains the sole
// allocation identity; Value only publishes this sealed composition proof.
func (schema *Schema) ModuleExportFreshRootAt(key heap.Key, index int) (heap.Key, bool) {
	if schema == nil || !schema.Valid() || !schema.heap.OwnsKey(key) || schema.moduleExportFresh == nil {
		return heap.Key{}, false
	}
	row, ok := schema.moduleExportFresh[key]
	if !ok || row.operation == 0 || index < 0 || index >= len(row.roots) {
		return heap.Key{}, false
	}
	root := row.roots[index]
	return root, schema.heap.OwnsKey(root) && root.Kind() == heap.RootAllocation
}

// sealModuleExportFreshRows joins only the composed ModuleLoadCall rows to
// their existing Target FreshResultCall keys. FreshResultCall's Target
// operation is the possible fresh-producing operation at that CallResult
// coordinate; it is not the scoped loader operation that authenticated the
// authored Import. The exact mounted application identity is therefore the
// canonical join, with the existing result coordinate as a second geometry
// fence. A row is admitted only when every matching composed load has a
// complete table-root fact; an absent or non-table alternative leaves the
// fresh result on the generic unknown path. The pass creates no new Value
// coordinate and no selected-operation×root product.
func (schema *valueBuilder) sealModuleExportFreshRows() bool {
	if schema == nil || schema.Schema == nil || schema.sealProject() == nil || schema.moduleExportFresh == nil || len(schema.moduleExportFresh) != 0 || schema.moduleLoadCalls == nil || schema.freshResultCalls == nil {
		return false
	}

	// Project's mounted application directory is the sole exact bridge from
	// Value's (module, authored Call) computation key to the Call application
	// that owns each fresh root. No path or operation scan can substitute for
	// this identity join.
	applications := schema.sealProject().Applications().Calls()
	applicationByCall := make(map[computationKey]identity.ContentID, applications.Count())
	for index := 0; index < applications.Count(); index++ {
		application, applicationOK := applications.At(index)
		applicationID, module, call, mountedOK := applications.MountedIdentity(application)
		if !applicationOK || !mountedOK || !applicationID.Available() || !module.Available() || !call.Available() {
			return false
		}
		key := computationKey{module: module, occurrence: call}
		if prior, duplicate := applicationByCall[key]; duplicate && prior != applicationID {
			return false
		}
		applicationByCall[key] = applicationID
	}

	loads := make(map[identity.ContentID][]ModuleLoadCall)
	for _, load := range schema.moduleLoadCalls {
		if !schema.OwnsModuleLoadCall(load) || !load.composed {
			continue
		}
		applicationID, applicationOK := applicationByCall[load.key]
		if !applicationOK || !applicationID.Available() {
			return false
		}
		loads[applicationID] = append(loads[applicationID], load)
	}

	for key, fresh := range schema.freshResultCalls {
		if !schema.OwnsFreshResultCall(fresh) {
			return false
		}
		_, operationOK := fresh.Operation()
		applicationID, applicationOK := fresh.ApplicationID()
		coordinate, coordinateOK := fresh.Coordinate()
		coordinateIndex, indexOK := schema.CoordinateIndex(coordinate)
		if !operationOK || !applicationOK || !coordinateOK || !indexOK {
			return false
		}
		candidates := loads[applicationID]
		if len(candidates) == 0 {
			continue
		}

		roots := make(map[heap.Key]struct{})
		complete := true
		var loader vocabulary.Operation
		for _, candidate := range candidates {
			result, _, resultOK := candidate.Endpoints()
			candidateCoordinateIndex, candidateIndexOK := schema.CoordinateIndex(result)
			candidateOperation, candidateOperationOK := candidate.RequireOperation()
			if !resultOK || !candidateIndexOK || candidateCoordinateIndex != coordinateIndex || !candidateOperationOK {
				complete = false
				break
			}
			if loader == 0 {
				loader = candidateOperation
			} else if loader != candidateOperation {
				complete = false
				break
			}
			fact, factOK := candidate.ResultFact()
			if !factOK {
				complete = false
				break
			}
			visited := schema.VisitAtoms(fact, func(atom Atom) bool {
				reference, _, rooted := atom.Reference()
				if !rooted || reference.Kind() != ReferenceTable {
					complete = false
					return false
				}
				root, rootOK := reference.AllocationKey()
				module, _, _, kind, _, originOK := schema.heap.AllocationOriginForKey(root)
				if !rootOK || !originOK || module == (identity.ContentID{}) || kind != heap.AllocationTable {
					complete = false
					return false
				}
				roots[root] = struct{}{}
				return true
			})
			if !visited || !complete {
				complete = false
				break
			}
		}
		if !complete || len(roots) == 0 {
			continue
		}
		ordered := make([]heap.Key, 0, len(roots))
		for root := range roots {
			ordered = append(ordered, root)
		}
		sort.Slice(ordered, func(left, right int) bool {
			leftID, leftOK := ordered[left].ContentID()
			rightID, rightOK := ordered[right].ContentID()
			if !leftOK || !rightOK {
				return leftOK && !rightOK
			}
			return bytes.Compare(leftID[:], rightID[:]) < 0
		})
		if loader == 0 {
			continue
		}
		schema.moduleExportFresh[key] = moduleExportFreshRow{operation: loader, roots: ordered}
	}
	return true
}
