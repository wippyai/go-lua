package engine

import (
	"context"
	"reflect"
	"testing"
)

// TestSolveWithDiagnosticsDisabledCollectsNothing pins the full counter
// contract behind newSolveDiagnosticState's flags==0 early return
// (runtime_diagnostics.go): a zero Presentation.Flags must collect nothing,
// not merely report Flags==0. TestSolveWithDiagnosticsDisabledParity already
// pins Flags and len(Rows); this test enumerates every remaining numeric
// field on SolveDiagnostics by reflection so a future field added to the
// struct is checked automatically instead of silently passing unexamined.
func TestSolveWithDiagnosticsDisabledCollectsNothing(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 3, nil, nil)
	state, status, diagnostics := fixture.solver.SolveWithDiagnostics(context.Background(), SolveDiagnosticOptions{})
	if state == nil || status != SolveComplete {
		t.Fatalf("disabled-diagnostics solve state=%t status=%v", state != nil, status)
	}
	if diagnostics.Rows != nil && len(diagnostics.Rows) != 0 {
		t.Fatalf("Rows = %#v, want nil or empty", diagnostics.Rows)
	}

	value := reflect.ValueOf(diagnostics)
	typ := value.Type()
	checked := 0
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.Name == "Failure" || field.Name == "Rows" {
			continue
		}
		checked++
		fieldValue := value.Field(index)
		switch fieldValue.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if fieldValue.Uint() != 0 {
				t.Fatalf("field %s = %d, want 0 when diagnostics are disabled", field.Name, fieldValue.Uint())
			}
		case reflect.Array:
			for element := 0; element < fieldValue.Len(); element++ {
				entry := fieldValue.Index(element)
				if entry.Kind() != reflect.Uint64 {
					t.Fatalf("field %s[%d] has unhandled element kind %s; extend this law's coverage", field.Name, element, entry.Kind())
				}
				if entry.Uint() != 0 {
					t.Fatalf("field %s[%d] = %d, want 0 when diagnostics are disabled", field.Name, element, entry.Uint())
				}
			}
		default:
			t.Fatalf("field %s has unhandled kind %s; extend this law's coverage", field.Name, fieldValue.Kind())
		}
	}
	if checked == 0 {
		t.Fatal("reflection walk found no counter fields on SolveDiagnostics; struct shape changed under this law")
	}
}
