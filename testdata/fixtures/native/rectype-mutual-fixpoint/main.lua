-- Two mutually recursive record types both reach a coinductive fixpoint: each link
-- resolves to a stable identity, so neither traversal re-derives a shape.

type Section = { title: string, body: Block? }
type Block = { text: string, owner: Section }

local function root_title(s: Section): string
    local b = s.body
    if b == nil then
        return s.title
    end
    return b.owner.title
end

return root_title
