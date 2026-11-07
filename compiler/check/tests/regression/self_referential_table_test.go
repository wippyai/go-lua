package regression

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// checkWithin checks source and fails the test when the checker does not
// finish within the deadline.
func checkWithin(t *testing.T, source string) *testutil.ModuleResult {
	t.Helper()
	done := make(chan *testutil.ModuleResult, 1)
	go func() {
		done <- testutil.CheckAndExport(source, "self_table", testutil.WithStdlib())
	}()
	select {
	case result := <-done:
		if nonConverged := nonConvergenceWarnings(result.Session.Diagnostics); len(nonConverged) > 0 {
			t.Fatalf("unexpected non-convergence warnings: %v", nonConverged)
		}
		return result
	case <-time.After(30 * time.Second):
		t.Fatal("type checking did not finish")
		return nil
	}
}

// exportedField returns the type of the named field of the module export.
func exportedField(t *testing.T, result *testutil.ModuleResult, name string) typ.Type {
	t.Helper()
	record, ok := unwrap.Alias(result.Manifest.Export).(*typ.Record)
	if !ok {
		t.Fatalf("export is %v, want a record", result.Manifest.Export)
	}
	field := record.GetField(name)
	if field == nil {
		t.Fatalf("export %v has no field %q", record, name)
	}
	return field.Type
}

// recursiveBody returns the body of a recursive type.
func recursiveBody(t *testing.T, got typ.Type) (*typ.Recursive, typ.Type) {
	t.Helper()
	rec, ok := got.(*typ.Recursive)
	if !ok || rec.Body == nil {
		t.Fatalf("type is %v, want a recursive table type", got)
	}
	return rec, rec.Body
}

// isSelf reports whether t refers to the recursive type rec.
func isSelf(t typ.Type, rec *typ.Recursive) bool {
	other, ok := t.(*typ.Recursive)
	return ok && other.ID == rec.ID
}

func TestSelfReferentialTable_IndexWrite(t *testing.T) {
	result := checkWithin(t, `
		local nodes = {}
		nodes[1] = nodes
		local deep = nodes[1][1][1]
		local count: integer = #deep
		return { nodes = nodes }
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
	rec, body := recursiveBody(t, exportedField(t, result, "nodes"))
	array, ok := body.(*typ.Array)
	if !ok || !isSelf(array.Element, rec) {
		t.Fatalf("nodes is %v, want mu X. X[]", rec)
	}
}

func TestSelfReferentialTable_DynamicKeyWrite(t *testing.T) {
	result := checkWithin(t, `
		local function index(key: string)
			local nodes = {}
			nodes[key] = nodes
			return nodes
		end
		return { index = index }
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
	fn, ok := unwrap.Alias(exportedField(t, result, "index")).(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("index is %v, want a function returning the table", exportedField(t, result, "index"))
	}
	rec, body := recursiveBody(t, fn.Returns[0])
	var key, value typ.Type
	switch shape := body.(type) {
	case *typ.Map:
		key, value = shape.Key, shape.Value
	case *typ.Record:
		key, value = shape.MapKey, shape.MapValue
	}
	if !typ.TypeEquals(key, typ.String) || !isSelf(value, rec) {
		t.Fatalf("nodes is %v, want mu X. {[string]: X}", rec)
	}
}

func TestSelfReferentialTable_TableInsert(t *testing.T) {
	result := checkWithin(t, `
		local nodes = {}
		table.insert(nodes, nodes)
		local deep = nodes[1][1][1]
		local count: integer = #deep
		return { nodes = nodes }
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
	rec, body := recursiveBody(t, exportedField(t, result, "nodes"))
	array, ok := body.(*typ.Array)
	if !ok || !isSelf(array.Element, rec) {
		t.Fatalf("nodes is %v, want mu X. X[]", rec)
	}
}

func TestSelfReferentialTable_MutualReferences(t *testing.T) {
	result := checkWithin(t, `
		local a = {}
		local b = {}
		a[1] = b
		b[1] = a
		local from_a = a[1][1][1][1]
		local from_b = b[1][1][1][1]
		local count: integer = #from_a + #from_b
		return { a = a, b = b }
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
	a := exportedField(t, result, "a")
	rec, _ := recursiveBody(t, a)
	b, ok := unwrap.Alias(exportedField(t, result, "b")).(*typ.Array)
	if !ok || !typ.TypeEquals(b.Element, rec) {
		t.Fatalf("b is %v, want an array of a (%v)", exportedField(t, result, "b"), rec)
	}
}

func TestSelfReferentialTable_FunctionReturningTable(t *testing.T) {
	result := checkWithin(t, `
		local nodes = {}
		nodes[1] = function() return nodes end
		return { nodes = nodes }
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
	array, ok := unwrap.Alias(exportedField(t, result, "nodes")).(*typ.Array)
	if !ok {
		t.Fatalf("nodes is %v, want an array of functions", exportedField(t, result, "nodes"))
	}
	fn, ok := unwrap.Alias(array.Element).(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("nodes is %v, want an array of functions returning the table", array)
	}
}

func TestSelfReferentialTable_InsideFunctionResult(t *testing.T) {
	result := checkWithin(t, `
		local function build()
			local nodes = {}; nodes[1] = nodes
			return { nodes = nodes }
		end
		local graph = build()
		local deep = graph.nodes[1][1]
		local count: integer = #deep
		return { build = build }
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestSelfReferentialTable_StaysTableTyped(t *testing.T) {
	result := checkWithin(t, `
		local nodes = {}
		nodes[1] = nodes
		local n: number = nodes[1]
	`)
	if !result.HasError() {
		t.Fatal("assigning the self-referential table to number must be an error")
	}
}
