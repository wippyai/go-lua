package checktest

import "testing"

func TestObjectLiteralEntryCastsCarryDeclaredContract(t *testing.T) {
	result := Check(`
type Page = {
    id: string?,
    config_overrides: any?,
}
type PageResponse = {
    id: string,
    configOverrides: {[string]: any}?,
}

local page: Page = { id = "p1", config_overrides = { theme = "dark" } }
local page_info: PageResponse = {
    id = page.id!,
    configOverrides = page.config_overrides :: {[string]: any}?,
}
local _ = page_info
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want object-literal entry assertions to carry their explicit evidence", result.Diagnostics)
	}
}

func TestImportedObjectLiteralEntryNonNilAssertionNarrowsOptionalField(t *testing.T) {
	pageRegistry := CheckAndExport(`
type PageInfo = {
    id: string,
    config_overrides: any?,
}

local M = {}

function M.find_all(): {PageInfo}
    return {
        {
            id = "p1",
            config_overrides = {
                theme = "dark",
            },
        },
    }
end

return M
`, "page_registry")
	if len(pageRegistry.Errors) != 0 {
		t.Fatalf("page_registry diagnostics = %#v, want clean module export", pageRegistry.Errors)
	}
	result := Check(`
local page_registry = require("page_registry")

type PageResponse = {
    id: string,
    configOverrides: {[string]: any}?,
}

local all_pages = page_registry.find_all()
local page = all_pages[1]!

local page_info: PageResponse = {
    id = page.id!,
    configOverrides = page.config_overrides :: {[string]: any}?,
}
local _ = page_info
`, WithModule("page_registry", pageRegistry))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported non-nil field assertion to narrow object-literal entry", result.Diagnostics)
	}
}

func TestObjectLiteralFunctionEntryCastCarriesSelfContract(t *testing.T) {
	result := Check(`
type Animal = { name: string, speak: (self: Animal) -> string }
type Dog = { name: string, speak: (self: Dog) -> string, fetch: (self: Dog) -> string }

local Animal = {}
function Animal.speak(self: Animal): string
    return self.name
end

local function fetch(self: Dog): string
    return self.name .. " fetches"
end

local dog: Dog = {
    name = "fido",
    speak = Animal.speak :: (self: Dog) -> string,
    fetch = fetch,
}
local _ = dog
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want function-valued object-literal entry cast to carry self contract", result.Diagnostics)
	}
}
