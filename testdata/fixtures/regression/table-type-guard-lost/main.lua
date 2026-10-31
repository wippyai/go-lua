-- From userspace.dataflow.node.agent:agent_checkpoint_test in dataflow/test.
local function content_text(marker: {content: string | {text: string?, content: string?}}): string
    local content = marker.content
    if type(content) == "table" then
        content = content.text or content.content or ""
    end
    return content
end

local function unguarded(marker: {content: string | {text: string?}})
    local content = marker.content
    content = content.text or "" -- expect-error: missing on string
    return content
end

return content_text({content = {text = "checkpointed"}})
