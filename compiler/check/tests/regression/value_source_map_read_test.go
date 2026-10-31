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

func TestValueSourceNestedDynamicFieldPresence(t *testing.T) {
	checkBothModes(t, `-- Writing a field of an entry reached through a variable key, t[k].f = v,
-- keeps the entries records whatever path the value comes from
-- (dataflow workflow_state: marking a child node cancelled from a status table).
local STATUS = { PENDING = "pending", CANCELLED = "cancelled" }
local methods = {}

function methods:cancel(pending: { [string]: string }, group_key: string)
    local function in_group(child_id: string): boolean
        local child = self.nodes[child_id]
        return child and type(child.metadata) == "table" and child.metadata[group_key] == true
    end
    for child_id, cached_status in pairs(pending) do
        if cached_status == STATUS.PENDING and in_group(child_id) then
            self.nodes[child_id].status = STATUS.CANCELLED
        end
    end
end

return methods
`, "")
}

func TestValueSourceDeclaredMapAbsenceStillRejected(t *testing.T) {
	checkBothModes(t, `
local function f(input: {[string]: boolean}, key: string): boolean
    local child = input[key]
    return child
end
return f`, "cannot return boolean?")
}
