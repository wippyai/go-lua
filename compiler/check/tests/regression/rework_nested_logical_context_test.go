package regression

import "testing"

func TestNestedLogicalFreshAlternativeUsesContext(t *testing.T) {
	checkBothModes(t, `
type Item = {id: string}
local function run(items: {Item}?)
 local config: {items: {Item}} = {items = items or {}}
 return config
end
return run`, "")
}

func TestNestedLogicalSharedAlternativeRetainsDomain(t *testing.T) {
	checkBothModes(t, `
type Item = {id: string}
local function run(items: {Item}?)
 local e = {}
 local config: {items: {Item}} = {items = items or e}
 local function push(values: {string}) table.insert(values, "wrong") end
 push(e)
 return config
end
return run`, "cannot assign")
}

// The alternatives are fresh literals. The declared return must establish
// their shared mutable field domain before their read types are joined.
func TestNestedLogicalRecordReturnUsesContext(t *testing.T) {
	checkBothModes(t, `
type T = {code: "one" | "two"}
local function run(flag: boolean): {result: T}
 return {result = flag and {code = "one"} or {code = "two"}}
end
return run`, "")
}

func TestNestedLogicalRecordReturnChecksDomain(t *testing.T) {
	checkBothModes(t, `
type T = {code: "one" | "two"}
local function run(flag: boolean): {result: T}
 return {result = flag and {code = "one"} or {code = "wrong"}}
end
return run`, "cannot return")
}
