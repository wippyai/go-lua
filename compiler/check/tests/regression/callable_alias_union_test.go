package regression

import "testing"

func TestValueSourceActionAliasDispatch(t *testing.T) {
	checkBothModes(t, `
type Params = {[string]: unknown}
type ActionFn = (Params) -> (unknown, string?)
local actions: {[string]: ActionFn} = {}
function actions.run(params: Params): (unknown, string?) return params, nil end
local function handler(action: string, params: Params)
    local fn = actions[action]
    if not fn then return nil, "unknown action" end
    return fn(params)
end
return handler`, "")
}

func TestValueSourceActionAliasUnionDispatch(t *testing.T) {
	checkBothModes(t, `
type Params = {[string]: unknown}
type ActionFn = (Params) -> (unknown, string?)
local actions: {[string]: ActionFn} = {}
function actions.first(params) return params.value, nil end
function actions.second(params) return "result", nil end
local function handler(action: string, params: Params)
    local fn = actions[action]
    if not fn then return nil, "unknown action" end
    local result, err = fn(params)
    return result, err
end
return handler`, "")
}

func TestCallableAliasUnionControls(t *testing.T) {
	checkBothModes(t, `
type Fn = (number) -> number
type Bad = string
local function run(fn: Bad)
 return fn(1)
end
return run`, "expected function")
	checkBothModes(t, `
type Fn = (number) -> number
local function run(fn: Fn)
 return fn("bad")
end
return run`, "argument 1:")
	checkBothModes(t, `
type Fn = (number) -> number
local function run(fn: Fn?)
 return fn(1)
end
return run`, "cannot call optional")
}
