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
