package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Assert must preserve an instantiated channel's identity and element type.
// Expanding Channel<T> while removing the assertion's falsy branch loses the
// type arguments used by channel.select to correlate result.channel/value.
func TestAssertReturnPreservesInstantiatedChannelSelectTypes(t *testing.T) {
	event := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("read")).Field("path", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("write")).Field("path", typ.String).Build(),
	)

	channelManifest := testutil.ChannelManifest()
	channelType, ok := channelManifest.LookupType("Channel")
	if !ok {
		t.Fatal("missing channel.Channel generic")
	}
	channelGeneric, ok := channelType.(*typ.Generic)
	if !ok {
		t.Fatalf("channel.Channel is not generic: %T", channelType)
	}

	maybeParam := typ.NewTypeParam("T", nil)
	maybeGeneric := typ.NewGeneric("Maybe", []*typ.TypeParam{maybeParam}, typ.NewOptional(maybeParam))

	ttyManifest := io.NewManifest("tty")
	ttyManifest.DefineType("Maybe", maybeGeneric)
	ttyManifest.SetExport(typ.NewInterface("tty", []typ.Method{
		{Name: "events", Type: typ.Func().Returns(typ.Instantiate(channelGeneric, event)).Build()},
		{Name: "optional_events", Type: typ.Func().Returns(typ.NewOptional(typ.Instantiate(channelGeneric, event))).Build()},
		{Name: "other", Type: typ.Func().Returns(typ.Instantiate(channelGeneric, typ.String)).Build()},
		{Name: "maybe", Type: typ.Func().Returns(typ.Instantiate(maybeGeneric, typ.String)).Build()},
	}))

	terminalManifest := io.NewManifest("terminal")
	terminalManifest.SetExport(typ.NewInterface("terminal", []typ.Method{
		{Name: "new", Type: typ.Func().Returns(typ.NewInterface("terminal.Terminal", []typ.Method{
			{Name: "send", Type: typ.Func().Param("self", typ.Self).Param("event", event).Returns(typ.Boolean).Build()},
		})).Build()},
	}))

	source := `
	local tty = require("tty")
	local channel = require("channel")
	local terminal = require("terminal").new()
	local input = assert(tty.events())
	local other = assert(tty.other())
	local optional_input = assert(tty.optional_events())
	local selected = channel.select({
	input:case_receive(),
	other:case_receive(),
})
if selected.channel == input then
	local event = selected.value
	terminal:send(event)
end

local selected_optional = channel.select({
	optional_input:case_receive(),
	other:case_receive(),
})
if selected_optional.channel == optional_input then
	local event = selected_optional.value
	terminal:send(event)
end

local maybe = assert(tty.maybe())
local _: string = maybe
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", channelManifest),
		testutil.WithManifest("tty", ttyManifest),
		testutil.WithManifest("terminal", terminalManifest),
	)
	if result.HasError() {
		t.Fatalf("assert should preserve channel select correlation and narrow Maybe<string>: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
