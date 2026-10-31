package regression

import (
	"strings"
	"testing"
)

func TestGateBooleanMutationRetainsRequiredValue(t *testing.T) {
	source := `
local cache: {[string]: {[string]: boolean}} = {}
local function implements(provider: string, contract_id: string, entries: {any}): boolean
 local cached = cache[provider]
 if cached and cached[contract_id] ~= nil then return cached[contract_id] == true end
 local has = false
 for _, entry in ipairs(entries or {}) do
  local data: any = entry.data
  local contracts: any = data and data.contracts
  if type(contracts) == "table" then
   for _, c in ipairs(contracts :: {any}) do
    if c.contract == contract_id then has = true break end
   end
  end
  if has then break end
 end
 if not cache[provider] then cache[provider] = {} end
 cache[provider][contract_id] = has
 return has
end
return implements`
	for _, tc := range []struct {
		name   string
		source string
	}{
		{"full", source},
		{"no_cache", strings.ReplaceAll(strings.ReplaceAll(source, " cache[provider][contract_id] = has", ""), " if not cache[provider] then cache[provider] = {} end", "")},
		{"no_nested_loop", strings.ReplaceAll(source, "for _, c in ipairs(contracts :: {any}) do", "do local c: any = {}")},
		{"no_outer_break", strings.ReplaceAll(source, "if has then break end", "")},
	} {
		t.Run(tc.name, func(t *testing.T) { checkBothModes(t, tc.source, "") })
	}

}

func TestGateBooleanMutationRejectsNilAlternative(t *testing.T) {
	checkBothModes(t, `
local function run(flag: boolean): boolean
 local has: boolean? = false
 if flag then has = nil end
 return has
end
return run`, "cannot return")
}

func TestGateNestedLoopBreakKeepsInitializedBoolean(t *testing.T) {
	checkBothModes(t, `
local function run(groups: {{boolean}}): boolean
 local has = false
 for _, group in ipairs(groups) do
  for _, item in ipairs(group) do
   if item then has = true break end
  end
  if has then break end
 end
 return has
end
return run`, "")
}
