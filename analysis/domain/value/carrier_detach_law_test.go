package value

import (
	"reflect"
	"strings"
	"testing"
)

// TestPublishedValueRowsDoNotRetainLinkAuthorityCarriers is a structural
// ownership gate.  The one construction-only seal context is intentionally
// excluded: Seal clears it before publishing Schema.  Every published row,
// map key, and hot operand must be representable without a Boundary, Host, or
// Project owner pointer.
func TestPublishedValueRowsDoNotRetainLinkAuthorityCarriers(t *testing.T) {
	for _, row := range []reflect.Type{
		reflect.TypeOf(Schema{}),
		reflect.TypeOf(atomRow{}),
		reflect.TypeOf(referenceRow{}),
		reflect.TypeOf(coordinateRow{}),
		reflect.TypeOf(capabilitySeedRow{}),
		reflect.TypeOf(hostMember{}),
		reflect.TypeOf(SourceSeed{}),
		reflect.TypeOf(SourceResult{}),
		reflect.TypeOf(GlobalBootstrapResult{}),
	} {
		assertDetachedValueRow(t, row, map[reflect.Type]bool{})
	}
}

func assertDetachedValueRow(t testing.TB, typ reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	if typ == nil || seen[typ] {
		return
	}
	seen[typ] = true
	if strings.HasPrefix(typ.PkgPath(), "github.com/wippyai/go-lua/program/link/") {
		t.Fatalf("published Value row retains Link carrier %s", typ)
	}
	if typ.PkgPath() != "" && typ.PkgPath() != "github.com/wippyai/go-lua/analysis/domain/value" {
		return
	}
	switch typ.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		assertDetachedValueRow(t, typ.Elem(), seen)
	case reflect.Map:
		assertDetachedValueRow(t, typ.Key(), seen)
		assertDetachedValueRow(t, typ.Elem(), seen)
	case reflect.Struct:
		for field := 0; field < typ.NumField(); field++ {
			current := typ.Field(field)
			assertDetachedValueRow(t, current.Type, seen)
		}
	}
}
