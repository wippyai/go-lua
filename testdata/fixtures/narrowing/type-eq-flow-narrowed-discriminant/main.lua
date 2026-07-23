type NumBox = { kind: "number", value: number | string }
type TextBox = { kind: "text", value: number | string }
type Box = NumBox | TextBox

local function need_number(n: number): number
    return n
end

local function check(box: Box): number
    if box.kind == "number" then
        if type(box.value) == box.kind then
            return need_number(box.value)
        end
    end
    return 0
end

check({ kind = "number", value = 7 })
