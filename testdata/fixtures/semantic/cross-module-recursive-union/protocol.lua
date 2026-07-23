type Text = {
    kind: "text",
    value: string,
}

type Group = {
    kind: "group",
    children: {Node},
}

type Node = Text | Group

local M = {}
M.Node = Node
M.Group = Group
M.Text = Text

return M
