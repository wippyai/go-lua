-- A callee that collects the keys of one of its formals into the array it
-- returns states a relation, not a value: the elements of that array are keys
-- of whatever container the caller bound to that formal. The callee names the
-- formal, the application names the argument, and the read at one of those
-- elements is decided against the caller's own container -- exactly as a read
-- at the key variable of a local enumeration is.
--
-- The relation answers to the enumeration's own revocations, measured from the
-- point it was established. A slot added to or removed from the container, a
-- container handed to a callee this analysis never read, an element the array
-- received from anywhere but that enumeration, and a callee that writes the
-- formal it enumerated each leave the array describing a state its container
-- has left.

local counts: {[string]: number} = {}

local function keys_of(source: {[string]: number})
    local out = {}
    for key in pairs(source) do
        table.insert(out, key)
    end
    return out
end

-- The relation holds: every element came from enumerating counts, and nothing
-- has moved a slot of counts since.
local names = keys_of(counts)
for _, name in ipairs(names) do
    local hits: number = counts[name]
end

-- Another container is not the one the relation names.
local other: {[string]: number} = {}
local other_names = keys_of(other)
for _, name in ipairs(other_names) do
    local hits: number = counts[name] -- expect-error
end

-- A store into the container after the relation was established leaves the
-- array naming slots the container no longer accounts for.
local mutated_names = keys_of(counts)
counts["added"] = 1
for _, name in ipairs(mutated_names) do
    local hits: number = counts[name] -- expect-error
end

-- An element the array received from anywhere but the enumeration is a key no
-- enumeration produced.
local fresh: {[string]: number} = {}
local mixed = keys_of(fresh)
table.insert(mixed, "literal")
for _, name in ipairs(mixed) do
    local hits: number = fresh[name] -- expect-error
end

-- A callee that writes the formal it enumerated returns keys of a container it
-- has itself since changed.
local written: {[string]: number} = {}
local function keys_and_write(source: {[string]: number})
    local out = {}
    for key in pairs(source) do
        table.insert(out, key)
    end
    source.extra = 1
    return out
end
local written_names = keys_and_write(written)
for _, name in ipairs(written_names) do
    local hits: number = written[name] -- expect-error
end

return names, other_names, mutated_names, mixed, written_names
