package judgment

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// NewTypeRef builds the canonical stable reference for a resolved type.
func NewTypeRef(t typ.Type) TypeRef {
	if t == nil {
		return TypeRef{Key: "type:unknown"}
	}
	return TypeRef{Key: fmt.Sprintf("type:%016x:%s", typ.EqualityHash(t), t.String()), Type: t}
}

// WithLabel returns r with a renderer-facing evidence label. The label is not
// part of Key; it preserves contract provenance without changing semantic
// identity.
func (r TypeRef) WithLabel(label string) TypeRef {
	r.Label = label
	return r
}

// NewValueRef builds the canonical stable reference for a solved value and its
// projected display type.
func NewValueRef(hash uint64, projected typ.Type) ValueRef {
	actual := "unknown"
	if projected != nil {
		actual = NewTypeRef(projected).Key
	}
	return ValueRef{Key: fmt.Sprintf("value:%016x:%s", hash, actual), ProjectedType: projected}
}

// WithLabel returns r with a renderer-facing value label. The label is not part
// of Key; it preserves source identity without changing semantic identity.
func (r ValueRef) WithLabel(label string) ValueRef {
	r.Label = label
	return r
}
