package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ForEachMissingMemberRead visits static member reads whose receiver is known
// to reject the member on the current solved path. It is the readmodel-owned
// source for the eventual missing-member-read obligation pass.
func (r Reader) ForEachMissingMemberRead(visit func(MissingMemberRead) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	return r.result.ForEachMissingMemberReadOccurrence(func(occ body.StaticMemberReadOccurrence) bool {
		item, ok := r.missingMemberRead(occ)
		if !ok {
			return true
		}
		visited = true
		return visit(item)
	}) || visited
}

func (r Reader) missingMemberRead(occ body.StaticMemberReadOccurrence) (MissingMemberRead, bool) {
	memberName := occ.MemberName
	if memberName == "" {
		return MissingMemberRead{}, false
	}
	if occ.AllowExactNilRead && r.exactLocalMissingFieldReadsNil(occ, memberName) {
		return MissingMemberRead{}, false
	}
	if occ.HasReceiverValueAtBoundary && r.ValueHasUntrustedTopOrigin(occ.ReceiverValueAtBoundary) {
		return MissingMemberRead{}, false
	}
	receiverType := occ.ReceiverTypeBeforeBoundary
	if !occ.HasReceiverTypeBeforeBoundary || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return MissingMemberRead{}, false
	}
	report := body.UnionArmRejectsFieldRead(receiverType, memberName)
	if !report {
		broad, broadOK := r.result.DeclaredPathTypeAt(occ.Point, occ.ReceiverPath, occ.HasReceiverPath)
		if !broadOK || broad == nil || !body.TypeIsMultiArmUnion(broad) {
			return MissingMemberRead{}, false
		}
		fieldBroad := broad
		if withoutNil := readmodelProjectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
			fieldBroad = withoutNil
		}
		if _, ok := body.TypeField(fieldBroad, memberName); !ok || !body.TypeFieldProvablyAbsent(receiverType, memberName) {
			return MissingMemberRead{}, false
		}
		report = true
	}
	if !report {
		return MissingMemberRead{}, false
	}
	return MissingMemberRead{
		Point:        occ.Point,
		ReadLabel:    occ.ReadLabel,
		MemberName:   memberName,
		ReceiverType: receiverType,
		Span:         sourceSpanFromBody(occ.Span),
	}, true
}

func (r Reader) exactLocalMissingFieldReadsNil(occ body.StaticMemberReadOccurrence, name string) bool {
	if name == "" || !occ.HasReceiverValueBeforeBoundary {
		return false
	}
	if r.declaredUnionHasMemberOnAnotherArm(occ, name) {
		return false
	}
	value := occ.ReceiverValueBeforeBoundary
	if !r.ValueHasLocalExclusiveExactIdentity(occ.Point, value) {
		return false
	}
	receiver, ok := r.ValueType(value)
	return ok && body.ClosedRecordLacksField(receiver, name)
}

func (r Reader) declaredUnionHasMemberOnAnotherArm(occ body.StaticMemberReadOccurrence, name string) bool {
	if r.result == nil || name == "" {
		return false
	}
	broad, broadOK := r.result.DeclaredPathTypeAt(occ.Point, occ.ReceiverPath, occ.HasReceiverPath)
	if !broadOK || broad == nil || !body.TypeIsMultiArmUnion(broad) {
		return false
	}
	fieldBroad := broad
	if withoutNil := readmodelProjectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
		fieldBroad = withoutNil
	}
	_, ok := body.TypeField(fieldBroad, name)
	return ok
}

func readmodelProjectionWithoutNil(t typ.Type) typ.Type {
	return body.ProjectionWithoutNil(t)
}
