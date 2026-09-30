package regression

import "testing"

func TestDeclaredRecordEffectsKeepSlots(t *testing.T) {
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
