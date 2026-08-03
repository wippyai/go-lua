package lua

import "testing"

func TestDebugLineHookCollectsCoverage(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
    local seen = {}
    local function record(event, line)
      assert(event == "line", "unexpected event: "..tostring(event))
      seen[line] = true
    end
    debug.sethook(record, "l")
    local a = 1
    local b = 2
    local c = a + b
    debug.sethook()
    assert(c == 3)
    local n = 0
    for _ in pairs(seen) do n = n + 1 end
    assert(n >= 3, "expected >=3 distinct lines, got "..n)
  `)
}

func TestDebugCountHookFires(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
    local calls = 0
    debug.sethook(function(event)
      assert(event == "count", "unexpected event: "..tostring(event))
      calls = calls + 1
    end, "", 1)
    local x = 0
    for i = 1, 10 do x = x + i end
    debug.sethook()
    assert(x == 55)
    assert(calls > 0, "count hook never fired")
  `)
}

func TestDebugGetHookRoundtrip(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
    local f = function() end
    debug.sethook(f, "l", 5)
    local h, mask, cnt = debug.gethook()
    assert(h == f, "hook fn mismatch")
    assert(mask == "l", "mask mismatch: "..tostring(mask))
    assert(cnt == 5, "count mismatch: "..tostring(cnt))
    debug.sethook()
    local h2, mask2 = debug.gethook()
    assert(h2 == nil, "hook not cleared")
    assert(mask2 == "", "mask not cleared: "..tostring(mask2))
  `)
}

func TestDebugLineHookAcrossNestedCalls(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
    local n = 0
    debug.sethook(function(event, line) n = n + 1 end, "l")
    local function inner()
      local z = 41
      return z + 1
    end
    local r = inner()
    local after = r + 0
    debug.sethook()
    assert(r == 42)
    assert(after == 42)
    assert(n >= 4, "expected line events across nested call, got "..n)
  `)
}

func TestDebugHookNoReentry(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
    local count = 0
    local function noise() return 1 + 1 end
    debug.sethook(function(event, line)
      count = count + 1
      noise()
    end, "l")
    local a = 1
    local b = 2
    local c = a + b
    debug.sethook()
    assert(c == 3)
    assert(count > 0, "hook never fired")
    assert(count < 1000, "hook reentered itself: "..count)
  `)
}

func TestDebugSethookRejectsCallReturnMask(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
    local ok1, err1 = pcall(debug.sethook, function() end, "c")
    assert(not ok1, "expected error for call hook")
    assert(string.find(err1, "not supported"), "unexpected error: "..tostring(err1))
    local ok2 = pcall(debug.sethook, function() end, "r")
    assert(not ok2, "expected error for return hook")
    assert(debug.gethook() == nil, "rejected sethook must not install a hook")
  `)
}

func TestDebugSethookClearWithNil(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfScriptFail(t, L, `
    debug.sethook(function() end, "l")
    assert(debug.gethook() ~= nil)
    debug.sethook(nil)
    assert(debug.gethook() == nil, "sethook(nil) did not clear")
  `)
}
