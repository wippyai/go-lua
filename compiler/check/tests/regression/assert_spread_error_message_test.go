package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// assert(f()) passes f's error value as the message: a string or an Error.
func TestAssertSpreadAcceptsErrorMessage(t *testing.T) {
	link := io.NewManifest("link")
	link.SetExport(typ.NewRecord().
		Field("run", typ.Func().Returns(typ.NewOptional(typ.Any), typ.NewOptional(typ.String)).Build()).
		Field("open", typ.Func().Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()).
		Build())
	result := testutil.Check(`
local link = require("link")
local a = assert(link.run())
local b: string = assert(link.open())
local c = assert(link.open(), "explicit message")
`, testutil.WithStdlib(), testutil.WithManifest("link", link))
	if messages := testutil.ErrorMessages(result.Diagnostics); len(messages) != 0 {
		t.Fatalf("assert must accept a string or Error message from an expanded call, got %v", messages)
	}
}
