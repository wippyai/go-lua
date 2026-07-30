package engine

import (
	"strings"
	"testing"
)

// nativeContractValues collects the published native contract rows of one
// family for a checked module.
func nativeContractValues(t *testing.T, source, family string) []string {
	t.Helper()
	result, err := Check(source)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Native == nil {
		return nil
	}
	values := make([]string, 0)
	for _, fact := range result.Native.Facts() {
		if strings.HasPrefix(fact.Key, family+"/") {
			values = append(values, fact.Value)
		}
	}
	return values
}

func metatableInstallSource(prelude, callee string) string {
	return prelude + `local base = { f = 1 }
local mt = {}
mt.__index = base
local t = {}
` + callee + `(t, mt)
local n = #t
return t, n
`
}

// TestBaseLibraryContractFollowsTheBinding pins that the metatable contracts
// belong to the base library's own semantics, so they rest on the binding a
// call reaches rather than on the name it is spelled with. The global binding
// installs a metatable, whichever spelling names it.
func TestBaseLibraryContractFollowsTheBinding(t *testing.T) {
	for _, callee := range []string{"setmetatable", "_G.setmetatable"} {
		seals := nativeContractValues(t, metatableInstallSource("", callee), "metatable_seal")
		if len(seals) == 0 {
			t.Errorf("%s installed no metatable seal", callee)
		}
		lengths := nativeContractValues(t, metatableInstallSource("", callee), "table_length")
		if len(lengths) == 0 {
			t.Errorf("%s left the sequence border unqualified by its metamethod", callee)
		}
	}
}

// TestShadowedBaseLibraryNameInstallsNoContract pins the same relation from the
// other side. A local of the same spelling is an ordinary function: it installs
// no metatable, so a seal claiming an index table and a length withheld for a
// possible metamethod would both be false of the program that runs.
func TestShadowedBaseLibraryNameInstallsNoContract(t *testing.T) {
	const shadow = "local setmetatable = function(a, b) return a end\n"
	if seals := nativeContractValues(t, metatableInstallSource(shadow, "setmetatable"), "metatable_seal"); len(seals) != 0 {
		t.Errorf("a shadowed local sealed a metatable it never installs: %v", seals)
	}
	if lengths := nativeContractValues(t, metatableInstallSource(shadow, "setmetatable"), "table_length"); len(lengths) != 0 {
		t.Errorf("a shadowed local withheld a length for a metamethod it never installs: %v", lengths)
	}
}
