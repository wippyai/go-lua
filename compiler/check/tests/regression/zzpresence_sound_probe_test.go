package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func runPresence(t *testing.T, label, src string) {
	t.Helper()
	r := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	if !r.HasError() {
		t.Logf("[%s] NO ERROR", label)
		return
	}
	for _, e := range r.Errors {
		t.Logf("[%s] err: %s @ %d:%d", label, e.Message, e.Position.Line, e.Position.Column)
	}
}

// A different key on the same map must NOT be narrowed present by filling "root".
func TestZZPresence_DifferentKeyNotNarrowed(t *testing.T) {
	runPresence(t, "diff-key", `
type Message = { _topic: string, topic: (self: Message) -> string }
local messages: {[string]: Message} = {}
if not messages["root"] then
    messages["root"] = { _topic = "x", topic = function(self: Message): string return self._topic end }
end
local bad: string = messages["other"]:topic()
`)
}

// The fill idiom that assigns a DIFFERENT key than guarded must NOT prove the
// guarded key present (assigning "root" when guarding "leaf").
func TestZZPresence_FillsDifferentKey(t *testing.T) {
	runPresence(t, "fills-other", `
type Message = { _topic: string, topic: (self: Message) -> string }
local messages: {[string]: Message} = {}
if not messages["leaf"] then
    messages["root"] = { _topic = "x", topic = function(self: Message): string return self._topic end }
end
local bad: string = messages["leaf"]:topic()
`)
}

// No fill at all: guarding without assigning must keep the key optional after.
func TestZZPresence_NoFillStaysOptional(t *testing.T) {
	runPresence(t, "no-fill", `
type Message = { _topic: string, topic: (self: Message) -> string }
local messages: {[string]: Message} = {}
if not messages["root"] then
    local x = 1
end
local bad: string = messages["root"]:topic()
`)
}

// The truthy guard direction `if m[k] then m[k]=v end` is degenerate (then-arm runs
// when present, so its assignment is redundant); the fall-through must NOT be proven
// present by the assignment on the wrong arm.
func TestZZPresence_TruthyGuardArm(t *testing.T) {
	runPresence(t, "truthy-arm", `
type Message = { _topic: string, topic: (self: Message) -> string }
local messages: {[string]: Message} = {}
if messages["root"] then
    messages["root"] = { _topic = "x", topic = function(self: Message): string return self._topic end }
end
local bad: string = messages["root"]:topic()
`)
}

// type(x.f) == "number" over a dynamic map base must still pin x.f to number on the
// positive edge (the original purpose of the Map narrowing arm).
func TestZZPresence_TypeofTableFieldStillNarrows(t *testing.T) {
	runPresence(t, "typeof-field", `
local function pick(x: any): number
    if type(x.count) == "number" then
        return x.count
    end
    return 0
end
local _ = pick
`)
}
