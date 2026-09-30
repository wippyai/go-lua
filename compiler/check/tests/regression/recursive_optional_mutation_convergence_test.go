package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRecursiveOptionalMutationConverges(t *testing.T) {
	const code = `
		type Message = {role: string}
		local function new(messages: {Message}?)
			local builder = {messages = messages or {}}
			builder.add = function(self: any, message: Message)
				table.insert(self.messages, message)
				return self
			end
			builder.clear = function(self: any)
				self.messages = {}
				return self
			end
			builder.clone = function(self: any)
				local copy = new()
				for _, message in ipairs(self.messages) do
					table.insert(copy.messages, message)
				end
				return copy
			end
			return builder
		end
	`
	for _, strict := range []bool{false, true} {
		name := "gradual"
		if strict {
			name = "strict"
		}
		t.Run(name, func(t *testing.T) {
			result := testutil.Check(code+`return new`, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			if len(result.Diagnostics) != 0 {
				t.Fatalf("recursive mutation fails to converge cleanly: %v", result.Diagnostics)
			}
		})
	}
	t.Run("incompatible_message", func(t *testing.T) {
		checkBothModes(t, code+`
			local builder = new()
			builder:add({role = 42})
			return new
		`, "expected Message")
	})
}
