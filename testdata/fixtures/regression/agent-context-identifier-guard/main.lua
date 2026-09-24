local agent_context = {}
agent_context.__index = agent_context

type LoadOptions = { model: string? }

function agent_context.new()
    return setmetatable({ current_agent_id = nil :: string?, active_traits = nil :: {string}? }, agent_context)
end

function agent_context:switch_to_agent(agent_identifier: string | table, options: LoadOptions?): (boolean, string?)
    if not agent_identifier then
        return false, "Agent spec or identifier is required"
    end

    options = options or {}

    local target_id = type(agent_identifier) == "table"
        and (agent_identifier.id or agent_identifier.name)
        or agent_identifier
    if target_id == nil or target_id ~= self.current_agent_id then
        self.active_traits = nil
    end
    return true, nil
end

return agent_context
