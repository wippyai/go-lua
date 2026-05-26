package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestZZHangProbe(t *testing.T) {
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
				if self.pending_ops == 0 then self.stopping = true end
			end
		end
		local b = Bus.new()
		b:run()
	`
	testutil.Check(source, testutil.WithStdlib())
}
