package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
)

func gatePageType() testutil.Option {
	return testutil.WithTypes(map[string]typ.Type{"Page": typ.NewRecord().
		Field("id", typ.String).Field("announced", typ.Boolean).Field("secure", typ.Boolean).
		Build()})
}

func TestGateGuardedUndeclaredRecordField(t *testing.T) {
	checkBothModes(t, `
local function project(pages: {Page})
 for _, page in ipairs(pages) do
  if (not page.secure or page.id ~= "") and page.announced then
   local response: {placement: string} = {
    placement = type(page.placement) == "string" and page.placement or "default"
   }
  end
 end
end
return project`, "", gatePageType())
}

func TestGateUnguardedUndeclaredRecordField(t *testing.T) {
	checkModes(t, `
local function project(page: Page): string
 return page.placement
end
return project`, "", "cannot return unknown, expected string", gatePageType())
}

func TestGateGuardedFieldStillRejectsWrongType(t *testing.T) {
	checkBothModes(t, `
local function project(page: Page): string
 if type(page.placement) == "number" then return page.placement end
 return "default"
end
return project`, "cannot return number, expected string", gatePageType())
}
