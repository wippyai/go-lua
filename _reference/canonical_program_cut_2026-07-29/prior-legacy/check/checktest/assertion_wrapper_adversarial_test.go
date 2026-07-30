package checktest

import "testing"

func TestAssertionWrapperNormalReturnPresenceDoesNotTaintCallerType(t *testing.T) {
	result := Check(`
local assert = {
    not_nil = function(val: any, msg: string?)
        if val == nil then error(msg or "assertion failed") end
    end
}

local function process(x: string?): ()
    assert.not_nil(x)
    local s: string = x
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after wrapper proves x non-nil without turning it into any", result.Diagnostics)
	}
}

func TestGuardedUnionFieldFeedsDirectCallJudgment(t *testing.T) {
	result := Check(`
type TemplatePage = {
    kind: "template",
    id: string,
    data_func: string?,
    template_set: string,
}

type ComponentPage = {
    kind: "component",
    id: string,
    url: string,
}

type Page = TemplatePage | ComponentPage

local function takes_string(name: string): string
    return name
end

local function get_page_data(page: Page?): ()
    if not page or not page.data_func or page.data_func == "" then
        return
    end

    local name: string = page.data_func
    takes_string(page.data_func)
end
	`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guards prove page.data_func is string", result.Diagnostics)
	}
}
