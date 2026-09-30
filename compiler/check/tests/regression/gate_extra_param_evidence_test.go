package regression

import (
	"strings"
	"testing"
)

const extraGateParameterCapture = `
local function consume(item: {worker_id: integer}) end
local function worker(worker_id)
 local success, result = pcall(function() return {worker_id = worker_id} end)
 if success then consume(result) end
 print(string.format("worker %d", worker_id))
end
for worker_id = 1, 2 do worker(worker_id) end`

func TestExtraGateCapturedParameterKeepsCallerEvidence(t *testing.T) {
	checkBothModes(t, extraGateParameterCapture, "")
}

func TestExtraGateCapturedParameterRejectsWrongFieldType(t *testing.T) {
	checkBothModes(t, strings.Replace(extraGateParameterCapture, "worker_id: integer", "worker_id: string", 1), "expected {worker_id: string}")
}

func TestExtraGateExplicitAnyParameterStillNeedsNarrowing(t *testing.T) {
	checkModes(t, strings.Replace(extraGateParameterCapture, "worker(worker_id)", "worker(worker_id: any)", 1), "", "expected {worker_id: integer}")
}

func TestCapturedReassignedParameterUsesCapturePointValue(t *testing.T) {
	const source = `
local function consume(item: {worker_id: string}) end
local function worker(worker_id)
 worker_id = "worker"
 local success, result = pcall(function() return {worker_id = worker_id} end)
 if success then consume(result) end
 print(string.format("worker %s", worker_id))
end
worker(1)`
	t.Run("reassigned_string", func(t *testing.T) { checkBothModes(t, source, "") })
	t.Run("wrong_target", func(t *testing.T) {
		checkBothModes(t, strings.Replace(source, "worker_id: string", "worker_id: integer", 1), "expected {worker_id: integer}")
	})
	t.Run("reassigned_integer", func(t *testing.T) {
		code := strings.Replace(source, `worker_id = "worker"`, "worker_id = 2", 1)
		code = strings.Replace(code, "worker_id: string", "worker_id: integer", 1)
		checkBothModes(t, code, "")
	})
	t.Run("no_reassignment", func(t *testing.T) {
		checkBothModes(t, strings.Replace(source, ` worker_id = "worker"`, "", 1), "expected {worker_id: string}")
	})
}

func TestSuppliedCaptureIgnoresNonDominatingFieldWrites(t *testing.T) {
	checkBothModes(t, `
local function run(flag: boolean)
 local M = {dep = {get = function() return nil end}}
 function M.run() return M.dep.get() end
 if flag then M.dep = {get = function() return {answer = "ok"} end} end
 local res = M.run()
 return res.answer
end
return run`, "cannot index type nil")
}

func TestSuppliedCaptureIncludesPrecedingFieldWrites(t *testing.T) {
	checkBothModes(t, `
local function run()
 local M = {dep = {get = function() return nil end}}
 M.dep = {get = function() return {answer = "ok"} end}
 function M.run() return M.dep.get() end
 local res = M.run()
 local answer: string = res.answer
 return answer
end
return run`, "")
}
