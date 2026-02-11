package domain

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
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
	root, suffix, ok := splitPathKeyRootAndSuffix(key)
	if !ok || suffix == "" {
		return "", "", false
	}

	segs := pathkey.ParseSuffix(suffix)
	if len(segs) == 0 {
		return "", "", false
	}

	last := segs[len(segs)-1]
	if last.Kind != constraint.SegmentField {
		return "", "", false
	}

	parent := root + pathkey.SegmentsSuffix(segs[:len(segs)-1])
	return constraint.PathKey(parent), last.Name, true
}

func splitPathKeyRootAndSuffix(key constraint.PathKey) (root string, suffix string, ok bool) {
	if sym, version, suffix, ok := pathkey.ParseKey(key); ok {
		var b strings.Builder
		b.WriteString("sym")
		b.WriteString(strconv.FormatUint(uint64(sym), 10))
		if version != 0 {
			b.WriteByte('@')
			b.WriteString(strconv.Itoa(version))
		}
		return b.String(), suffix, true
	}

	s := string(key)
	if s == "" {
		return "", "", false
	}

	if s[0] == '$' {
		end := 1
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end > 1 {
			return s[:end], s[end:], true
		}
	}

	if strings.HasPrefix(s, "ret[") {
		end := 4
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end > 4 && end < len(s) && s[end] == ']' {
			end++
			return s[:end], s[end:], true
		}
	}

	end := 0
	for end < len(s) && s[end] != '.' && s[end] != '[' {
		end++
	}
	if end == 0 {
		return "", "", false
	}
	return s[:end], s[end:], true
}

// IsChildPath returns true if child is a descendant path of parent.
//
// A path is a child if it extends the parent with one or more path segments
// (field or index access) under the same canonical root.
//
// Examples:
//   - IsChildPath("sym1@1", "sym1@1.foo") -> true
//   - IsChildPath("sym1@1", "sym1@1.foo.bar") -> true
//   - IsChildPath("sym1@1.foo", "sym1@1.foo.bar") -> true
//   - IsChildPath("sym1@1", "sym1@1[0]") -> true
//   - IsChildPath("sym1@1", "sym1@1") -> false (not strict descendant)
//   - IsChildPath("sym1@1", "sym1@10") -> false (different version, not child)
func IsChildPath(parent, child string) bool {
	if parent == "" || child == "" || parent == child {
		return false
	}

	parentRoot, parentSuffix, ok := splitPathKeyRootAndSuffix(constraint.PathKey(parent))
	if !ok {
		return false
	}
	childRoot, childSuffix, ok := splitPathKeyRootAndSuffix(constraint.PathKey(child))
	if !ok || parentRoot != childRoot {
		return false
	}

	parentSegs := pathkey.ParseSuffix(parentSuffix)
	childSegs := pathkey.ParseSuffix(childSuffix)
	if len(childSegs) == 0 || len(childSegs) <= len(parentSegs) {
		return false
	}
	return pathkey.SegmentsPrefix(parentSegs, childSegs)
}
