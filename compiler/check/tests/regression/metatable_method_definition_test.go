package regression

import "testing"

// A method definition (function T:m) is a field write on T: a value built
// from T after the definition sees the method.
func TestMethodDefinitionVisibleThroughIndexMetatable(t *testing.T) {
	source := `
local Resource = {}
Resource.__index = Resource
function Resource:close(): integer return 1 end
local handle = setmetatable({}, Resource)
local n: integer = handle:close()
`
	checkBothModes(t, source, "")
}

func TestMethodDefinitionVisibleThroughIndexTable(t *testing.T) {
	source := `
local Resource = {}
function Resource.close(self): integer return 1 end
local handle = setmetatable({}, {__index = Resource})
local n: integer = handle:close()
`
	checkBothModes(t, source, "")
}

func TestUndefinedMethodThroughIndexMetatableIsRejected(t *testing.T) {
	source := `
local Resource = {}
Resource.__index = Resource
function Resource:close(): integer return 1 end
local handle = setmetatable({}, Resource)
handle:open()
`
	checkBothModes(t, source, "expected function, got nil")
}
