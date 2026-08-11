package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestSourceDirectFunctionProofRequiresInstallationOnEveryPath(t *testing.T) {
	directAt := func(t *testing.T, p *program.Program, index int) keyspace.Term {
		t.Helper()
		flow := p.Flow()
		call, ok := flow.Authored().Calls().At(index)
		if !ok {
			t.Fatalf("missing Call %d", index)
		}
		function, _ := flow.DirectFunctions().Call(call)
		return function
	}

	t.Run("assignment after the Call cannot prove it", func(t *testing.T) {
		p := parseBindLower(t, `
local f
f()
f = function() end
f()
`)
		function, _ := p.Flow().Authored().Functions().At(0)
		if got := directAt(t, p, 0); got != 0 {
			t.Fatalf("pre-install Call direct = %v, want none", got)
		}
		if got := directAt(t, p, 1); got != function {
			t.Fatalf("post-install Call direct = %v, want %v", got, function)
		}
	})

	t.Run("unconditional do installation is direct after its Body", func(t *testing.T) {
		p := parseBindLower(t, `
local f
do f = function() end end
f()
`)
		function, _ := p.Flow().Authored().Functions().At(0)
		if got := directAt(t, p, 0); got != function {
			t.Fatalf("do-installed Call direct = %v, want %v", got, function)
		}
	})

	t.Run("captured Cell installation is direct in the same activation", func(t *testing.T) {
		p := parseBindLower(t, `
local f
local g = function()
	f = function() end
	f()
end
g()
`)
		installed, _ := p.Flow().Authored().Functions().At(1)
		if got := directAt(t, p, 0); got != installed {
			t.Fatalf("same-activation captured Call direct = %v, want %v", got, installed)
		}
	})

	for _, test := range []struct {
		name, source string
	}{
		{
			name: "multi-assign RHS runs before its writes",
			source: `local f, x
f, x = function() end, f()`,
		},
		{
			name: "extra assignment RHS Call runs before its write",
			source: `local f
f = function() end, f()`,
		},
		{
			name: "Call argument in assignment RHS runs before its write",
			source: `local f
f = function() end, invoke(f())`,
		},
		{
			name: "table field in assignment RHS runs before its write",
			source: `local f, value
f, value = function() end, { callback = f() }`,
		},
		{
			name:   "local initializer cannot see its own installation",
			source: `local f, value = function() end, f()`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, test.source)
			for index := 0; index < p.Flow().Authored().Calls().Count(); index++ {
				if got := directAt(t, p, index); got != 0 {
					t.Fatalf("same-RHS Call %d direct = %v, want none", index, got)
				}
			}
		})
	}

	t.Run("cross-activation installation is not direct", func(t *testing.T) {
		p := parseBindLower(t, `
local f
local install = function() f = function() end end
install()
f()
`)
		if got := directAt(t, p, 1); got != 0 {
			t.Fatalf("cross-activation Call direct = %v, want none", got)
		}
	})

	for _, test := range []struct {
		name, source string
	}{
		{
			name: "branch may bypass installation",
			source: `local f
if condition then f = function() end end
f()`,
		},
		{
			name: "loop may execute zero times",
			source: `local f
while condition do f = function() end end
f()`,
		},
		{
			name: "goto bypasses installation",
			source: `local f
goto after
f = function() end
::after::
f()`,
		},
		{
			name: "capture has another activation root",
			source: `local f
local g = function() f() end
f = function() end
g()`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, test.source)
			if got := directAt(t, p, 0); got != 0 {
				t.Fatalf("unproven Call direct = %v, want none", got)
			}
		})
	}

	t.Run("recursive local declaration remains exact", func(t *testing.T) {
		p := parseBindLower(t, `local function f() f() end`)
		function, _ := p.Flow().Authored().Functions().At(0)
		if got := directAt(t, p, 0); got != function {
			t.Fatalf("recursive Call direct = %v, want %v", got, function)
		}
	})

	t.Run("sole assignment closure retains recursive exactness", func(t *testing.T) {
		p := parseBindLower(t, `
local f
f = function() f() end
f()
`)
		function, _ := p.Flow().Authored().Functions().At(0)
		if got := directAt(t, p, 0); got != function {
			t.Fatalf("assignment-recursive Call direct = %v, want %v", got, function)
		}
		if got := directAt(t, p, 1); got != function {
			t.Fatalf("post-assignment Call direct = %v, want %v", got, function)
		}
	})
}
