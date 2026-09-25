type Actor = { id: (self: Actor) -> string, groups: (self: Actor) -> {string} }
type Scope = { name: (self: Scope) -> string }
return {} :: { actor: () -> Actor?, scope: () -> Scope? }
