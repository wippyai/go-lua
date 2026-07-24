-- The same literal gains a key after construction: the key set is not complete,
-- so the sealed-table fact is withheld for the whole module generation.

local levels = { debug = 10, info = 20, warn = 30 }
levels.fatal = 40

return levels.fatal
