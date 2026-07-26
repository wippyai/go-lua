-- An undeclared formal states no contract, so the caller's own published
-- argument type is what that formal holds for the duration of one application.
-- The body evaluates against it: the keys pairs draws from the argument are the
-- argument's key type, and the array the body accumulates them into carries that
-- element out through its return. The caller reads the element the body derived,
-- and a claim naming any other element is refuted against it.
--
-- The obligations run both ways. The same body applied to a differently typed
-- argument answers with that argument's key type, and an argument whose type is
-- unpublished leaves the result exactly as unconstrained as it is without a
-- caller type to instantiate from. A formal the body assigns is no longer the
-- argument past that assignment, so it takes no instantiation at all.

local function sorted_keys(t)
    local out = {}
    for k in pairs(t) do
        table.insert(out, k)
    end
    table.sort(out)
    return out
end

local by_name: {[string]: number} = {}
local names = sorted_keys(by_name)
local name: string = names[1]
local name_wrong: number = names[1] -- expect-error

-- The same body, a different argument: the result element follows the argument
-- this application binds, not a template shared with the application above.
local by_id: {[number]: string} = {}
local ids = sorted_keys(by_id)
local id: number = ids[1]
local id_wrong: string = ids[1] -- expect-error

-- An argument with no published type instantiates nothing, so the result stays
-- unconstrained rather than borrowing an element from another application.
local function opaque() end
local unknown_keys = sorted_keys(opaque())
local unknown_first: string = unknown_keys[1] -- expect-error

-- The array the body builds is itself an ordinary array result: an element
-- claim on the whole container is proven, and a mismatched one is refuted.
local names_array: {string} = names
local names_array_wrong: {number} = names -- expect-error

-- A body that assigns its own formal no longer holds the argument's type there,
-- so this application publishes no element.
local function reassigned_keys(t, replace)
    if replace then
        t = {}
    end
    local out = {}
    for k in pairs(t) do
        table.insert(out, k)
    end
    return out
end

local reassigned = reassigned_keys(by_name, false)
local reassigned_first: string = reassigned[1] -- expect-error

return name, name_wrong, id, id_wrong, unknown_first, names_array,
    names_array_wrong, reassigned_first
