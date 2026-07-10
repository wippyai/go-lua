package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
)

// ForEachRedundantClaim visits runtime claim/cast sites whose operand already
// has a proven subtype of the claimed type.
func (r Reader) ForEachRedundantClaim(visit func(RedundantClaim) bool) bool {
	return projectBodyOccurrences(r, visit, (*body.Result).ForEachRedundantClaimOccurrence, redundantClaimFromBody)
}

// ForEachAlwaysTrueGuard visits branch conditions whose solved expression type
// is exactly the singleton true or singleton false type.
func (r Reader) ForEachAlwaysTrueGuard(visit func(AlwaysTrueGuard) bool) bool {
	return projectBodyOccurrences(r, visit, (*body.Result).ForEachAlwaysTrueGuardOccurrence, alwaysTrueGuardFromBody)
}

// ForEachInvariantLoopRead visits loop-contained member/index reads whose read
// path is stable through the loop and whose receiver is non-nil.
func (r Reader) ForEachInvariantLoopRead(visit func(InvariantLoopRead) bool) bool {
	return projectBodyOccurrences(r, visit, (*body.Result).ForEachInvariantLoopReadOccurrence, invariantLoopReadFromBody)
}

func redundantClaimFromBody(_ Reader, occ body.RedundantClaimOccurrence) RedundantClaim {
	return RedundantClaim{
		Point:        occ.Point,
		OperandLabel: occ.OperandLabel,
		ClaimLabel:   occ.ClaimLabel,
		OperandType:  occ.OperandType,
		ClaimedType:  occ.ClaimedType,
		OperandSpan:  sourceSpanFromBody(occ.OperandSpan),
		ClaimSpan:    sourceSpanFromBody(occ.ClaimSpan),
	}
}

func alwaysTrueGuardFromBody(_ Reader, occ body.AlwaysTrueGuardOccurrence) AlwaysTrueGuard {
	return AlwaysTrueGuard{
		Point:          occ.Point,
		Always:         occ.Always,
		ConditionLabel: occ.ConditionLabel,
		ConditionType:  occ.ConditionType,
		ConditionSpan:  sourceSpanFromBody(occ.ConditionSpan),
	}
}

func invariantLoopReadFromBody(_ Reader, occ body.InvariantLoopReadOccurrence) InvariantLoopRead {
	receiverLabel := occ.ReceiverPath.String()
	if receiverLabel == "" {
		receiverLabel = "receiver"
	}
	return InvariantLoopRead{
		Point:         occ.Point,
		LoopHead:      occ.LoopHead,
		ReadLabel:     occ.ReadLabel,
		ReceiverLabel: receiverLabel,
		ReceiverPath:  occ.ReceiverPath,
		ReadPath:      occ.ReadPath,
		ReceiverType:  occ.ReceiverType,
		ReadSpan:      sourceSpanFromBody(occ.ReadSpan),
		LoopSpan:      sourceSpanFromBody(occ.LoopSpan),
	}
}
