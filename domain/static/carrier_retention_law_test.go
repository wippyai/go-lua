package static

import (
	"reflect"
	"strings"
	"testing"
)

// TestStaticCarrierBoundaryGate makes the Static seal boundary structural,
// rather than relying on a convention that construction-only Link handles are
// cleared later. It recursively follows Static-owned state types and rejects
// any direct Boundary, Project, Program, or Term carrier.
// External sealed scalar/domain types are leaves: their own packages own their
// retention gates.
func TestStaticCarrierBoundaryGate(t *testing.T) {
	roots := []reflect.Type{
		reflect.TypeOf(Authority{}),
		reflect.TypeOf(MountContext{}),
		reflect.TypeOf(MountedProgram{}),
		reflect.TypeOf(resultRow{}),
		reflect.TypeOf(Symbolic{}),
		reflect.TypeOf(RuntimeSubject{}),
	}
	seen := make(map[reflect.Type]struct{})
	for _, root := range roots {
		staticCarrierType(t, root, seen)
	}
}

func staticCarrierType(t *testing.T, typ reflect.Type, seen map[reflect.Type]struct{}) {
	t.Helper()
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Map {
		staticCarrierType(t, typ.Key(), seen)
		staticCarrierType(t, typ.Elem(), seen)
		return
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	if _, duplicate := seen[typ]; duplicate {
		return
	}
	seen[typ] = struct{}{}
	path := typ.PkgPath()
	if strings.Contains(path, "/program/link/boundary") || strings.Contains(path, "/program/link/project") || path == "github.com/wippyai/go-lua/analysis/program" {
		t.Fatalf("Static retained forbidden carrier %s from %s", typ, path)
	}
	// Recurse only through Static-owned types. Crossing into another sealed
	// domain would turn this into a test of that domain's private state.
	if path != "" && path != "github.com/wippyai/go-lua/domain/static" {
		return
	}
	for index := 0; index < typ.NumField(); index++ {
		staticCarrierType(t, typ.Field(index).Type, seen)
	}
}
