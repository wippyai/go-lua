type Actor = {
    id: (self: Actor) -> string,
    meta: (self: Actor) -> {[string]: any},
}

local security = {}

function security.actor(): Actor?
    return nil
end

function security.can(action: string, resource: string, context: any?): boolean
    return false
end

return security
