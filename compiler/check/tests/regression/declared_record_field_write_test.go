package regression

import "testing"

// A field write on a value of a declared record type sets the field's current
// value; the table's slots keep their declared types, so the value remains of
// its declared type.
func TestFieldWriteKeepsDeclaredRecordType(t *testing.T) {
	t.Run("bee_launch_phases", func(t *testing.T) {
		checkBothModes(t, `
		type Phase = "boot" | "admit" | "running" | "save" | "exit" | "stopping"
		type Child = {phase: Phase, pending: string, deadline: integer?, activation: string?, ready: boolean}
		local function fail(child: Child): () child.phase = "stopping" end
		local function render(child: Child): boolean return child.ready end
		local function control(child: Child): boolean child.deadline = 10; return false end
		local function receive(selected: Child?): ()
			local child = selected
			if not child then return end
			if child.phase == "stopping" then return end
			if child.phase == "boot" then
				child.phase, child.pending, child.deadline = "admit", "request", 10
				fail(child)
			elseif child.phase == "admit" then
				child.phase, child.pending, child.deadline = "running", "", nil
				if child.ready then fail(child) end
				render(child)
			elseif child.phase == "running" then
				child.phase = "save"
				if not control(child) then fail(child) end
			elseif child.phase == "save" then
				child.phase = "exit"
				if not control(child) then fail(child) end
			end
		end
		return receive
	`, "")
	})
	t.Run("local_copy_of_declared_value", func(t *testing.T) {
		checkBothModes(t, `
		type Child = {phase: "boot" | "admit"}
		local function fail(child: Child): () end
		local function receive(selected: Child?): ()
			local child = selected
			if not child then return end
			child.phase = "admit"
			fail(child)
		end
		return receive
	`, "")
	})
	t.Run("field_reads_current_value", func(t *testing.T) {
		checkBothModes(t, `
		type Child = {phase: "boot" | "admit"}
		local function receive(selected: Child?): ()
			local child = selected
			if not child then return end
			child.phase = "admit"
			local phase: "admit" = child.phase
		end
		return receive
	`, "")
	})
	t.Run("filled_optional_field_meets_stricter_type", func(t *testing.T) {
		checkBothModes(t, `
		type Child = {name: string, deadline: integer?}
		type Ready = {name: string, deadline: integer}
		local function need(r: Ready) end
		local function annotated(c: Child)
			c.deadline = 10
			need(c)
		end
		local function copied(selected: Child?)
			local c = selected
			if not c then return end
			c.deadline = 10
			need(c)
		end
		return {annotated = annotated, copied = copied}
	`, "")
	})
	t.Run("value_outside_declared_slot", func(t *testing.T) {
		checkBothModes(t, `
		type Child = {phase: "boot" | "admit"}
		local function fail(child: Child): () end
		local function receive(selected: Child?): ()
			local child = selected
			if not child then return end
			child.phase = "bogus"
			fail(child)
		end
		return receive
	`, "argument 1: expected Child")
	})
	t.Run("written_value_is_not_other_record", func(t *testing.T) {
		checkBothModes(t, `
		type Child = {phase: "boot" | "admit"}
		local function other(value: {phase: integer}): () end
		local function receive(selected: Child?): ()
			local child = selected
			if not child then return end
			child.phase = "admit"
			other(child)
		end
		return receive
	`, "expected {phase: integer}, got Child")
	})
	t.Run("field_reads_written_value_only", func(t *testing.T) {
		checkBothModes(t, `
		type Child = {phase: "boot" | "admit"}
		local function receive(selected: Child?): ()
			local child = selected
			if not child then return end
			child.phase = "admit"
			local phase: "boot" = child.phase
		end
		return receive
	`, "cannot assign \"admit\" to \"boot\"")
	})
}
