local function run(flag: boolean)
	local M = {
		dep = {
			get = function()
				return nil
			end,
		},
	}

	if flag then
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
	end

	local res = M.dep.get()
	local answer: string = res.answer -- expect-error: cannot index type nil -- expect-hint: implicit unknown flows into declared string
	return answer
end

return run
