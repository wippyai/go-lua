// Package calltarget owns closure-allocation to callable-boundary joins.
// It consumes only immutable Flow data and already-canonical compiler bundles;
// the parent maps the compact fault once and retains the returned target rows.
package calltarget

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/allocation"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/bodyboundary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
)

// Reason is the deliberately small failure vocabulary needed by this join.
// The parent compiler owns translation into its diagnostic vocabulary.
type Reason uint8

const (
	ReasonUnavailable Reason = iota + 1
	ReasonDuplicate
)

// Fault identifies the allocation/body row that prevented target construction.
type Fault struct {
	reason Reason
	row    int
	subrow int
	failed bool
}

func (fault Fault) Failed() bool   { return fault.failed }
func (fault Fault) Reason() Reason { return fault.reason }
func (fault Fault) Row() int       { return fault.row }
func (fault Fault) Subrow() int    { return fault.subrow }

func failure(reason Reason, row, subrow int) Fault {
	return Fault{reason: reason, row: row, subrow: subrow, failed: true}
}

// Input is the complete immutable boundary for target construction. Program
// supplies the existing Body identity proof, Flow supplies authored joins,
// and allocation/body rows are read through child-owned bundles; no compiler
// state crosses this boundary.
type Input struct {
	Program     *program.Program
	Allocations *allocation.Bundle
	Bodies      *bodyboundary.Bundle
}

// Build joins closure allocations to their canonical callable body rows in
// allocation order. It returns the schema rows directly: there is no child
// row façade or second representation for the parent to reconcile.
func Build(input Input) ([]calltarget.Target, Fault) {
	if input.Program == nil || input.Bodies == nil || len(input.Bodies.Bodies()) == 0 {
		return nil, failure(ReasonUnavailable, -1, -1)
	}
	bodyByContext := make(map[identity.ContentID]programschema.Body, len(input.Bodies.Bodies()))
	for index, body := range input.Bodies.Bodies() {
		if !body.Available() || !body.ContextID().Available() {
			return nil, failure(ReasonUnavailable, index, -1)
		}
		bodyByContext[body.ContextID()] = body
	}
	rows := make([]calltarget.Target, 0)
	seenAllocations := make(map[identity.ContentID]struct{})
	seenBodies := make(map[identity.ContentID]struct{})
	flowView := input.Program.Flow()
	boundaries := flowView.FunctionBoundaries()
	authoredFunctions := flowView.Authored().Functions()
	if input.Allocations == nil {
		return nil, failure(ReasonUnavailable, -1, -1)
	}
	for index := 0; index < input.Allocations.Count(); index++ {
		allocation, allocationOK := input.Allocations.RowAt(index)
		role, roleOK := allocation.Role()
		if !allocationOK || !roleOK {
			return nil, failure(ReasonUnavailable, index, -1)
		}
		if role != heapallocation.RoleClosure {
			continue
		}
		allocationTerm, allocationTermOK := allocation.Term()
		allocationID, allocationIDOK := allocation.Template()
		if !allocationTermOK || !allocationIDOK {
			return nil, failure(ReasonUnavailable, index, -1)
		}
		boundary, boundaryOK := boundaries.For(allocationTerm)
		functionTerm, functionTermOK := boundary.Function()
		bodyTerm, bodyTermOK := boundary.Body()
		body, bodyOK := input.Program.Body(bodyTerm)
		_, functionOK := body.Function()
		bodyID := body.PathID()
		context := body.ContextID()
		flowFormalID, flowFormalIDOK := programschema.CallFormalIdentity(context)
		copied, copiedOK := bodyByContext[context]
		functionID, functionIDOK := copied.FunctionContextID()
		formalID, formalIDOK := copied.CallFormalID()
		owner, authoredBody, _, authoredOK := authoredFunctions.Get(allocationTerm)
		if !boundaryOK || !boundaries.OwnsFunction(boundary) || !functionTermOK || functionTerm != allocationTerm || !bodyTermOK || owner == 0 || authoredBody != bodyTerm || !authoredOK || !bodyOK || !functionOK || !functionIDOK || !formalIDOK || !flowFormalIDOK || !allocationID.Available() || !bodyID.Available() || !copied.ID().Available() || !context.Available() || !functionID.Available() || !formalID.Available() || !copiedOK || !copied.Callable() || copied.ID() != bodyID || copied.ContextID() != context || formalID != flowFormalID {
			return nil, failure(ReasonUnavailable, index, -1)
		}
		if _, duplicate := seenAllocations[allocationID]; duplicate {
			return nil, failure(ReasonDuplicate, index, -1)
		}
		if _, duplicate := seenBodies[context]; duplicate {
			return nil, failure(ReasonDuplicate, index, -1)
		}
		seenAllocations[allocationID], seenBodies[context] = struct{}{}, struct{}{}
		target, targetOK := calltarget.NewTarget(allocationID, copied.ID(), context, functionID, formalID)
		if !targetOK {
			return nil, failure(ReasonUnavailable, index, -1)
		}
		rows = append(rows, target)
	}
	return rows, Fault{}
}
