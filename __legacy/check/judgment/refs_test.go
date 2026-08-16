package judgment

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestStableRefsIncludeHashAndRenderedType(t *testing.T) {
	typeRef := NewTypeRef(typ.String)
	if !strings.Contains(typeRef.Key, "string") || !strings.HasPrefix(typeRef.Key, "type:") {
		t.Fatalf("type ref = %q, want stable type key containing rendered type", typeRef.Key)
	}
	if typeRef.Type != typ.String {
		t.Fatalf("type ref sidecar = %v, want string", typeRef.Type)
	}

	valueRef := NewValueRef(0x2a, typ.String)
	if !strings.Contains(valueRef.Key, "value:000000000000002a:") || !strings.Contains(valueRef.Key, typeRef.Key) {
		t.Fatalf("value ref = %q, want value hash and projected type %q", valueRef.Key, typeRef.Key)
	}
	if valueRef.ProjectedType != typ.String {
		t.Fatalf("value ref sidecar = %v, want string", valueRef.ProjectedType)
	}
}
