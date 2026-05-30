package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// zzPresenceSrc mirrors testdata/fixtures/realworld/index-presence-laws/main.lua.
const zzPresenceSrc = `
type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local messages: {[string]: Message} = {}

if not messages["root"] then
    messages["root"] = {
        _topic = "installed",
        topic = function(self: Message): string
            return self._topic
        end,
    }
end

local installed: string = messages["root"]:topic()

local cached = messages["root"]
if cached then
    local cached_topic: string = cached:topic()
end

assert(messages["root"])
local asserted: string = messages["root"]:topic()
`

func TestZZPresence_IndexPresenceLaws(t *testing.T) {
	r := testutil.Check(zzPresenceSrc,
		testutil.WithStdlib(),
		testutil.WithCheckOption(check.WithCanonicalFlow()))
	if !r.HasError() {
		t.Logf("[presence] NO ERROR")
		return
	}
	for _, e := range r.Errors {
		t.Logf("[presence] err: %s @ %d:%d", e.Message, e.Position.Line, e.Position.Column)
	}
}

// zzPresenceUnprovenSrc reads an UNPROVEN key after proving "root" present, so the
// soundness gate must still report the unproven read optional/erroring.
const zzPresenceUnprovenSrc = `
type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local messages: {[string]: Message} = {}

assert(messages["root"])
local other: string = messages["other"]:topic()
`

func TestZZPresence_UnprovenKeyStillErrors(t *testing.T) {
	r := testutil.Check(zzPresenceUnprovenSrc,
		testutil.WithStdlib(),
		testutil.WithCheckOption(check.WithCanonicalFlow()))
	if !r.HasError() {
		t.Logf("[unproven] NO ERROR (UNSOUND if this prints)")
		return
	}
	for _, e := range r.Errors {
		t.Logf("[unproven] err: %s @ %d:%d", e.Message, e.Position.Line, e.Position.Column)
	}
}
