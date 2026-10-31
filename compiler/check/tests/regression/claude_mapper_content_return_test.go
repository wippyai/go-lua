package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// The mapper copies a user's content through an ipairs/table.insert loop,
// appends it to messages, applies cache markers, and consolidates the result.
func TestClaudeMapperCachedContentKeepsElementType(t *testing.T) {
	source := `
local function process_content_array(content)
    if type(content) == "string" then
        return content
    elseif type(content) == "table" then
        local processed = {}
        for _, part in ipairs(content) do
            table.insert(processed, part)
        end
        return processed
    end
    return content
end

local function consolidate_messages(messages)
    if #messages <= 1 then
        return messages
    end
    local result = {}
    for _, msg in ipairs(messages) do
        if msg.role == "assistant" and #result > 0 and result[#result].role == "assistant" then
            for _, part in ipairs(msg.content) do
                table.insert(result[#result].content, part)
            end
        else
            table.insert(result, msg)
        end
    end
    return result
end

local function ensure_content_exists(messages)
    for i = 1, #messages - 1 do
        if not messages[i].content or #messages[i].content == 0 then
            messages[i].content = {{ type = "text", text = "intercepted" }}
        end
    end
    return messages
end

local function map_messages(contract_messages)
    if not contract_messages or #contract_messages == 0 then
        return { messages = {}, system = nil }
    end
    local messages = {}
    local positions = {}
    for _, msg in ipairs(contract_messages) do
        if msg.role == "cache_marker" then
            positions[#positions + 1] = { message = #messages, block = 1 }
        elseif msg.role == "developer" then
            table.insert(messages, {
                role = "user",
                content = {{ type = "text", text = msg.content }}
            })
        else
            local content = process_content_array(msg.content)
            if type(content) == "string" then
                content = {{ type = "text", text = content }}
            end
            table.insert(messages, { role = msg.role, content = content })
        end
    end
    for _, pos in ipairs(positions) do
        local msg: any = (messages :: any)[pos.message]
        if msg and msg.content and msg.content[pos.block] then
            msg.content[pos.block].cache_control = { type = "ephemeral" }
        end
    end
    messages = consolidate_messages(messages)
    messages = ensure_content_exists(messages)
    return { messages = messages, system = nil }
end

local first = map_messages({
    { role = "user", content = {{ type = "text", text = "Question" }} },
    { role = "cache_marker" }
})
local second = map_messages({
    { role = "user", content = {{ type = "text", text = "Question" }} },
    { role = "cache_marker" },
    { role = "developer", content = "Changing memory recall" }
})
local left: string = second.messages[1].content[1].text
local right: string = first.messages[1].content[1].text
return left == right
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected mapped content to retain its text block, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
