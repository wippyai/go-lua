package regression

import "testing"

func TestValueSourceAnyMapArguments(t *testing.T) {
	checkModes(t, `
type Map = {[string]: any}
local function replay(component: string, port: string?, key: string?) end
local function run(args: any)
 local a = type(args) == "table" and (args :: Map) or {}
 replay("component", a.source_port, nil)
end
return run`, "", "argument 2: expected string?, got any")
	checkModes(t, `
type Map = {[string]: any}
local function replay(component: string, port: string?, key: string?) end
local function run(args: any)
 local a = type(args) == "table" and (args :: Map) or {}
 replay("component", nil, a.item_key)
end
return run`, "", "argument 3: expected string?, got any")
	checkBothModes(t, `
type Map = {[string]: any}
local function replay(component: string, port: string?, key: string?) end
local function run(args: any)
 local a = type(args) == "table" and (args :: Map) or {}
 local port = type(a.source_port) == "string" and a.source_port or nil
 local key = type(a.item_key) == "string" and a.item_key or nil
 replay("component", port, key)
end
return run`, "")
}

func TestValueSourceAnyCommandReturn(t *testing.T) {
	checkModes(t, `
type Map = {[string]: any}
local function node_cmd(cmds: any): Map?
 for _, c in ipairs(type(cmds) == "table" and (cmds :: {any}) or {}) do
  if c.type == "CREATE_NODE" and type(c.payload) == "table" then return c end
 end
 return nil
end
return node_cmd`, "", "cannot return any, expected Map?")
	checkBothModes(t, `
type Map = {[string]: any}
local function node_cmd(cmds: any): Map?
 for _, c in ipairs(type(cmds) == "table" and (cmds :: {any}) or {}) do
  if type(c) == "table" and c.type == "CREATE_NODE" and type(c.payload) == "table" then return c :: Map end
 end
 return nil
end
return node_cmd`, "")
}

func TestValueSourceUnknownCloneSort(t *testing.T) {
	checkModes(t, `
type DynamicTable = {[string | number]: unknown}
local function clone(v: unknown): unknown
 if type(v) ~= "table" then return v end
 local source = v :: DynamicTable
 local out: DynamicTable = {}
 for k, item in pairs(source) do out[k] = clone(item) end
 return out
end
local function run(target: {kinds: {unknown}?})
 local kinds = clone(target.kinds)
 table.sort(kinds)
end
return run`, "", "argument 1:")
	checkBothModes(t, `
local function clone(v: unknown): unknown return v end
local function run(target: {kinds: {unknown}})
 local kinds = clone(target.kinds) :: {unknown}
 table.sort(kinds)
end
return run`, "")
}

func TestRetainedMapFieldContract(t *testing.T) {
	checkModes(t, `
type Map = {[string]: any}
local function resolve(raw_ref: any, provider_result: any): (Map?, any)
 local ref = type(raw_ref) == "table" and (raw_ref :: Map) or {}
 if ref.kind == "sink" then return {flow_ref = ref}, nil end
 if type(provider_result) ~= "table" or type((provider_result :: Map).flow_ref) ~= "table" then
  return nil, "no flow_ref"
 end
 return provider_result :: Map, nil
end
local function register(ref: Map) end
local function run(input: any, provider_result: any)
 local spec = {flow_ref = {}}
 local resolution, err = resolve(input, provider_result)
 if err or not resolution then return nil end
 spec.flow_ref = (resolution :: Map).flow_ref
 register(spec.flow_ref)
end
return run`, "", "argument 1: expected Map, got any")
}
