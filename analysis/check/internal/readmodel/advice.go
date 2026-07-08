package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ForEachRedundantClaim visits runtime claim/cast sites whose operand already
// has a proven subtype of the claimed type.
func (r Reader) ForEachRedundantClaim(visit func(RedundantClaim) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	return r.result.ForEachRedundantClaimOccurrence(func(occ body.RedundantClaimOccurrence) bool {
		return visit(RedundantClaim{
			Point:        occ.Point,
			OperandLabel: occ.OperandLabel,
			ClaimLabel:   occ.ClaimLabel,
			OperandType:  occ.OperandType,
			ClaimedType:  occ.ClaimedType,
			OperandSpan:  sourceSpanFromBody(occ.OperandSpan),
			ClaimSpan:    sourceSpanFromBody(occ.ClaimSpan),
		})
	})
}

// ForEachAlwaysTrueGuard visits branch conditions whose solved expression type
// is exactly the singleton true or singleton false type.
func (r Reader) ForEachAlwaysTrueGuard(visit func(AlwaysTrueGuard) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	return r.result.ForEachUserVisibleBranchConditionOccurrence(func(occ body.BranchConditionOccurrence) bool {
		t, ok := r.result.ExpressionTypeBeforeBoundary(occ.Point, occ.Fact.Condition)
		if !ok || t == nil {
			return true
		}
		always, singleton := r.singletonBoolean(t)
		if !singleton {
			return true
		}
		label := body.ExpressionLabel(occ.Fact.Condition)
		if label == "" {
			label = "condition"
		}
		visited = true
		return visit(AlwaysTrueGuard{
			Point:          occ.Point,
			Always:         always,
			ConditionLabel: label,
			ConditionType:  t,
			ConditionSpan:  sourceSpanFromBody(occ.ConditionSpan),
		})
	}) || visited
}

func (r Reader) singletonBoolean(t typ.Type) (bool, bool) {
	if t == nil || r.result == nil {
		return false, false
	}
	if r.result.IsSubtype(t, typ.True) && r.result.IsSubtype(typ.True, t) {
		return true, true
	}
	if r.result.IsSubtype(t, typ.False) && r.result.IsSubtype(typ.False, t) {
		return false, true
	}
	return false, false
}

// ForEachInvariantLoopRead visits loop-contained member/index reads whose read
// path is stable through the loop and whose receiver is non-nil.
func (r Reader) ForEachInvariantLoopRead(visit func(InvariantLoopRead) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	r.result.ForEachStaticMemberReadOccurrence(func(occ body.StaticMemberReadOccurrence) bool {
		if !occ.HasReceiverPath || occ.ReceiverPath.IsEmpty() ||
			!occ.HasReadPath || occ.ReadPath.IsEmpty() ||
			!occ.HasReceiverTypeBeforeBoundary ||
			occ.ReceiverTypeBeforeBoundary == nil ||
			typ.IsTopLike(occ.ReceiverTypeBeforeBoundary) ||
			typ.IsNever(occ.ReceiverTypeBeforeBoundary) ||
			typevalue.TypeIncludesNil(occ.ReceiverTypeBeforeBoundary) {
			return true
		}
		loop, ok := r.result.InnermostLoopForPoint(occ.Point)
		if !ok {
			return true
		}
		if r.result.PathInvalidatedInLoop(loop.Head, occ.ReadPath) {
			return true
		}
		receiverLabel := occ.ReceiverPath.String()
		if receiverLabel == "" {
			receiverLabel = "receiver"
		}
		visited = true
		return visit(InvariantLoopRead{
			Point:         occ.Point,
			LoopHead:      loop.Head,
			ReadLabel:     occ.ReadLabel,
			ReceiverLabel: receiverLabel,
			ReceiverPath:  occ.ReceiverPath,
			ReadPath:      occ.ReadPath,
			ReceiverType:  occ.ReceiverTypeBeforeBoundary,
			ReadSpan:      sourceSpanFromBody(occ.Span),
			LoopSpan:      sourceSpanFromBody(loop.Span),
		})
	})
	return visited
}
