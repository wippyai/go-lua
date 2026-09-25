type Conn = { find: (self: Conn, any) -> (any?, string?), ensure_thread: (self: Conn, any) -> (any?, string?) }
type Def = { with_actor: (self: Def, any) -> Def, with_scope: (self: Def, any) -> Def, open: (self: Def) -> (Conn?, string?) }
return {} :: { get: (string) -> (Def?, string?) }
