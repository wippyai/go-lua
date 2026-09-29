package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAnnotatedMapCaptureReturnAssignsToOptionalString(t *testing.T) {
	result := testutil.Check(`
local MIME_TYPES: {[string]: string} = { txt = "text/plain" }

local function get_file_extension(filename: string)
    return filename:match("%.([^%.]+)$") or ""
end

local function get_mime_type_from_extension(filename: string)
    local ext = get_file_extension(filename):lower()
    if ext and MIME_TYPES[ext] then
        return MIME_TYPES[ext]
    end
    return "application/octet-stream"
end

local M = {}
function M.upload_file(filename: string, mime_type: string?)
    if not mime_type or mime_type == "" or mime_type == "application/octet-stream" then
        mime_type = get_mime_type_from_extension(filename)
    end
    return mime_type
end
return M
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("annotated string map lookup must return string: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestAnnotatedMapCaptureDirectLookupAssignsToOptionalString(t *testing.T) {
	result := testutil.Check(`
local MIME_TYPES: {[string]: string} = { txt = "text/plain" }
local function lookup(ext: string)
    return MIME_TYPES[ext]
end
local mime_type: string? = lookup("txt")
return mime_type
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("direct annotated map lookup must return string?: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestAnnotatedMapCaptureGuardedLookupAssignsToOptionalString(t *testing.T) {
	result := testutil.Check(`
local MIME_TYPES: {[string]: string} = { txt = "text/plain" }
local function lookup(ext: string)
    if MIME_TYPES[ext] then
        return MIME_TYPES[ext]
    end
    return "application/octet-stream"
end
local mime_type: string? = lookup("txt")
return mime_type
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("guarded annotated map lookup must return string: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestAnnotatedMapCaptureGuardedLocalAssignsToOptionalString(t *testing.T) {
	result := testutil.Check(`
local MIME_TYPES: {[string]: string} = { txt = "text/plain" }
local function lookup(ext: string)
    local value = MIME_TYPES[ext]
    if value then
        return value
    end
    return "application/octet-stream"
end
local mime_type: string? = lookup("txt")
return mime_type
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("guarded map value must return string: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestAnnotatedMapCaptureStringReturnRejectsNumber(t *testing.T) {
	result := testutil.Check(`
local MIME_TYPES: {[string]: string} = { txt = "text/plain" }
local function lookup(ext: string)
    if MIME_TYPES[ext] then return MIME_TYPES[ext] end
    return "application/octet-stream"
end
local count: number = lookup("txt")
return count
`, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("string-valued helper must not satisfy a number annotation")
	}
}

func TestAnnotatedNumericMapCaptureStillRejectsString(t *testing.T) {
	result := testutil.Check(`
local COUNTS: {[string]: number} = { txt = 1 }
local function lookup(ext: string)
    if COUNTS[ext] then return COUNTS[ext] end
    return "fallback"
end
local mime_type: string? = lookup("txt")
return mime_type
`, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("a known numeric branch must not disappear behind pending evidence")
	}
}
