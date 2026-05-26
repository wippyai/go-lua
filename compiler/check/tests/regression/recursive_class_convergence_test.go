package regression

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// checkWithinTimeout runs the checker on source and fails if it does not return
// within limit. A non-terminating type-check is an inter-procedural fixpoint that
// does not converge; the value domain must prevent that by construction. This
// guard turns a non-convergence regression into a fast, clear failure instead of
// a multi-minute suite timeout.
func checkWithinTimeout(t *testing.T, limit time.Duration, source string, opts ...testutil.Option) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		testutil.Check(source, opts...)
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("type-check did not converge within %s: inter-procedural fixpoint non-convergence", limit)
	}
}

// TestSelfRecursiveClassConverges is a fast convergence guard for the recursive
// self-record pattern: a setmetatable class whose methods take self and whose run
// loop mutates loop-carried self fields. The class type is recursive through its
// metatable, so its summary is a recursive product. The inter-procedural fixpoint
// must converge; it cannot if the recursive product's Equal is inconsistent with
// its Hash (equal hash, distinct interned node), which prevents the fixpoint from
// ever detecting a fixed point.
func TestSelfRecursiveClassConverges(t *testing.T) {
	source := `
		local Bus = { stopping = false, pending_ops = 0 :: number }
		Bus.__index = Bus

		function Bus.new()
			local self = setmetatable({}, Bus)
			self.pending_ops = 1
			return self
		end

		function Bus:run()
			while not self.stopping do
				self.pending_ops = self.pending_ops - 1
				if self.pending_ops == 0 then
					self.stopping = true
				end
			end
		end

		local b = Bus.new()
		b:run()
	`
	checkWithinTimeout(t, 8*time.Second, source, testutil.WithStdlib())
}
