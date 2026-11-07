package regression

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/types/kind"

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
	a, aBody := recursiveBody(t, exportedField(t, result, "a"))
	b, bBody := recursiveBody(t, exportedField(t, result, "b"))
	if a.ID == b.ID {
		t.Fatalf("a and b share recursion identity %d", a.ID)
	}
	aArray, ok := aBody.(*typ.Array)
	if !ok || !isSelf(aArray.Element, b) {
		t.Fatalf("a is %v, want an array of b", aBody)
	}
	bArray, ok := bBody.(*typ.Array)
	if !ok || !isSelf(bArray.Element, a) {
		t.Fatalf("b is %v, want an array of a", bBody)
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

// A table that only looks like the self-referential table is a different
// table: writes into the self-referential table do not reach it.
func TestSelfReferentialTable_StructurallyEqualTableStaysSeparate(t *testing.T) {
	result := checkWithin(t, `
		local t = {tag = 1}
		t[1] = {back = t, other = {tag = 1}}
		t.x = "hi"
		local other = t[1].other
		if other then
			local absent: nil = other.x
		end
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
}

// A variable rebound to a new self-referential table names a new table; the
// first table keeps its own recursion identity.
func TestSelfReferentialTable_ReboundVariableIsANewTable(t *testing.T) {
	result := checkWithin(t, `
		local t = {}
		t[1] = t
		local old = t
		t = {tag = "new"}
		t[1] = t
		return { old = old, t = t }
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
	old, oldBody := recursiveBody(t, exportedField(t, result, "old"))
	current, _ := recursiveBody(t, exportedField(t, result, "t"))
	if old.ID == current.ID {
		t.Fatalf("the rebound table shares recursion identity %d with the first table", old.ID)
	}
	if _, ok := oldBody.(*typ.Array); !ok {
		t.Fatalf("old is %v, want mu X. X[] without the second table's fields", old)
	}
}

// Writing nil into a self-referential table removes an entry, so its entries
// read as possibly absent.
func TestSelfReferentialTable_NilWriteMakesEntriesOptional(t *testing.T) {
	present := checkWithin(t, `
		local t = {}
		t[1] = t
		local entry: {any} = t[1]
	`)
	if present.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(present.Errors))
	}
	removed := checkWithin(t, `
		local t = {}
		t[1] = t
		t[1] = nil
		local entry: {any} = t[1]
	`)
	if !removed.HasError() {
		t.Fatal("an entry of the table after a nil write must read as possibly absent")
	}
}

// A write through a self reference is a write into the table itself.
func TestSelfReferentialTable_WriteThroughSelfReference(t *testing.T) {
	result := checkWithin(t, `
		local t = {}
		t[1] = t
		t[1][2] = "hi"
		return { t = t }
	`)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
	rec, body := recursiveBody(t, exportedField(t, result, "t"))
	array, ok := body.(*typ.Array)
	if !ok {
		t.Fatalf("t is %v, want an array", rec)
	}
	hasSelf, hasString := false, false
	members := []typ.Type{array.Element}
	if u, ok := array.Element.(*typ.Union); ok {
		members = u.Members
	}
	for _, m := range members {
		hasSelf = hasSelf || isSelf(m, rec)
		hasString = hasString || unwrap.Alias(m).Kind() == kind.String || isStringLiteral(m)
	}
	if !hasSelf || !hasString {
		t.Fatalf("t is %v, want mu X. (X | string)[]", rec)
	}
}

func isStringLiteral(t typ.Type) bool {
	lit, ok := t.(*typ.Literal)
	if !ok {
		return false
	}
	_, isString := lit.Value.(string)
	return isString
}
