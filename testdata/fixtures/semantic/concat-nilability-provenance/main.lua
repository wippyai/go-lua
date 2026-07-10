local function nilable_return(): string?
    return nil
end

local function from_return(): string
    return "return:" .. nilable_return()
end

type Payload = { name: string? }

local function from_field(payload: Payload): string
    return "field:" .. payload.name
end

local function from_unproven_guard(value: string?, checked: boolean): string
    if checked then
        print(value)
    end
    return "guard:" .. value
end

local raw: any = nil
local any_label = "any:" .. raw
print(any_label)

return from_return() .. from_field({}) .. from_unproven_guard(nil, false)
