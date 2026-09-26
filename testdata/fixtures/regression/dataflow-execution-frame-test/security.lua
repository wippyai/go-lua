type Actor = {
    id: (self: Actor) -> string,
    meta: (self: Actor) -> {[string]: any},
}

type Policy = {
    id: (self: Policy) -> string,
    evaluate: (self: Policy, actor: Actor, action: string, resource: string, context: any?) -> string,
}

type Scope = {
    with: (self: Scope, policy: Policy) -> Scope,
    without: (self: Scope, policy: any) -> Scope,
    evaluate: (self: Scope, actor: Actor, action: string, resource: string, context: any?) -> string,
    contains: (self: Scope, policy: any) -> boolean,
    policies: (self: Scope) -> {Policy},
}

local security = {}

function security.actor(): Actor?
    return nil
end

function security.scope(): Scope?
    return nil
end

function security.can(action: string, resource: string, context: any?): boolean
    return false
end

function security.policy(name: string): (Policy, error?)
    return nil :: any, nil
end

function security.named_scope(name: string): (Scope, error?)
    return nil :: any, nil
end

function security.new_scope(policies: {Policy}?): Scope
    return nil :: any
end

function security.new_actor(id: string, meta: any?): Actor
    return nil :: any
end

return security
