-- A write inside a loop body is a write on the trips the loop runs. Running no
-- trip is an execution too, so a slot the loop introduces is possible after the
-- loop, never established -- whether the body writes it on every path or only
-- on some.

-- An unconditional body write still leaves the slot possible: an empty iterator
-- runs the body zero times.
local unconditional: {string} = {}
for _, s in ipairs({"a"}) do
    unconditional[1] = s
end
local first: string = unconditional[1] -- expect-error
print(first)

-- A conditional body write is possible for the same reason and one more.
local conditional: {string} = {}
for _, s in ipairs({"a"}) do
    if s ~= "" then
        conditional[1] = s
    end
end
local guarded: string = conditional[1] -- expect-error
print(guarded)

-- A slot that already held a value keeps what the join of both admits: the
-- pre-loop write is what the zero-trip execution leaves, so the member stays a
-- string rather than becoming optional.
local seeded: {name: string} = {name = "start"}
for _, s in ipairs({"a"}) do
    seeded.name = s
end
local kept: string = seeded.name
print(kept)

-- The same write outside a loop establishes the slot outright.
local straight: {string} = {}
straight[1] = "a"
local exact: string = straight[1]
print(exact)
