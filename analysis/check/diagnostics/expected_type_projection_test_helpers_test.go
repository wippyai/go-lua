package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func expectedTypeAtSegments(root typ.Type, segments []segment.Segment) (typ.Type, bool) {
	current := root
	for _, seg := range segments {
		next, ok := expectedSegmentType(current, seg)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, current != nil
}

func expectedSegmentType(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	t = transparentExpectedType(t)
	switch tt := t.(type) {
	case *typ.Optional:
		return expectedSegmentType(tt.Inner, seg)
	case *typ.Union:
		var matches []typ.Type
		for _, member := range tt.Members {
			if next, ok := expectedSegmentType(member, seg); ok {
				matches = append(matches, next)
			}
		}
		if len(matches) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(matches...), true
	case *typ.Intersection:
		var matches []typ.Type
		for _, member := range tt.Members {
			if next, ok := expectedSegmentType(member, seg); ok {
				matches = append(matches, next)
			}
		}
		if len(matches) == 0 {
			return nil, false
		}
		return normalize.IntersectionForMeet(matches...), true
	case *typ.Array:
		if seg.Kind != segment.SegmentIndexInt {
			return nil, false
		}
		return tt.Element, tt.Element != nil
	case *typ.Tuple:
		if seg.Kind != segment.SegmentIndexInt || seg.Index <= 0 || seg.Index > len(tt.Elements) {
			return nil, false
		}
		elem := tt.Elements[seg.Index-1]
		return elem, elem != nil
	case *typ.Record:
		return expectedRecordSegmentType(tt, seg)
	case *typ.Interface:
		return expectedInterfaceSegmentType(tt, seg)
	case *typ.Map:
		return expectedMapSegmentType(tt.Key, tt.Value, seg)
	case *typ.ReadonlyMap:
		return expectedMapSegmentType(tt.Key, tt.Value, seg)
	}
	return nil, false
}

func expectedMapSegmentType(keyType, valueType typ.Type, seg segment.Segment) (typ.Type, bool) {
	key, ok := luatypeprojection.SegmentKeyType(seg)
	if !ok || !subtype.IsSubtype(key, keyType) {
		return nil, false
	}
	return valueType, valueType != nil
}

func expectedInterfaceSegmentType(iface *typ.Interface, seg segment.Segment) (typ.Type, bool) {
	if iface == nil || seg.Kind != segment.SegmentField {
		return nil, false
	}
	for _, method := range iface.Methods {
		if method.Name != seg.Name || method.Type == nil {
			continue
		}
		return subst.Self(method.Type, iface), true
	}
	return nil, false
}

func expectedRecordSegmentType(record *typ.Record, seg segment.Segment) (typ.Type, bool) {
	if record == nil {
		return nil, false
	}
	switch seg.Kind {
	case segment.SegmentField:
		if field := record.GetField(seg.Name); field != nil {
			return field.Type, field.Type != nil
		}
	case segment.SegmentIndexString:
		if member := record.GetStaticStringIndex(seg.Name); member != nil {
			return member.Type, member.Type != nil
		}
	case segment.SegmentIndexInt:
		if member := record.GetStaticIntIndex(int64(seg.Index)); member != nil {
			return member.Type, member.Type != nil
		}
	}
	if !record.HasMapComponent() {
		return nil, false
	}
	key, ok := luatypeprojection.SegmentKeyType(seg)
	if !ok || !subtype.IsSubtype(key, record.MapKey) {
		return nil, false
	}
	return record.MapValue, record.MapValue != nil
}
