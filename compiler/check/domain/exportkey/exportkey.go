// Package exportkey resolves module export members through structural CFG paths.
package exportkey

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/constraint"
)

// SymbolSource exposes the graph-owned identities needed to resolve exported
// function members without reparsing display names.
type SymbolSource interface {
	NameOf(cfg.SymbolID) string
	FuncDefPathForSymbol(cfg.SymbolID) (constraint.Path, bool)
}

// MemberPathKey is the stable comparable identity of an exported member path.
// It is a canonical serialization of structural segments, not a source display
// name; use MemberPath.Segments when applying it back to a type.
type MemberPathKey constraint.PathKey

// MemberPath identifies a statically-known member inside a module export. It is
// rooted at the exported value itself, so `M.api.run` is stored as
// `.api.run`, not as the display string "M.api.run".
type MemberPath struct {
	key      MemberPathKey
	segments []fieldkey.Key
}

// NewMemberPath validates and canonicalizes a structural export member path.
func NewMemberPath(segments []fieldkey.Key) (MemberPath, bool) {
	if len(segments) == 0 {
		return MemberPath{}, false
	}
	copied := make([]fieldkey.Key, 0, len(segments))
	for _, seg := range segments {
		key, ok := fieldkey.FromSegment(seg)
		if !ok {
			return MemberPath{}, false
		}
		copied = append(copied, key)
	}
	return MemberPath{
		key:      MemberPathKey(constraint.FormatSegments(copied)),
		segments: copied,
	}, true
}

// Key returns the stable comparable cache/map key for this member path.
func (p MemberPath) Key() MemberPathKey { return p.key }

// Segments returns a defensive copy of the structural member path.
func (p MemberPath) Segments() []fieldkey.Key {
	if len(p.segments) == 0 {
		return nil
	}
	out := make([]fieldkey.Key, len(p.segments))
	copy(out, p.segments)
	return out
}

// MemberPathFromTargetPath projects a structural function target path to the
// exported member path it contributes to. When rootName is known, all static
// member segments below that root are admissible; this supports nested exported
// functions without collapsing them to their leaf name. Without a known root, a
// direct function name maps to that top-level member, while segmented paths are
// still applied structurally and must match the export shape later.
func MemberPathFromTargetPath(rootName string, path constraint.Path) (MemberPath, bool) {
	if path.IsEmpty() {
		return MemberPath{}, false
	}
	if rootName != "" {
		if path.Root != rootName || len(path.Segments) == 0 {
			return MemberPath{}, false
		}
		return NewMemberPath(path.Segments)
	}
	if len(path.Segments) == 0 {
		key, ok := fieldkey.FromName(path.Root)
		if !ok {
			return MemberPath{}, false
		}
		return NewMemberPath([]fieldkey.Key{key})
	}
	return NewMemberPath(path.Segments)
}

// MemberPathFromGraphSymbol resolves the exported member path published by sym.
// Function definitions use CFG structure; the display-name fallback is only for
// direct local/global symbols.
func MemberPathFromGraphSymbol(rootName string, source SymbolSource, sym cfg.SymbolID) (MemberPath, bool) {
	if source == nil || sym == 0 {
		return MemberPath{}, false
	}
	if path, ok := source.FuncDefPathForSymbol(sym); ok {
		return MemberPathFromTargetPath(rootName, path)
	}
	name := source.NameOf(sym)
	if rootName != "" || name == "" || strings.Contains(name, ".") {
		return MemberPath{}, false
	}
	key, ok := fieldkey.FromName(name)
	if !ok {
		return MemberPath{}, false
	}
	return NewMemberPath([]fieldkey.Key{key})
}

// FromGraphSymbol resolves the top-level export member published by sym.
//
// Function definitions use the CFG's structural TargetPath. The NameOf fallback
// is intentionally only for direct local/global symbols; dotted symbol names are
// rejected so callers do not recover semantic paths by reparsing display names.
func FromGraphSymbol(rootName string, source SymbolSource, sym cfg.SymbolID) (fieldkey.Key, bool) {
	memberPath, ok := MemberPathFromGraphSymbol(rootName, source, sym)
	if !ok {
		return fieldkey.Key{}, false
	}
	segments := memberPath.Segments()
	if len(segments) != 1 {
		return fieldkey.Key{}, false
	}
	return segments[0], true
}

// FromTargetPath projects a structural function target path to the export member
// it contributes to. Direct exports are root symbols; table exports are exactly
// one static member below the exported root. Deeper paths are rejected because
// mapping them to a leaf member would mis-associate nested/private functions.
func FromTargetPath(rootName string, path constraint.Path) (fieldkey.Key, bool) {
	memberPath, ok := MemberPathFromTargetPath(rootName, path)
	if !ok {
		return fieldkey.Key{}, false
	}
	segments := memberPath.Segments()
	if len(segments) != 1 {
		return fieldkey.Key{}, false
	}
	return segments[0], true
}
