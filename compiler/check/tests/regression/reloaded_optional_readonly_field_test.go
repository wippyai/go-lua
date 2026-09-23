package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// viewportManifest declares a platform module type with an optional readonly
// field, the shape of tty.Viewport.
func viewportManifest() *io.Manifest {
	m := io.NewManifest("tty")

	viewport := typ.NewAlias("tty.Viewport", typ.NewRecord().
		ReadonlyField("width", typ.Integer).
		OptReadonlyField("cursor", typ.Integer).
		Build())
	m.DefineType("Viewport", viewport)

	m.SetExport(typ.NewInterface("tty", []typ.Method{
		{Name: "viewport", Type: typ.Func().Returns(viewport).Build()},
		{Name: "render", Type: typ.Func().Param("view", viewport).Returns(typ.Integer).Build()},
	}))
	return m
}

const reloadedHolderModule = `
local tty = require("tty")

type Holder = {view: tty.Viewport}

local M = {}
M.Holder = Holder

function M.draw(view: tty.Viewport): integer
    return view.width
end

return M
`

const reloadedHolderMain = `
local tty = require("tty")
local lib = require("lib")

local function show(view: tty.Viewport): integer
    return view.width
end

local function use(h: lib.Holder): integer
    local shown = show(h.view)
    local drawn = lib.draw(tty.viewport())
    local rebuilt: lib.Holder = {view = tty.viewport()}
    local rendered = tty.render(rebuilt.view)
    return shown + drawn + rendered
end

return use
`

// A manifest reloaded from its encoded form keeps an optional readonly field
// optional, so its types stay interchangeable with the live module type.
func TestReloadedOptionalReadonlyFieldMatchesLiveType(t *testing.T) {
	t.Run("reloaded type and live type are mutual subtypes", func(t *testing.T) {
		live := viewportManifest()
		reloaded := reloadManifest(t, live)

		liveType, ok := live.LookupType("Viewport")
		if !ok {
			t.Fatal("live tty manifest has no Viewport type")
		}
		reloadedType, ok := reloaded.LookupType("Viewport")
		if !ok {
			t.Fatal("reloaded tty manifest has no Viewport type")
		}

		if !subtype.IsSubtype(reloadedType, liveType) {
			t.Errorf("reloaded %s is not a subtype of live %s", typ.FormatShort(reloadedType), typ.FormatShort(liveType))
		}
		if !subtype.IsSubtype(liveType, reloadedType) {
			t.Errorf("live %s is not a subtype of reloaded %s", typ.FormatShort(liveType), typ.FormatShort(reloadedType))
		}
	})

	t.Run("library reloaded from its manifest checks against the live module", func(t *testing.T) {
		live := viewportManifest()
		lib := exportModule(t, "lib", reloadedHolderModule, testutil.WithManifest("tty", live))

		result := testutil.Check(reloadedHolderMain,
			testutil.WithStdlib(),
			testutil.WithManifest("tty", live),
			testutil.WithManifest("lib", lib),
		)
		if result.HasError() {
			t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
		}
	})
}
