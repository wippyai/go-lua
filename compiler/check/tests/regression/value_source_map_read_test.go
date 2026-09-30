package regression

import "testing"

func TestValueSourceEmptyUnionMemberContributesNil(t *testing.T) {
	checkBothModes(t, `
type UM = {[string]: unknown}
type OS = { object: UM }
local function f(ctx: {objs: unknown}, k: string): (OS?, string?)
    local objects = type(ctx.objs) == "table" and (ctx.objs :: {[string]: UM}) or {}
    local object = objects[k]
    if not object then return nil, "no" end
    return { object = object }, nil
end
return f`, "")
}

func TestValueSourceEmptyUnionWrongValueStillRejected(t *testing.T) {
	checkBothModes(t, `
type OS = { object: {[string]: unknown} }
local function f(k: string): OS?
    local objects = {["x"] = "bad"}
    local object = objects[k]
    if not object then return nil end
    return { object = object }
end
return f`, "cannot return")
}

func TestValueSourceMissingMapKeyStillRejected(t *testing.T) {
	checkBothModes(t, `
type UM = {[string]: unknown}
local function f(ctx: {objs: unknown}, k: string)
    local objects = type(ctx.objs) == "table" and (ctx.objs :: {[string]: UM}) or {}
    local object: UM = objects[k]
end
return f`, "cannot assign")
}
