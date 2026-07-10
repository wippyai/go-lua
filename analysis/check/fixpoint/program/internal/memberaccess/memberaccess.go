// Package memberaccess centralizes summary-member access semantics shared by
// summary projection and call-result application.
package memberaccess

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

// Valid reports whether seg names an exact summary member.
func Valid(seg segment.Segment) bool {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name != ""
	case segment.SegmentIndexInt:
		return true
	default:
		return false
	}
}

// Callable resolves the callable member reached by seg on receiver.
func Callable(receiver typ.Type, member segment.Segment) (*typ.Function, typecall.MemberCallStatus, bool) {
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if member.Name == "" {
			return nil, typecall.MemberCallMissing, false
		}
		return typecall.MemberCallable(receiver, member.Name)
	case segment.SegmentIndexInt:
		return typecall.IndexedMemberCallable(receiver, typ.LiteralInt(int64(member.Index)))
	default:
		return nil, typecall.MemberCallMissing, false
	}
}

// Paths returns all concrete paths equivalent to receiver.member.
func Paths(receiver pathdom.Path, member segment.Segment) []pathdom.Path {
	if receiver.IsEmpty() || !Valid(member) {
		return nil
	}
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return []pathdom.Path{receiver.Field(member.Name), receiver.IndexStr(member.Name)}
	case segment.SegmentIndexInt:
		return []pathdom.Path{receiver.IndexInt(member.Index)}
	default:
		return nil
	}
}
