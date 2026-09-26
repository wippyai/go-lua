// SPDX-License-Identifier: MPL-2.0

package lua

import "testing"

func TestMethodCallOnConcatenationWithCallOperand(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "left call one result in expression",
			code: `local function f() return "a" end
				local x = "b"
				return (f() .. x):upper()`,
			want: "AB",
		},
		{
			name: "left call multiple results in expression",
			code: `local function f() return "a", "ignored" end
				local x = "b"
				return (f() .. x):upper()`,
			want: "AB",
		},
		{
			name: "right call one result in expression",
			code: `local function f() return "b" end
				local a = "a"
				return (a .. f()):sub(1)`,
			want: "ab",
		},
		{
			name: "right call multiple results in expression",
			code: `local function f() return "b", "ignored" end
				local a = "a"
				return (a .. f()):sub(1)`,
			want: "ab",
		},
		{
			name: "generic for one result",
			code: `local function f(v) return v end
				local NL = "\n"
				local out = ""
				for x in (f("a") .. NL):gmatch("[^\n]+") do out = out .. x end
				return out`,
			want: "a",
		},
		{
			name: "generic for multiple results",
			code: `local function f(v) return v, "ignored" end
				local NL = "\n"
				local out = ""
				for x in (f("a") .. NL):gmatch("[^\n]+") do out = out .. x end
				return out`,
			want: "a",
		},
		{
			name: "right call one result in generic for",
			code: `local function f() return "b" end
				local a = "a"
				local out = ""
				for x in (a .. f()):gmatch(".") do out = out .. x end
				return out`,
			want: "ab",
		},
		{
			name: "right call multiple results in generic for",
			code: `local function f() return "b", "ignored" end
				local a = "a"
				local out = ""
				for x in (a .. f()):gmatch(".") do out = out .. x end
				return out`,
			want: "ab",
		},
		{
			name: "direct gmatch control",
			code: `local function f(v) return v, "ignored" end
				local out = ""
				for x in string.gmatch(f("a") .. "\n", "[^\n]+") do out = out .. x end
				return out`,
			want: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("DoString: %v", err)
			}
			if got := L.Get(-1).String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
