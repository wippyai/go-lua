
local function mock_ctx(state_config)
    local captured = {
        persisted = nil :: table?,
        upstream = nil :: table?,
        system_messages = 0,
        developer_messages = 0,
        switched_agent = nil :: string?,
        switched_model = nil :: string?,
    }

    local ctx = {
        config = nil,
        reader = {
            state = function()
                return { config = state_config or {} }
            end,
            reset = function()
                return true
            end,
        },
        writer = {
            update_meta = function(self, meta)
                captured.persisted = meta.config
                return true
            end,
            add_message = function(self, message_type)
                if message_type == "system" then
                    captured.system_messages = captured.system_messages + 1
                elseif message_type == "developer" then
                    captured.developer_messages = captured.developer_messages + 1
                end
                return "msg-id"
            end,
        },
        upstream = {
            update_session = function(self, payload)
                captured.upstream = payload
            end,
        },
        agent_ctx = {
            current_model = "model:new",
            switch_to_agent = function(self, agent_id)
                captured.switched_agent = agent_id
                self.current_model = "model:new"
                return true
            end,
            switch_to_model = function(self, model)
                captured.switched_model = model
                return true
            end,
        },
    }

    return ctx, captured
end
local function it(name: string, fn: () -> ())
    fn()
end

local function check_captured(expect: (any, any) -> ())
    it("agent_change persists and forwards the new agent", function()
        local ctx, captured = mock_ctx({ agent_id = "agent:old", model = "model:old" })
        ctx.writer:update_meta({ config = { agent_id = "agent:new", model = "model:new" } })
        expect((captured.persisted or {}).agent_id, "agent:new")
        expect((captured.persisted or {}).model, "model:new")
        expect((captured.upstream or {}).agent, "agent:new")
    end)
end

return check_captured
