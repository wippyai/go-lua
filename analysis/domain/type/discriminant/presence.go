package discriminant

import (
	"github.com/wippyai/go-lua/analysis/domain/type/literal"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// RecordsPresenceConflict reports whether two records are distinguished by the
// mutual presence of required, non-literal fields.
func (d *Detector) RecordsPresenceConflict(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return false
	}
	return requiredNonLiteralPayloadMissingFrom(a, b) && requiredNonLiteralPayloadMissingFrom(b, a)
}

func requiredNonLiteralPayloadMissingFrom(src, dst *typ.Record) bool {
	for _, field := range src.Fields {
		if field.Optional {
			continue
		}
		if _, ok := literal.ExtractAliasOnly(field.Type); ok {
			continue
		}
		if dst.GetField(field.Name) == nil {
			return true
		}
	}
	for _, member := range src.StaticMembers {
		if member.Optional {
			continue
		}
		if _, ok := literal.ExtractAliasOnly(member.Type); ok {
			continue
		}
		if dst.GetStaticMember(member.Kind, member.Name, member.Index) == nil {
			return true
		}
	}
	return false
}
