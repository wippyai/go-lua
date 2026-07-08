package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
)

// ForEachMissingMemberRead visits static member reads whose receiver is known
// to reject the member on the current solved path. It is the readmodel-owned
// source for the eventual missing-member-read obligation pass.
func (r Reader) ForEachMissingMemberRead(visit func(MissingMemberRead) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	return r.result.ForEachMissingMemberRead(func(read body.MissingMemberRead) bool {
		return visit(MissingMemberRead{
			Point:        read.Point,
			ReadLabel:    read.ReadLabel,
			MemberName:   read.MemberName,
			ReceiverType: read.ReceiverType,
			Span:         sourceSpanFromBody(read.Span),
		})
	})
}
