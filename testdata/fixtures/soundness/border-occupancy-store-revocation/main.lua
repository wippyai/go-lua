-- A length term names the border its container had when the term was taken. The
-- closed inventory licenses a read at that border because slot one is occupied,
-- so 0 is not a border this table can return. A store that can empty a slot may
-- have retracted the border since, and the remembered length then names a slot
-- the container no longer occupies.
local retracted: {string} = { "a" }
retracted[3] = "c"
local n = #retracted
retracted[3] = nil
local v: string = retracted[n] -- expect-error
print(v)

-- A key the analysis cannot resolve addresses a slot just as directly.
local function slot(): number return 3 end
local opaque: {string} = { "a" }
opaque[3] = "c"
local m = #opaque
local k = slot()
opaque[k] = nil
local w: string = opaque[m] -- expect-error
print(w)

-- With nothing between the length and the read the inventory still proves slot
-- one occupied, so every border the operator may return names a written slot.
local clean: {string} = { "a" }
clean[3] = "c"
local c = #clean
local cv: string = clean[c]
print(cv)

-- A store that lands on another container revokes nothing about this one.
local kept: {string} = { "a" }
kept[3] = "c"
local other: {string} = { "x" }
local t = #kept
other[1] = nil
local tv: string = kept[t]
print(tv)

-- A length taken after the store describes the container as it stands then.
local after: {string} = { "a" }
after[3] = "c"
after[3] = nil
local a = #after
local av: string = after[a]
print(av)
