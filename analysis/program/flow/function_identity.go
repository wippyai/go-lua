package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/internal/framing"
)

const functionVarargCellRole = uint64(2)

const (
	functionFormalCellRole   = uint64(1)
	functionCaptureInnerRole = uint64(3)
	functionCaptureOuterRole = uint64(4)
)

// FunctionFormalIDs issues the semantic identity and cell identity of one
// existing formal input. Flow owns the Body path and callable boundary
// admission; root-fenced storage and optional declared-type joins remain
// outside this owner query.
func (view View) FunctionFormalIDs(boundary functionboundary.Boundary, index int) (id, cellID identity.ContentID, ok bool) {
	if !view.available() || !boundary.Available() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	boundaries := view.FunctionBoundaries()
	if !boundaries.OwnsFunction(boundary) {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	term, termOK := boundary.FormalAt(index)
	bodyTerm, bodyTermOK := boundary.Body()
	body, bodyOK := boundaries.ForBody(bodyTerm)
	pathID, pathOK := view.BodyPath(bodyTerm)
	if !termOK || term == 0 || !bodyTermOK || !bodyOK || !boundaries.OwnsBody(body) || !pathOK || !pathID.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	cellID = functionCellID(pathID, functionFormalCellRole, uint64(index))
	id = flowSemanticID("program/transformer/formal", func(writer *framing.Writer) bool {
		return writer.Bytes(pathID[:]) == nil && writer.Uint(uint64(index)) == nil && writer.Bytes(cellID[:]) == nil
	})
	return id, cellID, id.Available() && cellID.Available()
}

// FunctionCaptureCells issues the semantic cell identities and Body paths of
// one existing closure capture. Flow proves the exact inner/outer Body
// polarity and owns the path-based cell equations. The final capture identity
// remains a Program/root-fenced equation because it commits both storage-cell
// identities as well.
func (view View) FunctionCaptureCells(boundary functionboundary.Boundary, index int) (innerID, outerID, innerBodyID, outerBodyID identity.ContentID, ok bool) {
	if !view.available() || !boundary.Available() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	boundaries := view.FunctionBoundaries()
	if !boundaries.OwnsFunction(boundary) {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	pair, pairOK := boundary.CaptureAt(index)
	bodyTerm, bodyTermOK := boundary.Body()
	body, bodyOK := boundaries.ForBody(bodyTerm)
	innerBody, innerBodyOK := boundaries.ForBody(pair.InnerBody)
	outerBody, outerBodyOK := boundaries.ForBody(pair.OuterBody)
	if !pairOK || pair.Inner == 0 || pair.Outer == 0 || !bodyTermOK || !bodyOK || !innerBodyOK || !outerBodyOK ||
		!boundaries.OwnsBody(body) || !boundaries.OwnsBody(innerBody) || !boundaries.OwnsBody(outerBody) ||
		!innerBody.Equal(body) || outerBody.Equal(innerBody) {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	var innerBodyPathOK, outerBodyPathOK bool
	innerBodyID, innerBodyPathOK = view.BodyPath(pair.InnerBody)
	outerBodyID, outerBodyPathOK = view.BodyPath(pair.OuterBody)
	if !innerBodyPathOK || !outerBodyPathOK || !innerBodyID.Available() || !outerBodyID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	innerID = functionCellID(innerBodyID, functionCaptureInnerRole, uint64(index))
	outerID = functionCellID(outerBodyID, functionCaptureOuterRole, uint64(index))
	return innerID, outerID, innerBodyID, outerBodyID, innerID.Available() && outerID.Available()
}

// FunctionVarargIDs returns the scalar identities of one existing Function's
// optional vararg input. Function boundary ownership, vararg admission, Body
// ownership/path, and both identity equations are canonical Flow data.
func (view View) FunctionVarargIDs(boundary functionboundary.Boundary) (id, cellID identity.ContentID, ok bool) {
	if !view.available() || !boundary.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	boundaries := view.FunctionBoundaries()
	if !boundaries.OwnsFunction(boundary) {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	term, termOK := boundary.Vararg()
	bodyTerm, bodyTermOK := boundary.Body()
	body, bodyOK := boundaries.ForBody(bodyTerm)
	pathID, pathOK := view.BodyPath(bodyTerm)
	if !termOK || term == 0 || !bodyTermOK || !bodyOK || !boundaries.OwnsBody(body) || !body.Available() || !pathOK || !pathID.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	cellID = functionCellID(pathID, functionVarargCellRole, 0)
	id = flowSemanticID("program/transformer/vararg", func(writer *framing.Writer) bool {
		return writer.Bytes(pathID[:]) == nil && writer.Bytes(cellID[:]) == nil
	})
	return id, cellID, id.Available() && cellID.Available()
}

func functionCellID(bodyPath identity.ContentID, role, index uint64) identity.ContentID {
	return flowSemanticID("program/transformer/cell-semantic", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Uint(role) == nil && writer.Uint(index) == nil
	})
}
