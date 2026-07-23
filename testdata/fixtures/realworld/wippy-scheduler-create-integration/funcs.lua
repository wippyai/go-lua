local security = require("security")
local target = require("target")

type Executor = {
    actor: security.Actor?,
    scope: security.Scope?,
    context: {[string]: any}?,
    with_actor: (self: Executor, actor: security.Actor) -> Executor,
    with_scope: (self: Executor, scope: security.Scope) -> Executor,
    with_context: (self: Executor, context: {[string]: any}) -> Executor,
    call: (self: Executor, name: "ns:target", input: target.Input) -> (target.RunResult?, string?),
}

local M = {}
M.Executor = Executor

function M.new(): Executor
    local executor: Executor
    executor = {
        actor = nil,
        scope = nil,
        context = nil,
        with_actor = function(self: Executor, actor: security.Actor): Executor
            self.actor = actor
            return self
        end,
        with_scope = function(self: Executor, scope: security.Scope): Executor
            self.scope = scope
            return self
        end,
        with_context = function(self: Executor, context: {[string]: any}): Executor
            self.context = context
            return self
        end,
        call = function(self: Executor, name: "ns:target", input: target.Input): (target.RunResult?, string?)
            local actor = self.actor
            if not actor then
                return nil, "missing actor"
            end
            local scope = self.scope
            if not scope then
                return nil, "missing scope"
            end
            return target.run(input, actor, scope)
        end,
    }
    return executor
end

return M
