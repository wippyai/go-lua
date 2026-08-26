package obligation

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/typestate/statecell"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Cell is one published state-cell row: the coordinate this rule reads the
// resource's state at, and the tag the selection stamps that row with so the
// fold correlates the returned state with the protocol it was selected for.
//
// An operation moves a resource's state and does not move the resource, so
// both endpoints of the row are the one cell.
type Cell struct {
	coordinate statecell.Cell
	tag        uint64
}

// Coordinates are the cell this row reads and the cell it publishes into.
func (cell Cell) Coordinates() (statecell.Cell, statecell.Cell) {
	return cell.coordinate, cell.coordinate
}

// Predicate is the tag this row carries: the protocol handle the cell holds a
// state under.
func (cell Cell) Predicate() uint64 { return cell.tag }

// CellPlan is the state-cell set one mounted call actual reads.
//
// Which cells those are is not a directory anything could enumerate before the
// call's own fact is known: the resource is what the actual's Value fact
// names, and the protocols are what the dispatched operations declare about
// that actual. The plan is therefore published by the operation below rather
// than resolved from a table.
type CellPlan struct {
	rows []Cell
}

// Count is the census of published cell rows.
func (plan CellPlan) Count() int { return len(plan.rows) }

// At returns one cell row at its publication position.
func (plan CellPlan) At(index int) (Cell, bool) {
	if index < 0 || index >= len(plan.rows) {
		return Cell{}, false
	}
	return plan.rows[index], true
}

// StateCellCount and StateCellAt are the direct composition accessors of one
// derived cell plan.
func StateCellCount(plan CellPlan) int { return plan.Count() }

// StateCellAt returns one cell row of a derived plan.
func StateCellAt(plan CellPlan, index int) (Cell, bool) { return plan.At(index) }

// DeriveStateCells is the operation that publishes the state-cell rows of one
// mounted call actual: for every allocation root the actual's Value fact
// reaches, the cell that root holds its state in under each protocol the
// dispatched operations declare an obligation about at this position.
//
// The space is the axis's own sealed coordinate space and every cell comes out
// of it, so a coordinate this rule publishes at is one that space issued. A
// value that reaches no allocation root, and a call actual no protocol speaks
// about, both publish no row: the absence of an obligation is not a cell.
func (judgment Judgment) DeriveStateCells(
	heap heapdomain.Schema,
	space statecell.Space,
	candidate valuedomain.MountedCallArgument,
	argument valuedomain.Value,
	dispatched calldomain.Value,
) (CellPlan, bool) {
	if !judgment.Valid() || !judgment.values.OwnsMountedCallArgument(candidate) || !space.Available() {
		return CellPlan{}, false
	}
	actual, actualOK := candidate.ActualIndex()
	if !actualOK {
		return CellPlan{}, false
	}
	protocols, protocolsOK := judgment.governedProtocols(dispatched, actual)
	if !protocolsOK {
		return CellPlan{}, false
	}
	if len(protocols) == 0 {
		return CellPlan{}, true
	}
	allocations, allocationsOK := judgment.allocationsOf(heap, argument)
	if !allocationsOK {
		return CellPlan{}, false
	}
	plan := CellPlan{}
	for _, allocation := range allocations {
		for _, protocol := range protocols {
			coordinate, coordinateOK := space.Cell(allocation, protocol)
			if !coordinateOK {
				return CellPlan{}, false
			}
			plan.rows = append(plan.rows, Cell{coordinate: coordinate, tag: uint64(protocol)})
		}
	}
	return plan, true
}

// governedProtocols are the protocols that declare an obligation about this
// actual at any alternative the call dispatches to, in sealed declaration
// order and without repetition.
//
// A callee the analysis cannot follow declares nothing, so it contributes no
// protocol here. That is not the call being dropped: the fold still answers
// for the candidate, and it answers it with the escape the judgment applies to
// whatever cell the site's other alternatives named.
func (judgment Judgment) governedProtocols(dispatched calldomain.Value, actual uint32) ([]vocabulary.Protocol, bool) {
	seen := make(map[vocabulary.Protocol]struct{})
	governed := make([]vocabulary.Protocol, 0, 2)
	for index := 0; index < dispatched.KnownTargetCount(); index++ {
		target, targetOK := dispatched.KnownTargetAt(index)
		if !targetOK {
			return nil, false
		}
		operation, kind := judgment.calls.ClassifyTargetOperation(target)
		switch kind {
		case calldomain.TargetOperationNone:
			continue
		case calldomain.TargetOperationPresent:
		default:
			return nil, false
		}
		for _, protocol := range judgment.sealed.protocolsAt(operation, actual) {
			if _, duplicate := seen[protocol]; duplicate {
				continue
			}
			seen[protocol] = struct{}{}
			governed = append(governed, protocol)
		}
	}
	return governed, true
}

// allocationsOf are the dense allocation-root ordinals one Value fact reaches,
// in Heap's own numbering and without repetition.
//
// The roots are read out of the fact's own atoms through the Value schema that
// issued them, so this resolves no reference it was not handed and mints no
// key. An atom that reaches no allocation root contributes nothing.
func (judgment Judgment) allocationsOf(heap heapdomain.Schema, argument valuedomain.Value) ([]int, bool) {
	seen := make(map[int]struct{})
	roots := make([]int, 0, 2)
	for index := 0; index < judgment.values.ValueAtomCount(argument); index++ {
		atom, atomOK := judgment.values.ValueAtomAt(argument, index)
		if !atomOK {
			return nil, false
		}
		reference, _, referenceOK := atom.Reference()
		if !referenceOK || !judgment.values.OwnsReference(reference) {
			continue
		}
		key, keyOK := reference.AllocationKey()
		if !keyOK || !key.Valid() || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		ordinal, ordinalOK := heap.AllocationKeyIndex(key)
		if !ordinalOK {
			return nil, false
		}
		if _, duplicate := seen[ordinal]; duplicate {
			continue
		}
		seen[ordinal] = struct{}{}
		roots = append(roots, ordinal)
	}
	return roots, true
}
