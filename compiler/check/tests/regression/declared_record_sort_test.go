package regression

import "testing"

func TestSortPreservesDeclaredLiteralUnionElement(t *testing.T) {
	for _, write := range []string{
		``,
		`snapshot.bindings[#snapshot.bindings + 1] = binding`,
		`table.insert(snapshot.bindings, binding)`,
	} {
		t.Run(write, func(t *testing.T) {
			checkBothModes(t, `
type Binding = {binding_id: string, state: "compatible" | "incompatible"}
type Snapshot = {bindings: {Binding}}
local function read(binding: Binding): Snapshot
 local snapshot: Snapshot = {bindings = {}}
 `+write+`
 table.sort(snapshot.bindings, function(left: Binding, right: Binding): boolean
  return left.binding_id < right.binding_id
 end)
 return snapshot
end
return read`, "")
		})
	}
}

func TestSortPreservesDeclaredLiteralUnionRows(t *testing.T) {
	checkBothModes(t, `
type OptionRow = {name: string, kind: "enum" | "text"}
local function options(names: {string}): {OptionRow}
 local rows: {OptionRow} = {}
 for _, name in ipairs(names) do
  rows[#rows + 1] = {name = name, kind = "enum"}
 end
 table.sort(rows, function(left: OptionRow, right: OptionRow): boolean
  return left.name < right.name
 end)
 return rows
end
return options`, "")
}

func TestSortRejectsNarrowerLiteralUnionComparator(t *testing.T) {
	checkBothModes(t, `
type Binding = {binding_id: string, state: "compatible" | "incompatible"}
type Compatible = {binding_id: string, state: "compatible"}
local function read(bindings: {Binding})
 table.sort(bindings, function(left: Compatible, right: Compatible): boolean
  return left.binding_id < right.binding_id
 end)
end
return read`, "unsatisfiable bounds:")
}

func TestSortRejectsStringFieldForLiteralUnionComparator(t *testing.T) {
	checkBothModes(t, `
type OptionRow = {name: string, kind: "enum" | "text"}
local function options(rows: {{name: string, kind: string}})
 table.sort(rows, function(left: OptionRow, right: OptionRow): boolean
  return left.name < right.name
 end)
end
return options`, "unsatisfiable bounds:")
}

func TestDeclaredLiteralUnionRowsRejectInvalidRow(t *testing.T) {
	checkBothModes(t, `
type OptionRow = {name: string, kind: "enum" | "text"}
local function options(rows: {OptionRow})
 local row: OptionRow = {name = "bad", kind = "other"}
 rows[#rows + 1] = row
end
return options`, "cannot assign")
}
