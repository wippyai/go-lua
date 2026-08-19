package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/internal/framing"
)

const functionVarargCellRole = uint64(2)

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
	cellID = flowSemanticID("program/transformer/cell-semantic", func(writer *framing.Writer) bool {
		return writer.Bytes(pathID[:]) == nil && writer.Uint(functionVarargCellRole) == nil && writer.Uint(0) == nil
	})
	id = flowSemanticID("program/transformer/vararg", func(writer *framing.Writer) bool {
		return writer.Bytes(pathID[:]) == nil && writer.Bytes(cellID[:]) == nil
	})
	return id, cellID, id.Available() && cellID.Available()
}
