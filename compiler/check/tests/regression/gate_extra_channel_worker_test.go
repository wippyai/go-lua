package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestExtraGateChannelWorkerKeepsWorkItemFields(t *testing.T) {
	checkBothModes(t, `
local channel = require("channel")
local function consume(item: {chunk: string, index: integer, worker_id: integer}) end
local function worker(worker_id, work_ch, result_ch)
 local work, ok = work_ch:receive()
 if not ok then return end
 local success, result = pcall(function()
  return {chunk = work.chunk, index = work.index, error = nil, worker_id = worker_id}
 end)
 if success then
  consume(result)
  result_ch:send(result)
 else
  print(string.format("worker %d", worker_id))
  result_ch:send({chunk = work.chunk, index = work.index, error = tostring(result), worker_id = worker_id})
 end
end
local function run(chunks: string[])
 local work_ch = channel.new(1)
 local result_ch = channel.new(1)
 for i = 1, #chunks do work_ch:send({index = i, chunk = chunks[i]}) end
 for worker_id = 1, 2 do
  local function spawn() worker(worker_id, work_ch, result_ch) end
  spawn()
 end
end
return run`, "", testutil.WithManifest("channel", testutil.ChannelManifest()))
}

func TestExtraGateProtectedCallbackRejectsIncompatibleCapturedFields(t *testing.T) {
	checkBothModes(t, `
local channel = require("channel")
local function consume(item: {chunk: string}) end
local function run(work_ch: channel.Channel<{chunk: number}>)
 local work, ok = work_ch:receive()
 if not ok then return end
 local success, result = pcall(function() return {chunk = work.chunk} end)
 if success then consume(result) end
end
return run`, "expected {chunk: string}", testutil.WithManifest("channel", testutil.ChannelManifest()))
}

func TestExtraGateMethodCallbackKeepsCaptureEnvironment(t *testing.T) {
	checkBothModes(t, `
local channel = require("channel")
local service = {}
function service:apply(callback: () -> string): string return callback() end
local function worker(work_ch)
 local work, ok = work_ch:receive()
 if not ok then return end
 local chunk: string = service:apply(function() return work.chunk end)
 return chunk
end
local work_ch = channel.new(1)
work_ch:send({chunk = "text"})
return worker(work_ch)`, "", testutil.WithManifest("channel", testutil.ChannelManifest()))
}

func TestExtraGateRecursiveCallbackEnvironmentConverges(t *testing.T) {
	const source = `
local function filter(schema)
 local function recurse(obj)
  if type(obj) ~= "table" then return obj end
  obj.multipleOf = nil
  obj.additionalProperties = nil
  for key, value in pairs(obj) do
   if type(value) == "table" then obj[key] = recurse(value) end
  end
  return obj
 end
 schema.examples = nil
 return recurse(schema)
end
return filter`
	for _, strict := range []bool{false, true} {
		result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
		if len(result.Diagnostics) != 0 {
			t.Fatalf("strict=%v: recursive environment did not converge: %v", strict, result.Diagnostics)
		}
	}
}
