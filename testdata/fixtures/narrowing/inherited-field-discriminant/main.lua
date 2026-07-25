-- The discriminant lives on the prototype, not on the instance. Each instance
-- table carries only its payload, so every read of the tag resolves through
-- __index. The prototypes are sealed here: each is installed once and neither the
-- metatable nor the index table is written again, so the tag a read resolves is
-- the literal the prototype was built with.

type Circle = { kind: "circle", radius: number }
type Square = { kind: "square", side: number }
type Shape = Circle | Square

local CircleProto = { kind = "circle" }
CircleProto.__index = CircleProto

local SquareProto = { kind = "square" }
SquareProto.__index = SquareProto

-- An inherited read is typed by the prototype that supplies it.
local function inherited_read(): string
    local obj = setmetatable({ radius = 1 }, CircleProto)
    local tag: string = obj.kind
    return tag
end

-- A prototype-supplied tag is a discriminant like any other: the instance carries
-- the arm its sealed prototype names.
local function circle(radius: number): Shape
    return setmetatable({ radius = radius }, CircleProto)
end

local function square(side: number): Shape
    return setmetatable({ side = side }, SquareProto)
end

local function area(s: Shape): number
    if s.kind == "circle" then
        return s.radius * s.radius * 3
    end
    return s.side * s.side
end

-- The prototype supplies a string, so an inherited read is not a licence to
-- assume anything else about it.
local function wrong_inherited_type(): number
    local obj = setmetatable({ radius = 1 }, CircleProto)
    local tag: number = obj.kind -- expect-error
    return tag
end

-- A field no link of the chain defines is absent, not open.
local function absent_field(): string
    local obj = setmetatable({ radius = 1 }, CircleProto)
    local nope: string = obj.absent -- expect-error
    return nope
end

-- The metatable supplies the tag, never the payload: this instance reads as
-- kind "circle" with no radius, so it is neither arm of the union.
local function wrong_payload(): Shape
    return setmetatable({ side = 1 }, CircleProto) -- expect-error
end

-- An unsealed chain proves nothing: __index is repointed after the instance is
-- built, so the base that supplied the payload is no longer the one a read
-- resolves through.
local function repointed_base(): number
    local first = { kind = "circle", radius = 2 }
    local second = { kind = "square" }
    local meta = { __index = first }
    local obj = setmetatable({}, meta)
    meta.__index = second
    local r: number = obj.radius -- expect-error
    return r
end

return inherited_read, area(circle(2)) + area(square(3)), wrong_inherited_type,
    absent_field, wrong_payload, repointed_base
