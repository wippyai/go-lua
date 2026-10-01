package regression

import "testing"

func TestDeclaredRecordEffectsKeepSlots(t *testing.T) {
	t.Run("recursive_optional_widget", func(t *testing.T) {
		checkBothModes(t, `
type TextField = {value: string}
type Field = {kind: "text" | "number", text: TextField?, error: string?, baseline: string, validate: ((Field) -> string?)?}
type Form = {fields: {Field}}
local function text_set(field: TextField, value: string) field.value = value end
local function reset_field(field: Field)
 if field.kind == "text" and field.text then text_set(field.text, field.baseline) end
 field.error = nil
end
local function reset(form: Form)
 for _, field in ipairs(form.fields) do reset_field(field) end
end
return reset
`, "")
	})
	t.Run("recursive_optional_widget_incompatible_record", func(t *testing.T) {
		checkBothModes(t, `
type TextField = {value: string}
type Field = {kind: "text" | "number", text: TextField?, error: string?, baseline: string, validate: ((Field) -> string?)?}
type Form = {fields: {Field}}
local function text_set(field: TextField, value: string) field.value = value end
local function reset_field(field: Field)
 if field.kind == "text" and field.text then text_set(field.text, field.baseline) end
 field.error = nil
end
local function reset(form: Form)
 for _, field in ipairs(form.fields) do field.kind = "bogus"; reset_field(field) end
end
return reset
`, "argument 1: expected Field")
	})
	t.Run("callee_waiter_array", func(t *testing.T) {
		checkBothModes(t, `
type Waiter = {recipient: string}
type Instance = {pid: string, waiters: {Waiter}}
local function stop(item: Instance, waiter: Waiter)
 item.waiters[#item.waiters + 1] = waiter
end
local function control(find: (string) -> Instance?, pid: string, waiter: Waiter)
 local item = find(pid)
 if not item then return end
 stop(item, waiter)
end
return control
`, "")
	})
	t.Run("callee_waiter_array_incompatible_record", func(t *testing.T) {
		checkBothModes(t, `
type Waiter = {recipient: string}
type Instance = {pid: string, waiters: {Waiter}}
local function stop(item: Instance, waiter: Waiter)
 item.waiters[#item.waiters + 1] = waiter
end
local function control(find: (string) -> Instance?, pid: string, waiter: Waiter)
 local item = find(pid)
 if not item then return end
 stop({pid = 42, waiters = {}}, waiter)
end
return control
`, "argument 1: expected Instance")
	})
	t.Run("callee_leases_map", func(t *testing.T) {
		checkBothModes(t, `
type Host = {phase: string, leases: {[string]: string}}
type State = {hosts: {[string]: Host}}
local function hold(host: Host, lease: string, holder: string)
 host.leases[lease] = holder
end
local function acquire(state: State, id: string, lease: string, holder: string)
 local host = state.hosts[id]
 if host then
  if host.phase == "stopping" then return end
  hold(host, lease, holder)
 end
end
return acquire
`, "")
	})
	t.Run("callee_leases_map_incompatible_record", func(t *testing.T) {
		checkBothModes(t, `
type Host = {phase: string, leases: {[string]: string}}
type State = {hosts: {[string]: Host}}
local function hold(host: Host, lease: string, holder: string)
 host.leases[lease] = holder
end
local function acquire(state: State, id: string, lease: string, holder: string)
 local host = state.hosts[id]
 if host then
  if host.phase == "stopping" then return end
  hold({phase = 42, leases = {}}, lease, holder)
 end
end
return acquire
`, "argument 1: expected Host")
	})
	t.Run("future_array_mutation", func(t *testing.T) {
		checkBothModes(t, `
type Surface = {fixed: string, allowed_traits: {string}}
local function grant(surface: Surface): Surface?
 return {fixed = surface.fixed, allowed_traits = {"one", "two"}}
end
local function select(surface: Surface): string return surface.fixed end
local function test(surface: Surface)
 local granted = grant(surface)
 if not granted then return end
 select(granted)
 granted.allowed_traits[1] = "changed"
end
return test
`, "")
	})
	t.Run("future_array_mutation_incompatible_record", func(t *testing.T) {
		checkBothModes(t, `
type Surface = {fixed: string, allowed_traits: {string}}
local function grant(surface: Surface): Surface?
 return {fixed = surface.fixed, allowed_traits = {"one", "two"}}
end
local function select(surface: Surface): string return surface.fixed end
local function test(surface: Surface)
 local granted = grant(surface)
 if not granted then return end
 select({fixed = 42, allowed_traits = {"one"}})
 granted.allowed_traits[1] = "changed"
end
return test
`, "argument 1: expected Surface")
	})
}

func TestDeclaredRecordNestedEffectsKeepSlots(t *testing.T) {
	for _, tt := range []struct {
		name  string
		write string
		want  string
	}{
		{"valid_nested_slot", `item.child.kind = "b"`, ""},
		{"incompatible_nested_slot", `item.child.kind = "bogus"`, "argument 1: expected Item"},
		{"valid_optional_deletion", `item.note = nil`, ""},
		{"incompatible_optional_value", `item.note = 42`, "argument 1: expected Item"},
		{"valid_array_element", `item.names[1] = "changed"`, ""},
		{"incompatible_array_slot", `item.names = {42}`, "argument 1: expected Item"},
		{"valid_map_value", `item.labels["key"] = "changed"`, ""},
		{"incompatible_map_slot", `item.labels = {key = 42}`, "argument 1: expected Item"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkBothModes(t, `
			type Child = {kind: "a" | "b"}
			type Item = {fixed: string, child: Child, note: string?, names: {string}, labels: {[string]: string}}
			local function consume(item: Item) end
			local function use(selected: Item?)
				local item = selected
				if not item then return end
				`+tt.write+`
				consume(item)
			end
			return use
			`, tt.want)
		})
	}
}

func TestDeclaredRecursiveNestedFieldFacts(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  string
	}{
		{"admitted", `"b"`, ""},
		{"incompatible", `"bogus"`, "argument 1: expected Tree"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkBothModes(t, `
			type Leaf = {kind: "a" | "b"}
			type Tree = {fixed: string, child: Leaf?, clone: (Tree) -> Tree}
			local function consume(tree: Tree) end
			local function use(selected: Tree?)
				local tree = selected
				if not tree or not tree.child then return end
				tree.child.kind = `+tt.value+`
				consume(tree)
			end
			return use
			`, tt.want)
		})
	}
}
