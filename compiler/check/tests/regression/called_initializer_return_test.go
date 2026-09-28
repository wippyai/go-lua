package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestCalledInitializerExportedReturnHasRequiredMethod(t *testing.T) {
	mod := testutil.CheckAndExport(`
local function make()
	local obj = {}
	local function write()
		obj.method = function(): number return 42 end
	end
	local function initialize()
		write()
	end
	initialize()
	return obj
end
return { new = make }
`, "m", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatalf("module errors: %v", testutil.ErrorMessages(mod.Errors))
	}
	export := unwrap.Alias(mod.Manifest.Export).(*typ.Record)
	fn := export.GetField("new").Type.(*typ.Function)
	result, ok := unwrap.Alias(fn.Returns[0]).(*typ.Record)
	if !ok {
		t.Fatalf("exported return = %v, want record", fn.Returns[0])
	}
	method := result.GetField("method")
	if method == nil || method.Optional || method.InferredPresence {
		t.Fatalf("exported return = %v, want required method", result)
	}
}

func TestCalledInitializerReturnPresence(t *testing.T) {
	cases := []struct {
		name, body string
		required   bool
	}{
		{"conditional", `if flag then initialize() end`, false},
		{"short_circuit", `local _ = flag and initialize()`, false},
		{"uncalled", `local unused = initialize`, false},
		{"both_branches", `if flag then initialize() else initialize() end`, true},
		{"removed_after_call", `initialize(); obj.method = nil`, false},
		{"removed_inside_initializer", `local function clear() obj.method = nil end; initialize(); clear()`, false},
		{"conditional_clear_after_initializer", `local function clear() obj.method = nil end; initialize(); local _ = flag and clear()`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := testutil.CheckAndExport(`
local function make(flag: boolean)
  local obj = {}
  local function write()
    obj.method = function(): number return 42 end
  end
  local function initialize() write() end
  `+tc.body+`
  return obj
end
return { new = make }
`, "m", testutil.WithStdlib())
			if mod.HasError() {
				t.Fatalf("module errors: %v", testutil.ErrorMessages(mod.Errors))
			}
			export := unwrap.Alias(mod.Manifest.Export).(*typ.Record)
			result := unwrap.Alias(export.GetField("new").Type.(*typ.Function).Returns[0]).(*typ.Record)
			method := result.GetField("method")
			got := method != nil && method.Type != typ.Nil && !method.Optional && !method.InferredPresence
			if got != tc.required {
				t.Fatalf("exported return = %v, required method = %v, want %v", result, got, tc.required)
			}
		})
	}
}

func TestComposedFieldWriteReturnPresence(t *testing.T) {
	cases := []struct {
		name, body string
		required   bool
	}{
		{"alias_observes_write", `local obj = {}; local alias = obj; local function write() obj.method = function() return 1 end end; write(); return alias`, true},
		{"write_through_alias", `local obj = {}; local alias = obj; local function write() alias.method = function() return 1 end end; write(); return obj`, true},
		{"parameter_alias_observes_write", `local obj = {}; local function write(target) local alias = target; alias.method = function() return 1 end end; write(obj); return obj`, true},
		{"reassigned_callee", `local obj = {}; local function write() obj.method = function() return 1 end end; local function other() end; write = other; write(); return obj`, false},
		{"captured_callee_reassigned", `local obj = {}; local function write() obj.method = function() return 1 end end; local function swap() write = function() end end; swap(); write(); return obj`, false},
		{"parameter_rebinding", `local obj = {}; local function write(target) target = {}; target.method = function() return 1 end end; write(obj); return obj`, false},
		{"nested_parameter_rebinding", `local obj = {}; local function write(target) local function swap() target = {} end; swap(); target.method = function() return 1 end end; write(obj); return obj`, false},
		{"capture_rebinding", `local obj = {}; local original = obj; local function write() obj = {}; obj.method = function() return 1 end end; write(); return original`, false},
		{"capture_cell_read_at_call", `local obj = {}; local old = obj; local function write() obj.method = function() return 1 end end; obj = {}; write(); return obj`, true},
		{"capture_rebind_leaves_old_object", `local obj = {}; local old = obj; local function write() obj.method = function() return 1 end end; obj = {}; write(); return old`, false},
		{"unknown_callback", `local obj = {}; local function write() obj.method = function() return 1 end end; write(); callback(obj); return obj`, false},
		{"write_after_unknown_callback", `local obj = {}; local function write() obj.method = function() return 1 end end; callback(obj); write(); return obj`, true},
		{"short_circuit_unknown_callback", `local obj = {}; local function write() obj.method = function() return 1 end end; write(); local _ = flag and callback(obj); return obj`, false},
		{"branch_unknown_callback", `local obj = {}; local function write() obj.method = function() return 1 end end; write(); if flag and callback(obj) then end; return obj`, false},
		{"escaping_writer_callback", `local obj = {}; local function write() obj.method = function() return 1 end end; write(); callback(write); return obj`, false},
		{"recursion", `local obj = {}; local function write(n) if n > 0 then write(n - 1) end end; write(1); return obj`, false},
		{"recursion_with_base_write", `local obj = {}; local function write(n) if n > 0 then write(n - 1) end; obj.method = function() return 1 end end; write(1); return obj`, true},
		{"distinct_factory_cells", `local function factory() local obj = {}; local function write() obj.method = function() return 1 end end; return obj, write end; local first, writeFirst = factory(); local second, writeSecond = factory(); writeSecond(); return first`, false},
		{"called_factory_cell", `local function factory() local obj = {}; local function write() obj.method = function() return 1 end end; return obj, write end; local first, writeFirst = factory(); local second, writeSecond = factory(); writeSecond(); return second`, true},
		{"escaping_factory_callback", `local function factory() local obj = {}; local function write() obj.method = function() return 1 end end; return obj, write end; local obj, write = factory(); write(); callback(write); return obj`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := testutil.CheckAndExport(`
local function make(callback: (any) -> (), flag: boolean)
  `+tc.body+`
end
return { new = make }
`, "m", testutil.WithStdlib())
			if mod.HasError() {
				t.Fatalf("module errors: %v", testutil.ErrorMessages(mod.Errors))
			}
			export := unwrap.Alias(mod.Manifest.Export).(*typ.Record)
			result := unwrap.Alias(export.GetField("new").Type.(*typ.Function).Returns[0]).(*typ.Record)
			method := result.GetField("method")
			got := method != nil && method.Type != typ.Nil && !method.Optional && !method.InferredPresence
			if got != tc.required {
				t.Fatalf("exported return = %v, required method = %v, want %v", result, got, tc.required)
			}
		})
	}
}
