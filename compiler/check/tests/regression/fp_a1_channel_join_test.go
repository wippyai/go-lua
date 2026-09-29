package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// The send and receive closures observe successive snapshots of the same
// captured channel. Its unresolved element must yield to the sent integer.
func TestCapturedChannelElementResolvesAcrossTwoClosures(t *testing.T) {
	source := `
local channel = require("channel")
local jobs = channel.new(1)
local function produce()
    jobs:send(10)
end
local function consume(): integer
    local value, ok = jobs:receive()
    if ok then return value + 1 end
    return 0
end
produce()
return consume()
`
	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithManifest("channel", testutil.ChannelManifest()))
	if result.HasError() {
		t.Fatalf("captured channel should keep its integer element: %v", testutil.ErrorMessages(result.Errors))
	}
}
