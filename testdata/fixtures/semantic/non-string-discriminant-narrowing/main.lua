type OK = {ok: true, value: string}
type ERR = {ok: false, error: string}
type R = OK | ERR

local function by_true(r: R): string
    if r.ok == true then
        return r.value
    end
    return r.error
end

local function by_not_false(r: R): string
    if r.ok ~= false then
        local s: string = r.value
        return s
    else
        local e: string = r.error
        return e
    end
end

type A = {code: 1, value: string}
type B = {code: 2, value: number}
type Coded = A | B

local function by_code(r: Coded): string
    if r.code == 1 then
        local s: string = r.value
        return s
    else
        local n: number = r.value
        return "number"
    end
end

type Box = {inner: R}

local function nested(box: Box): string
    if box.inner.ok == false then
        return box.inner.error
    end
    return box.inner.value
end

type Only = {ok: true, value: string}

local function impossible(x: Only): string
    if x.ok == false then
        x.nope()
    end
    return x.value
end

local only: Only = {ok = true, value = "ok"}
if only.ok == true then
    if only.ok ~= true then
        only.nope()
    end
end
