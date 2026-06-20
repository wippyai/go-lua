-- Memory-model soundness guards: each rejection is a way a user could otherwise
-- feed the JIT a wrong-typed value. The checker must reject every one.

-- Array covariance: a write through a covariant alias of an opaque array must not
-- corrupt the source element type.
local function array_covariance(a: {string}): string
    local b: {string | number} = a
    b[1] = 42
    local s: string = a[1] -- expect-error
    return s
end

-- Map value type is invariant: no {[K]:any} alias over {[K]:string}.
local function map_invariance(m: {[string]: string}): nil
    local n: {[string]: any} = m -- expect-error
    return nil
end

-- A concrete cast adopts its type for inference but cannot prove an obligation.
local function need(o: {name: string}): number return 1 end
local function cast_no_launder(y: any): number
    return need(y as {name: string}) -- expect-error
end

-- Missing required field on a call argument.
local function missing_field_arg(): number
    return need({}) -- expect-error
end

-- Missing required field on a return.
local function missing_field_return(): {name: string}
    return {} -- expect-error
end

-- Floor division of float operands is a float, not an integer.
local function floor_div(a: number, b: number): integer
    local x: integer = a // b -- expect-error
    return x
end

-- A return-position operator expression is checked against the return type.
local function return_operator(a: number, b: number): integer
    return a // b -- expect-error
end

-- gmatch yields strings; a loop variable is a string, not any.
local function gmatch_iter(s: string): number
    for w in s:gmatch("%a+") do
        local n: number = w -- expect-error
        return n
    end
    return 0
end

-- An absent field of an empty table literal is nil, not an arbitrary top type.
local function empty_literal(): number
    local t = {}
    local g: fun(): number = t.run -- expect-error
    return g()
end

-- A multi-assignment under-supply leaves the surplus target nil.
local function one(): number return 1 end
local function undersupply(): number
    local a: number, b: number = one() -- expect-error
    return b
end

-- A callee that mutates a fresh argument covariantly cannot launder the write.
local function corrupt(w: {x: number | string}) w.x = "boom" end
local function fresh_escape(): number
    local narrow: {x: number} = {x = 1}
    corrupt(narrow)
    local n: number = narrow.x -- expect-error
    return n
end

-- An interprocedural mutation invalidates a field's guard narrowing.
local function clear(b: {value: string?}) b.value = nil end
local function interproc(box: {value: string?}): string
    if box.value then
        clear(box)
        local n: string = box.value -- expect-error
        return n
    end
    return "x"
end

return array_covariance, map_invariance, cast_no_launder, missing_field_arg,
    missing_field_return, floor_div, return_operator, gmatch_iter, empty_literal,
    undersupply, fresh_escape, interproc
