package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ForEachMissingMemberRead visits static member reads whose receiver is known
// to reject the member on the current solved path. It is the readmodel-owned
// source for the eventual missing-member-read obligation pass.
func (r Reader) ForEachMissingMemberRead(visit func(MissingMemberRead) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	var occurrences []body.StaticMemberReadOccurrence
	nilDefault := map[missingMemberReadKey]bool{}
	r.result.ForEachMissingMemberReadOccurrence(func(occ body.StaticMemberReadOccurrence) bool {
		occurrences = append(occurrences, occ)
		if occ.AllowExactNilRead {
			nilDefault[missingMemberOccurrenceKey(occ)] = true
		}
		return true
	})
	visited := false
	for _, occ := range occurrences {
		if !occ.AllowExactNilRead && nilDefault[missingMemberOccurrenceKey(occ)] {
			continue
		}
		item, ok := r.missingMemberRead(occ)
		if !ok {
			continue
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

type missingMemberReadKey struct {
	readLabel  string
	memberName string
	span       readapi.SourceSpan
}

func missingMemberOccurrenceKey(occ body.StaticMemberReadOccurrence) missingMemberReadKey {
	return missingMemberReadKey{
		readLabel:  occ.ReadLabel,
		memberName: occ.MemberName,
		span:       sourceSpanFromBody(occ.Span),
	}
}

func (r Reader) missingMemberRead(occ body.StaticMemberReadOccurrence) (MissingMemberRead, bool) {
	memberName := occ.MemberName
	if memberName == "" {
		return MissingMemberRead{}, false
	}
	if occ.AllowExactNilRead && r.exactLocalMissingFieldReadsNil(occ, memberName) {
		return MissingMemberRead{}, false
	}
	receiverType := occ.ReceiverTypeBeforeBoundary
	if !occ.HasReceiverTypeBeforeBoundary || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return MissingMemberRead{}, false
	}
	if occ.AllowExactNilRead && body.TypeFieldProvablyAbsent(receiverType, memberName) {
		return MissingMemberRead{}, false
	}
	report := body.UnionArmRejectsFieldRead(receiverType, memberName)
	if !report {
		broad, broadOK := r.missingMemberBroadReceiverType(occ, receiverType)
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

func (r Reader) missingMemberBroadReceiverType(occ body.StaticMemberReadOccurrence, current typ.Type) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	if broad, ok := r.result.DeclaredPathTypeAt(occ.Point, occ.ReceiverPath, occ.HasReceiverPath); ok {
		return broad, true
	}
	if occ.HasReceiverValueAtBoundary {
		if broad, ok := r.FullVariantOriginType(occ.ReceiverValueAtBoundary); ok && currentBelongsToBroadFamily(r, current, broad) {
			return broad, true
		}
	}
	if occ.HasReceiverValueBeforeBoundary {
		if broad, ok := r.FullVariantOriginType(occ.ReceiverValueBeforeBoundary); ok && currentBelongsToBroadFamily(r, current, broad) {
			return broad, true
		}
	}
	return nil, false
}

func currentBelongsToBroadFamily(r Reader, current, broad typ.Type) bool {
	return current != nil && broad != nil && r.IsSubtype(current, broad)
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
