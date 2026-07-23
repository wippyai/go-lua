type ActorMeta = {
    security_groups: {string},
}

type Actor = {
    id: (self: Actor) -> string,
    meta: (self: Actor) -> ActorMeta,
}

type Scope = {
    name: string,
}

local M = {}
M.ActorMeta = ActorMeta
M.Actor = Actor
M.Scope = Scope

function M.new_actor(id: string, meta: ActorMeta?): Actor
    local actor_meta = meta or {security_groups = {}}
    local actor: Actor = {
        id = function(self: Actor): string
            return id
        end,
        meta = function(self: Actor): ActorMeta
            return actor_meta
        end,
    }
    return actor
end

function M.named_scope(name: string): Scope
    return {name = name}
end

return M
