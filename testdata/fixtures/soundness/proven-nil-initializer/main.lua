-- An initializer that the analysis proved nil refutes the declaration that
-- admits no nil. The front writes the read's result into the declared cell and
-- annotates it in place, so the claim reads its own target; the value is still
-- the initializer's, not the cell's default.
local border: {string} = { "a", "b" }
local n = #border
border[2] = nil
local v: string = border[n] -- expect-error
print(v)

-- The key spelling never decides it.
local dynamic: {string} = { "a", "b" }
local m = #dynamic
local j = 2
dynamic[j] = nil
local w: string = dynamic[m] -- expect-error
print(w)

-- A container reached through a member lens carries the same obligation.
local box: {items: {string}} = { items = { "a", "b" } }
local k = #box.items
box.items[2] = nil
local b: string = box.items[k] -- expect-error
print(b)

-- A declaration with no initializer reads its own Lua nil slot. That slot is
-- the declared local's downstream contract, not an assignment of nil to it.
local bare: string
print(bare)

-- An initializer the analysis cannot refute keeps its declaration.
local function text(): string return "x" end
local proven: string = text()
print(proven)

-- A declaration that admits nil is satisfied by the same proven-nil read.
local admits: {string} = { "a", "b" }
local a = #admits
admits[2] = nil
local optional: string? = admits[a]
print(optional)
