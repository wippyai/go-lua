package read_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
)

// Reader is deliberately a sealed value handle.  This external-package law
// prevents the old injectable-interface authority from returning: a sibling
// package must not be able to implement a reader that lies about its root,
// layout, scan results, or scope algebra.
func TestReaderIsOpaqueAndZeroValueRefuses(t *testing.T) {
	typeOfReader := reflect.TypeOf(read.Reader{})
	if typeOfReader.Kind() != reflect.Struct {
		t.Fatalf("Reader became implementable interface: %v", typeOfReader.Kind())
	}
	for field := 0; field < typeOfReader.NumField(); field++ {
		if typeOfReader.Field(field).PkgPath == "" {
			t.Fatalf("Reader exposes injectable field %q", typeOfReader.Field(field).Name)
		}
	}
	var zero read.Reader
	if zero.Available() || zero.Layout().Available() {
		t.Fatal("zero Reader redeemed an authority")
	}
	if completed, valid := zero.Scan(func(read.Row) bool { return true }); completed || valid {
		t.Fatal("zero Reader scanned as valid")
	}
}

// Row retains an unexported package-owned marker.  Even though its values are
// borrowed by operators, an external package cannot manufacture a row whose
// ID, scope, lineage, or cells are inconsistent with a committed Reader.
func TestRowCannotBeImplementedOutsideReadPackage(t *testing.T) {
	typeOfRow := reflect.TypeOf((*read.Row)(nil)).Elem()
	marker, ok := typeOfRow.MethodByName("rowFrom")
	if !ok || marker.PkgPath == "" {
		t.Fatal("Row lost its package-owned authentication marker")
	}
}
