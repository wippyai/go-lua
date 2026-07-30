package checktest

import "testing"

func TestImportedGenericConstructorMethodReturnDoesNotCollapseToAny(t *testing.T) {
	collectionCheck := CheckFile(`
type Collection<T> = {
    items: {T},
    count: (self: Collection<T>) -> number,
    add: (self: Collection<T>, item: T) -> (),
}

local M = {}

function M.new<T>(): Collection<T>
    local c: Collection<T> = {
        items = {},
        count = function(self: Collection<T>): number
            return #self.items
        end,
        add = function(self: Collection<T>, item: T)
            table.insert(self.items, item)
        end,
    }
    return c
end

return M
`, "collection.lua", WithStdlib())
	collection := moduleResultFromCheck("collection", collectionCheck)
	if len(collection.Errors) != 0 {
		t.Fatalf("collection diagnostics = %#v", collection.Errors)
	}

	result := Check(`
local collection = require("collection")

local nums = collection.new()
nums:add(1)
local count: number = nums:count()
	`, WithStdlib(), WithModule("collection", collection))
	if len(result.Diagnostics) != 0 {
		debug := ""
		if result.checked != nil {
			debug = callOutcomeDebug(result.checked.RootResult())
		}
		t.Fatalf("diagnostics = %#v, want imported generic constructor receiver method return to stay number\ncalls: %s", result.Diagnostics, debug)
	}
}
