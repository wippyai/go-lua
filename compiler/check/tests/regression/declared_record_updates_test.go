package regression

import "testing"

// A nested field write below a dynamic index updates the existing entry; it
// does not insert a partial entry into the declared array.
func TestNestedIndexedFieldWriteKeepsDeclaredElement(t *testing.T) {
	const prelude = `
		type Rect = {x: integer, y: integer}
		type Window = {normal_bounds: Rect, bounds: Rect, mode: "collapsed" | "floating"}
		local function copy_windows(values: {Window}): {Window}
			local result: {Window} = {}
			for i = 1, #values do
				result[i] = {normal_bounds = {x = values[i].normal_bounds.x, y = values[i].normal_bounds.y}, bounds = values[i].bounds, mode = values[i].mode}
			end
			return result
		end
		local function commit(windows: {Window}): {Window} return windows end
	`
	t.Run("bee_desktop_place", func(t *testing.T) {
		checkBothModes(t, prelude+`
		type Scene = {windows: {Window}}
		local function place(scene: Scene, index: integer, placed: Rect): {Window}
			local current = scene.windows[index]
			if current.mode == "collapsed" then
				local windows = copy_windows(scene.windows)
				windows[index].bounds = placed
				windows[index].normal_bounds.x = placed.x
				windows[index].normal_bounds.y = placed.y
				return commit(windows)
			end
			return scene.windows
		end
		return place
	`, "")
	})
	t.Run("two_level_update", func(t *testing.T) {
		checkBothModes(t, prelude+`
		local function place(ws: {Window}, index: integer, placed: Rect): {Window}
			local windows = copy_windows(ws)
			windows[index].normal_bounds.x = placed.x
			return commit(windows)
		end
		return place
	`, "")
	})
	t.Run("inferred_entries_gain_nested_field", func(t *testing.T) {
		checkBothModes(t, `
		local function place(index: integer)
			local rows = {}
			rows[index] = {pos = {x = 1}}
			rows[index].pos.x = 2
			local x: integer = rows[index].pos.x
			return x
		end
		return place
	`, "")
	})
	t.Run("updated_array_is_not_other_array", func(t *testing.T) {
		checkBothModes(t, prelude+`
		local function rects(values: {Rect}): {Rect} return values end
		local function place(ws: {Window}, index: integer, placed: Rect)
			local windows = copy_windows(ws)
			windows[index].normal_bounds.x = placed.x
			return rects(windows)
		end
		return place
	`, "expected Rect[], got Window[]")
	})
	t.Run("nested_write_widens_inferred_entry", func(t *testing.T) {
		checkBothModes(t, `
		local function place(index: integer)
			local rows = {}
			rows[index] = {pos = {x = 1}}
			rows[index].pos.x = "left"
			local x: integer = rows[index].pos.x
			return x
		end
		return place
	`, "to integer")
	})
}

// A table literal may omit a field whose declared type admits nil: an absent
// key reads as nil. The literal then takes the expected record type.
func TestTableLiteralOmitsNilAdmittingField(t *testing.T) {
	t.Run("bee_owner_stop_call", func(t *testing.T) {
		checkBothModes(t, `
		type OwnerRef = {node_id: string, service_id: string, resource_ref: string?}
		type Call = {protocol_revision: string, request_id: string, idempotency_key: string, deadline: string?, owner_ref: OwnerRef, target: {operation_ref: string?, interface_ref: string?}, input: {[string]: unknown}}
		local types = {REVISION = "bee.hive@1"}
		local owner_stop = {SERVICE = "bee.hive.owner", STOP = "bee.hive.owner:stop"}
		function owner_stop.decode(call: Call): boolean return true end
		local test = {}
		function test.it(name: string, run: () -> ()) run() end
		local function call(input: {[string]: unknown}, operation: string?, owner: OwnerRef?): Call
			return {protocol_revision = types.REVISION, request_id = "request-1", idempotency_key = "key-1", owner_ref = owner or {node_id = "owner-node", service_id = owner_stop.SERVICE},
				target = {operation_ref = operation or owner_stop.STOP}, input = input}
		end
		local function run(): ()
			test.it("decode", function()
				owner_stop.decode(call({alone = true}))
				owner_stop.decode(call({alone = false, force = true}))
			end)
		end
		return run
	`, "")
	})
	t.Run("literal_union_field", func(t *testing.T) {
		checkBothModes(t, `
		type Call = {phase: "a" | "b", deadline: string?}
		local function make(): Call
			return {phase = "a"}
		end
		return make
	`, "")
	})
	t.Run("required_field_missing", func(t *testing.T) {
		checkBothModes(t, `
		type Call = {phase: "a" | "b", deadline: string}
		local function make(): Call
			return {phase = "a"}
		end
		return make
	`, "cannot return")
	})
	t.Run("nil_admitting_field_wrong_value", func(t *testing.T) {
		checkBothModes(t, `
		type Call = {phase: "a" | "b", deadline: string?}
		local function make(): Call
			return {phase = "a", deadline = 5}
		end
		return make
	`, "cannot return")
	})
}

// A call-site hint refines an annotated map parameter within the annotation:
// keys the hint does not list keep the annotation's key and value types.
func TestRefinedMapParameterKeepsAnnotationMap(t *testing.T) {
	t.Run("returned_in_record", func(t *testing.T) {
		checkBothModes(t, `
		type Call = {input: {[string]: unknown}}
		local function call(input: {[string]: unknown}): Call
			local c = {input = input}
			return c
		end
		local function run(): ()
			call({alone = true})
			call({alone = false, force = true})
		end
		return run
	`, "")
	})
	t.Run("passed_as_annotation", func(t *testing.T) {
		checkBothModes(t, `
		local function take(m: {[string]: unknown}): () end
		local function call(input: {[string]: unknown}): ()
			take(input)
		end
		local function run(): ()
			call({alone = true})
		end
		return run
	`, "")
	})
	t.Run("hint_field_is_readable", func(t *testing.T) {
		checkBothModes(t, `
		local function call(input: {[string]: unknown}): boolean
			local alone = input.alone
			return alone == true
		end
		local function run(): ()
			call({alone = true})
		end
		return run
	`, "")
	})
	// Gradual mode reports unknown flowing into string as a hint.
	t.Run("unknown_values_are_not_strings", func(t *testing.T) {
		checkModes(t, `
		local function take(m: {[string]: string}): () end
		local function call(input: {[string]: unknown}): ()
			take(input)
		end
		local function run(): ()
			call({alone = "x"})
		end
		return run
	`, "", "argument 1")
	})
	t.Run("record_value_is_not_string_map", func(t *testing.T) {
		checkBothModes(t, `
		type Call = {input: {[string]: string}}
		local function call(input: {[string]: unknown}): Call
			local c = {input = input}
			return c
		end
		local function run(): ()
			call({alone = "x"})
		end
		return run
	`, "cannot return")
	})
}
