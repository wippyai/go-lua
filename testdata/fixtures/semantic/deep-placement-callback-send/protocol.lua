type Meta = {
    route: string,
    shard: string,
}

type Child = {
    id: string,
    meta: Meta,
}

type Item = {
    id: string,
    tags: {[string]: string},
    child: Child,
}

type Batch = {
    items: {[string]: Item},
    count: number,
}

local M = {}
M.Meta = Meta
M.Child = Child
M.Item = Item
M.Batch = Batch

return M
