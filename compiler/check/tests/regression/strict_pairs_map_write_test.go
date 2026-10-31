package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestStrictPairsStringMapWritePreservesKeyType(t *testing.T) {
	source := `
type Map = {[string]: any}
local function source(): Map
    return {status = "active"}
end
local function metadata(): Map
    return {title = "hello"}
end
local function copy(): Map
    local values = source()
    local target = metadata()
    for k, v in pairs(values) do
        target[k] = v
    end
    return target
end
return copy
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
	if result.HasError() {
		t.Fatalf("string-keyed map copy: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestStrictPairsUnknownKeyStillRejected(t *testing.T) {
	source := `
type Map = {[string]: any}
local function add(key: any): Map
    local target = {title = "hello"}
    target[key] = "value"
    return target
end
return add
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
	if !result.HasError() {
		t.Fatal("an any key must not prove a string-keyed map")
	}
}
