package regression

import "testing"

// Assertions supply the member domain in both modes. Strict mode checks uses
// of a dynamic result against concrete destinations, while dynamic calls are
// allowed; the operand's pre-assertion contract must not reappear at the read.
func TestGateAssertionCallControls(t *testing.T) {
	t.Run("automation asserted dynamic callee", func(t *testing.T) {
		checkBothModes(t, `
local function run(component: {set_meta: (() -> boolean)?})
 return (component :: any).set_meta()
end
return run`, "")
	})
	t.Run("automation unasserted optional callee", func(t *testing.T) {
		checkBothModes(t, `
local function run(component: {set_meta: (() -> boolean)?})
 return component.set_meta()
end
return run`, "cannot call optional value without nil check")
	})
	t.Run("inbox asserted dynamic arity", func(t *testing.T) {
		checkBothModes(t, `
local function run(inbox: {handle: (unknown) -> boolean})
 return (inbox :: any).handle({}, {})
end
return run`, "")
	})
	t.Run("inbox unasserted arity", func(t *testing.T) {
		checkBothModes(t, `
local function run(inbox: {handle: (unknown) -> boolean})
 return inbox.handle({}, {})
end
return run`, "too many arguments")
	})
	t.Run("sdk asserted dynamic argument", func(t *testing.T) {
		checkBothModes(t, `
local function run(funcs: {call: (string) -> any}, spec: any)
 return (funcs :: any).call(spec.id)
end
return run`, "")
	})
	t.Run("sdk unasserted argument", func(t *testing.T) {
		checkModes(t, `
local function run(funcs: {call: (string) -> any}, spec: any)
 return funcs.call(spec.id)
end
return run`, "", "expected string, got any")
	})
	t.Run("bedrock asserted dynamic call", func(t *testing.T) {
		checkBothModes(t, `
local function run(client: {post: ({headers?: {[string]: string}}) -> any}, headers: any)
 return (client :: any).post({headers = headers})
end
return run`, "")
	})
	t.Run("bedrock unasserted call", func(t *testing.T) {
		checkModes(t, `
local function run(client: {post: ({headers?: {[string]: string}}) -> any}, headers: any)
 return client.post({headers = headers})
end
return run`, "", "argument 1:")
	})
}

func TestGateAssertionResultControls(t *testing.T) {
	checkBothModes(t, `
local function run(row: any): string?
 return (row :: {hunt_id: string}).hunt_id
end
return run`, "")
	checkModes(t, `
local function run(row: any): string?
 return row.hunt_id
end
return run`, "", "cannot return any, expected string?")
	checkBothModes(t, `
local function run(row: any): string?
 return (row :: {hunt_id: number}).hunt_id
end
return run`, "cannot return number, expected string?")
}

func TestGateExplicitAnyMapValuesStayDynamic(t *testing.T) {
	checkModes(t, `
type Map = {[string]: any}
local function consume(value: string) end
local function run(row: {binding_id: string})
 consume((row :: Map).binding_id)
end
return run`, "", "expected string, got any")
	checkModes(t, `
type Map = {[string]: any}
local function run(row: {retry_after_ms: number}): number?
 return (row :: Map).retry_after_ms
end
return run`, "", "cannot return any, expected number?")
	checkModes(t, `
type Map = {[string]: any}
local function consume(value: Map) end
local function run(row: {config: Map})
 consume((row :: Map).config)
end
return run`, "", "expected Map, got any")
}
