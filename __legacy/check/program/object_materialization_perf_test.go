package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// Distilled from wippy.llm.discovery:models. A returned object literal must
// retain the exact correlations of several short-circuit-derived members
// without enumerating their Cartesian product as whole-heap lane terminals.
func TestObjectMaterializationFactorsGuardedMembers(t *testing.T) {
	runExternalCensusChunkWithGlobals(t, `
local function build(entry)
	return {
		id = entry.id or "",
		name = entry.meta and entry.meta.name or "",
		title = entry.meta and entry.meta.title or "",
		description = entry.meta and entry.meta.comment or "",
		capabilities = entry.meta and entry.meta.capabilities or {},
		class = entry.meta and entry.meta.class or {},
		priority = entry.meta and entry.meta.priority or 0,
		max_tokens = entry.data and entry.data.max_tokens or 0,
		output_tokens = entry.data and entry.data.output_tokens or 0,
		pricing = entry.data and entry.data.pricing or {},
		providers = entry.data and entry.data.providers or {},
	}
end

return build(entry)
`, map[string]typ.Type{"entry": typ.Any})
}
