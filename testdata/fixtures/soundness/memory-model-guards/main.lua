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

-- A concrete cast is runtime validation on the normal path.
local function need(o: {name: string}): number return 1 end
local function cast_runtime_validate(y: any): number
    return need(y as {name: string})
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

-- A non-nil assertion on a provably-nil value always fails at runtime; it must
-- not launder nil into a non-nil type the JIT would trust.
local function nonnil_assert_always_nil(): string
    local x: nil = nil
    return x! -- expect-error
end

-- The nil branch of a guard proves the operand is nil there, so the assertion
-- still always fails.
local function nonnil_assert_narrowed_nil(x: string?): string
    if x == nil then
        return x! -- expect-error
    end
    return x
end

-- A covariant record alias widens the source field, so a write through the alias
-- (here deferred into a closure) cannot launder a wider value back as the narrow
-- type. narrow.x reads as number | string after the alias.
local function covariant_alias_closure(): number
    local narrow: {x: number} = {x = 1}
    local wide: {x: number | string} = narrow
    local function leak() wide.x = "boom" end
    leak()
    local n: number = narrow.x -- expect-error
    return n
end

-- A cast creates a wider mutable view of narrow. The cast is runtime-validated,
-- but the aliasing still lets a write through the wide view corrupt narrow, so
-- the operand widens at the cast.
local function covariant_cast(): number
    local narrow: {x: number} = {x = 1}
    local wide = narrow as {x: number | string}
    wide.x = "boom"
    local n: number = narrow.x -- expect-error
    return n
end

-- Covariant alias via ordinary assignment, mutated through a closure.
local function covariant_reassign(): number
    local narrow: {x: number} = {x = 1}
    local wide: {x: number | string} = {x = 2}
    wide = narrow
    local function leak() wide.x = "boom" end
    leak()
    local n: number = narrow.x -- expect-error
    return n
end

-- narrow stored into a wider record-field slot, mutated through a closure via the
-- container.
local function covariant_field_store(): number
    local narrow: {x: number} = {x = 1}
    local holder: {ref: {x: number | string}} = {ref = narrow}
    local function leak() holder.ref.x = "boom" end
    leak()
    local n: number = narrow.x -- expect-error
    return n
end

-- narrow stored into a wider array element, mutated through a closure.
local function covariant_index_store(): number
    local narrow: {x: number} = {x = 1}
    local sink: {{x: number | string}} = {narrow}
    local function leak() sink[1].x = "boom" end
    leak()
    local n: number = narrow.x -- expect-error
    return n
end

-- A sub-object aliased to a wider local view, mutated through a closure; the
-- ancestor narrow.inner widens so it cannot re-project the narrow field.
local function covariant_subobject(): number
    local narrow: {inner: {x: number}} = {inner = {x = 1}}
    local wideinner: {x: number | string} = narrow.inner
    local function leak() wideinner.x = "boom" end
    leak()
    local n: number = narrow.inner.x -- expect-error
    return n
end

-- A callee that stores its parameter into a wider returned container exposes the
-- argument across the call boundary: narrow becomes reachable through the wider
-- returned reference, so a write through it corrupts narrow. The argument widens
-- at the call to the return-member type.
local function ibox(o: {x: number | string}): {ref: {x: number | string}} return {ref = o} end
local function covariant_interproc(): number
    local narrow: {x: number} = {x = 1}
    local h = ibox(narrow)
    h.ref.x = "boom"
    local n: number = narrow.x -- expect-error
    return n
end

-- A callee that stores one parameter into another parameter's wider field
-- exposes the argument across the call: narrow becomes reachable through holder's
-- wider .ref, so a write corrupts it. The argument widens to the destination
-- member type.
local function ilink(dst: {ref: {x: number | string}}, o: {x: number | string}) dst.ref = o end
local function covariant_interproc_param(): number
    local narrow: {x: number} = {x = 1}
    local holder: {ref: {x: number | string}} = {ref = {x = 0}}
    ilink(holder, narrow)
    holder.ref.x = "boom"
    local n: number = narrow.x -- expect-error
    return n
end

-- A callee that stores its parameter into a captured sink exposes the argument
-- across the call at the sink slot type.
local isink: {ref: {x: number | string}} = {ref = {x = 0}}
local function istash(o: {x: number | string}) isink.ref = o end
local function covariant_interproc_sink(): number
    local narrow: {x: number} = {x = 1}
    istash(narrow)
    isink.ref.x = "boom"
    local n: number = narrow.x -- expect-error
    return n
end

return array_covariance, map_invariance, cast_runtime_validate, missing_field_arg,
    missing_field_return, floor_div, return_operator, gmatch_iter, empty_literal,
    undersupply, fresh_escape, interproc, nonnil_assert_always_nil,
    nonnil_assert_narrowed_nil, covariant_alias_closure, covariant_cast,
    covariant_reassign, covariant_field_store, covariant_index_store,
    covariant_subobject, covariant_interproc, covariant_interproc_param,
    covariant_interproc_sink
