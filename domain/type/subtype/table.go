package subtype

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func readonlyStaticMemberKeyType(member typ.StaticMember) (typ.Type, bool) {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return typ.LiteralString(member.Name), true
	case typ.StaticMemberIntIndex:
		return typ.LiteralInt(member.Index), true
	default:
		return nil, false
	}
}

func checkTableTop(sub, super typ.Type) (bool, bool) {
	if !typ.IsBuiltinTableTopMarker(super) {
		return false, false
	}
	return typetable.IsLike(sub), true
}

func emptyRecordAdoptsContainerShape(sub *typ.Record, super typ.Type) bool {
	if sub == nil || super == nil || len(sub.Fields) != 0 || len(sub.StaticMembers) != 0 {
		return false
	}
	return super.Kind() == kind.Array || super.Kind() == kind.Map || super.Kind() == kind.ReadonlyMap
}
