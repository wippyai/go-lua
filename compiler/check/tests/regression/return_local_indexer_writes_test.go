package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

const indexedBuilderModule = `
local M = { _client = require("openai_client") }
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
function M.embed(contract_args)
    if not contract_args.model or not contract_args.input then
        return nil, "invalid input"
    end
    local openai_response, req_err = M._client.request("/embeddings", contract_args)
    if req_err then return nil, req_err end
    if not openai_response or not openai_response.data or #openai_response.data == 0 then
        return nil, "invalid response"
    end
    local embeddings = table.create(#openai_response.data, 0)
    for i, item in ipairs(openai_response.data) do
        embeddings[i] = item.embedding
    end
    local contract_response = {
        success = true,
        result = { embeddings = embeddings },
        model = openai_response.model,
        metadata = openai_response.metadata or {}
    }
    if openai_response.usage then
        contract_response.tokens = {
            prompt_tokens = openai_response.usage.prompt_tokens or 0,
            total_tokens = openai_response.usage.total_tokens or openai_response.usage.prompt_tokens or 0
        }
    end
    return contract_response
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
	embedConsumer := testutil.Check(`
local m = require("m")
local response = m.embed({model = "small", input = "text"})
local first = response.result.embeddings[1]
return first[1]
`, testutil.WithStdlib(), testutil.WithModule("m", mod))
	if embedConsumer.HasError() {
		t.Fatalf("embed consumer errors: %v", testutil.ErrorMessages(embedConsumer.Errors))
	}
}
