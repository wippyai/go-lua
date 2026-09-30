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
