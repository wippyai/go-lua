package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

const indexedBuilderModule = `
local M = {}
function M.build2(data: any)
    local embeddings = table.create(#data, 0)
    for i, item in ipairs(data) do
        embeddings[i] = item.embedding
    end
    local r = { embeddings = embeddings }
    return r
end
function M.build3(data: any)
    local embeddings = table.create(#data, 0)
    for i, item in ipairs(data) do
        embeddings[i] = item.embedding
    end
    local r = { result = { embeddings = embeddings } }
    return r
end
return M
`

// A returned local reads its value at the return point: the list the loop
// filled through an index, not the empty table its initializer made.
func TestReturnedLocalReadsIndexWrites(t *testing.T) {
	mod := testutil.CheckAndExport(indexedBuilderModule, "m", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatalf("module errors: %v", testutil.ErrorMessages(mod.Errors))
	}
	rec := unwrap.Alias(mod.Manifest.Export).(*typ.Record)
	for _, name := range []string{"build2", "build3"} {
		ret := rec.GetField(name).Type.(*typ.Function).Returns[0]
		r, _ := unwrap.Alias(ret).(*typ.Record)
		if name == "build3" && r != nil {
			r, _ = unwrap.Alias(r.GetField("result").Type).(*typ.Record)
		}
		if r == nil || r.GetField("embeddings") == nil {
			t.Fatalf("%s returns %v", name, ret)
		}
		emb := r.GetField("embeddings").Type
		if e, ok := unwrap.Alias(emb).(*typ.Record); ok && len(e.Fields) == 0 && !e.HasMapComponent() {
			t.Fatalf("%s embeddings = %v, want the indexed list", name, emb)
		}
	}
	consumer := testutil.Check(`
local m = require("m")
local function first()
    local r = m.build3({})
    return r.result.embeddings[1]
end
return first
`, testutil.WithStdlib(), testutil.WithModule("m", mod))
	if consumer.HasError() {
		t.Fatalf("consumer errors: %v", testutil.ErrorMessages(consumer.Errors))
	}
}
