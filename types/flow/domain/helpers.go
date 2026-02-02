package domain

import (
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
)

// SplitPathKey extracts the parent key and field name from a path key.
//
// Given a key like "sym42@3.field", returns ("sym42@3", "field", true).
// This is used for parent narrowing: when a field like r.ok is tested,
// we may need to narrow the parent type r based on the field's value.
//
// The function finds the last '.' not inside brackets and splits there.
// If no such dot exists (the key has no field segment), returns ("", "", false).
//
// Examples:
//   - "sym1@1.foo" -> ("sym1@1", "foo", true)
//   - "sym1@1.foo.bar" -> ("sym1@1.foo", "bar", true)
//   - "sym1@1[0].x" -> ("sym1@1[0]", "x", true)
//   - "sym1@1" -> ("", "", false)
//   - "sym1@1[0]" -> ("", "", false)
func SplitPathKey(key constraint.PathKey) (constraint.PathKey, string, bool) {
	s := string(key)
	lastDot := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			lastDot = i
			break
		}
		if s[i] == '[' {
			break
		}
	}
	if lastDot <= 0 {
		return "", "", false
	}
	return constraint.PathKey(s[:lastDot]), s[lastDot+1:], true
}

// IsChildPath returns true if child is a descendant path of parent.
//
// A path is a child if it extends the parent with a field access. This is
// determined by prefix matching with a dot separator.
//
// Examples:
//   - IsChildPath("sym1@1", "sym1@1.foo") -> true
//   - IsChildPath("sym1@1", "sym1@1.foo.bar") -> true
//   - IsChildPath("sym1@1.foo", "sym1@1.foo.bar") -> true
//   - IsChildPath("sym1@1", "sym1@1") -> false (not strict descendant)
//   - IsChildPath("sym1@1", "sym1@10") -> false (different version, not child)
//   - IsChildPath("sym1@1", "sym1@1[0]") -> false (index, not field)
func IsChildPath(parent, child string) bool {
	if parent == "" || child == "" || parent == child {
		return false
	}
	if len(child) <= len(parent) {
		return false
	}
	return strings.HasPrefix(child, parent+".")
}
