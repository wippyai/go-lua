// Package calltarget owns closure-allocation to callable-boundary joins.
// It consumes only immutable Flow data and already-canonical compiler bundles;
// the parent sequences the canonical construction fault and retains the
// returned target rows.
package calltarget

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/allocation"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/bodyboundary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
)

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
func Build(input Input) ([]calltarget.Target, programconstruction.Fault) {
	if input.Program == nil || input.Bodies == nil || len(input.Bodies.Bodies()) == 0 {
		return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, -1, -1)
	}
	bodyByContext := make(map[identity.ContentID]programschema.Body, len(input.Bodies.Bodies()))
	for index, body := range input.Bodies.Bodies() {
		if !body.Available() || !body.ContextID().Available() {
			return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, index, -1)
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
		return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, -1, -1)
	}
	for index := 0; index < input.Allocations.Count(); index++ {
		allocation, allocationOK := input.Allocations.RowAt(index)
		role, roleOK := allocation.Role()
		if !allocationOK || !roleOK {
			return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, index, -1)
		}
		if role != heapallocation.RoleClosure {
			continue
		}
		allocationTerm, allocationTermOK := allocation.Term()
		allocationID, allocationIDOK := allocation.Template()
		if !allocationTermOK || !allocationIDOK {
			return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, index, -1)
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
			return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, index, -1)
		}
		if _, duplicate := seenAllocations[allocationID]; duplicate {
			return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetDuplicate, index, -1)
		}
		if _, duplicate := seenBodies[context]; duplicate {
			return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetDuplicate, index, -1)
		}
		seenAllocations[allocationID], seenBodies[context] = struct{}{}, struct{}{}
		target, targetOK := calltarget.NewTarget(allocationID, copied.ID(), context, functionID, formalID)
		if !targetOK {
			return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, index, -1)
		}
		rows = append(rows, target)
	}
	return rows, programconstruction.Fault{}
}

// ClosureCaptureProofs issues the exact positive-capture conclusion adjacent
// to target construction. The generic issuance machine consumes only these
// identities; it never scans allocation, target, body, or capture rows.
func ClosureCaptureProofs(input Input, targets []calltarget.Target) ([]identity.ContentID, programconstruction.Fault) {
	if input.Bodies == nil {
		return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, -1, -1)
	}
	proofs := make([]identity.ContentID, 0, len(targets))
	for index, target := range targets {
		boundary, boundaryOK := input.Bodies.FunctionBoundaryForBody(target.BodyID())
		if !target.Available() || !boundaryOK {
			return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, index, -1)
		}
		if boundary.CaptureCount() == 0 {
			continue
		}
		for captureIndex := 0; captureIndex < boundary.CaptureCount(); captureIndex++ {
			capture, captureOK := input.Bodies.FunctionCaptureAt(boundary, captureIndex)
			position, positionOK := capture.Position()
			if !captureOK || !positionOK || position != captureIndex || capture.InnerBodyID() != boundary.BodyID() {
				return nil, programconstruction.New(programcatalog.CallTarget(), programconstruction.IssueCallTargetUnavailable, index, captureIndex)
			}
		}
		proofs = append(proofs, target.AllocationID())
	}
	return proofs, programconstruction.Fault{}
}
