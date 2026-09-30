package regression

import "testing"

func TestCreatedTableHasCompleteEmptyDomain(t *testing.T) {
	checkBothModes(t, `
local function run(items: {string}): {[string]: boolean}
 local set: {[string]: boolean} = table.create(0, #items)
 for _, item in ipairs(items) do set[item] = true end
 return set
end
return run`, "")
}

func TestCreatedTableRejectsWrongValue(t *testing.T) {
	checkBothModes(t, `
local function run(): {[string]: boolean}
 local set: {[string]: boolean} = table.create(0, 1)
 set.bad = "wrong"
 return set
end
return run`, "cannot assign")
}
