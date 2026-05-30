package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZFieldPathProbe drives the field-path / error-return narrowing fixtures
// through the canonical flow so ZNARROW traces the per-edge narrowing. Debug probe.
func TestZZFieldPathProbe(t *testing.T) {
	cases := map[string]string{
		"guard": `
type Page = {
    data_func: string?,
}
local function takes_string(name: string)
    return name
end
local function get_page_data(page: Page?)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end
    local name: string = page.data_func
    takes_string(page.data_func)
    return {}, nil
end
return get_page_data
`,
		"error-return-pattern": `
local function getData(): (string?, string?)
    return "data", nil
end
local data, err = getData()
if err then
    error(err)
end
local s: string = data
`,
		"intersection": `
type PageInfo = {
    id: string,
    name: string,
    secure: boolean,
}
type PageDetail = PageInfo & {
    data_func: string?,
    template_set: string?,
}
local function takes_string(name: string)
    return name
end
local function get_page_data(page: PageDetail?)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end
    local name: string = page.data_func
    takes_string(page.data_func)
    return {}, nil
end
return get_page_data
`,
		"inferred": `
type Page = {
    data_func: string?,
}
local function load_page(): (Page?, string?)
    return { data_func = "demo" }, nil
end
local function takes_string(name: string)
    return name
end
local function get_page_data(page)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end
    local name: string = page.data_func
    takes_string(page.data_func)
    return {}, nil
end
local page, err = load_page()
if err then
    return nil, err
end
return get_page_data(page)
`,
		"nested-arith": `
type PayloadCarrier = {
    data: fun(self: PayloadCarrier): any,
}
local function bump(carrier: PayloadCarrier?)
    local data = carrier and carrier:data() or nil
    if type(data) ~= "table" or type(data.amount) ~= "number" then
        return nil
    end
    local next_amount = data.amount + 1
    local exact: number = data.amount
    return next_amount, exact
end
return bump
`,
		"inferred-conditional-error": `
local function maybeError(cond: boolean)
    if cond then
        error("condition was true")
    end
end
function process(x: string?)
    maybeError(x == nil)
    local s: string = x
end
`,
		"inferred-error-all-branches": `
local function fail(msg: string)
    error(msg)
end
function process(x: string?): string
    if x == nil then
        fail("x is nil")
    end
    return x
end
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			for _, m := range testutil.ErrorMessages(res.Diagnostics) {
				t.Logf("DIAG: %s", m)
			}
		})
	}
}
