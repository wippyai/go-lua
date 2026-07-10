package readmodel

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/body"
)

// ForEachDeadAssignment visits writes whose assigned value is discarded before
// any reachable read on every path.
func (r Reader) ForEachDeadAssignment(visit func(DeadAssignment) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	proofs := r.result.DeadAssignmentProofs()
	for _, proof := range proofs {
		item := readmodelDeadAssignmentFromBody(proof)
		if !visit(item) {
			return true
		}
	}
	return len(proofs) > 0
}

func readmodelDeadAssignmentFromBody(proof body.DeadAssignmentProof) DeadAssignment {
	item := DeadAssignment{
		Point:     proof.Write.Point,
		Name:      proof.Write.Name,
		Key:       strconv.Itoa(int(proof.Write.Symbol)),
		WriteSpan: sourceSpanFromBody(proof.Write.Span),
	}
	for _, overwrite := range proof.Overwrites {
		item.Overwrites = append(item.Overwrites, DeadAssignmentOverwrite{
			Point: overwrite.Point,
			Span:  sourceSpanFromBody(overwrite.Span),
		})
	}
	for _, exit := range proof.Exits {
		item.Exits = append(item.Exits, DeadAssignmentExit{
			Point: exit.Point,
			Span:  sourceSpanFromBody(exit.Span),
		})
	}
	return item
}
