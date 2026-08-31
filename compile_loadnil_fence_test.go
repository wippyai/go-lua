// SPDX-License-Identifier: MPL-2.0

package lua

import (
	"testing"
)

// The LOADNIL merge peephole (codeStore.AddLoadNil) must not extend a LOADNIL
// that sits before a jump target: an `or nil` arm ends in a skippable LOADNIL,
// and folding the next statement's nil-register init into it leaves that
// register uninitialized on the short-circuit path. The VM then dereferences
// an empty stack slot. Mirrors PUC Lua's `fs->lasttarget` fence in luaK_nil.
func TestLoadNilMergeStopsAtJumpTarget(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			// The or takes the truthy lhs: the skipped nil arm must not own
			// the comparison's nil register.
			name: "or nil over a table hit inside ipairs",
			code: `
				local t = { k = { skip = true } }
				local out = 0
				for _, key in ipairs({ "k" }) do
					local v = t[key] or nil
					if v ~= nil then out = out + 1 end
				end
				return out
			`,
			expected: "1",
		},
		{
			name: "or nil over a table miss inside ipairs",
			code: `
				local t = { k = { skip = true } }
				local out = 0
				for _, key in ipairs({ "absent" }) do
					local v = t[key] or nil
					if v == nil then out = out + 1 end
				end
				return out
			`,
			expected: "1",
		},
		{
			name: "scalar or nil inside ipairs",
			code: `
				local out = 0
				for _, key in ipairs({ "k" }) do
					local x = key or nil
					if x ~= nil then out = out + 1 end
				end
				return out
			`,
			expected: "1",
		},
		{
			name: "or nil inside pairs",
			code: `
				local t = { k = { skip = true } }
				local out = 0
				for key in pairs({ k = 1 }) do
					local v = t[key] or nil
					if v ~= nil then out = out + 1 end
				end
				return out
			`,
			expected: "1",
		},
		{
			name: "guarded chain with mixed hit and miss keys",
			code: `
				local t = { k = { skip = true } }
				local out = 0
				for _, key in ipairs({ "a", "k" }) do
					local v = key ~= nil and t[key] or nil
					if v ~= nil and v.skip == true then out = out + 1 end
				end
				return out
			`,
			expected: "1",
		},
		{
			// Adjacent nil locals with no label in between keep merging.
			name: "plain adjacent nil locals still fold",
			code: `
				local a, b
				local c = nil
				if a == nil and b == nil and c == nil then return 1 end
				return 0
			`,
			expected: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			got := L.Get(-1)
			if got.String() != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got.String())
			}
		})
	}
}
