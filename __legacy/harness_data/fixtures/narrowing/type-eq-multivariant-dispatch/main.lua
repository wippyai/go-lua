type IntCell  = { kind: "number",  raw: number | string | boolean }
type TextCell = { kind: "string",  raw: number | string | boolean }
type FlagCell = { kind: "boolean", raw: number | string | boolean }
type Cell = IntCell | TextCell | FlagCell

local function add(a: number, b: number): number return a + b end
local function upper(s: string): string return s end
local function flip(b: boolean): boolean return not b end

local function render(cell: Cell): string
    if cell.kind == "number" and type(cell.raw) == cell.kind then
        return upper("n")
    elseif cell.kind == "string" and type(cell.raw) == cell.kind then
        return upper(cell.raw)
    elseif cell.kind == "boolean" and type(cell.raw) == cell.kind then
        if flip(cell.raw) then
            return "t"
        end
        return "f"
    end
    return "?"
end

local function total(cells: Cell[], want: string): number
    local sum: number = 0
    for _, cell in ipairs(cells) do
        if want == "number" and type(cell.raw) == want then
            sum = add(sum, cell.raw)
        end
    end
    return sum
end

local function misuse(cell: Cell): string
    if cell.kind == "number" and type(cell.raw) == cell.kind then
        return upper(cell.raw)
    end
    return ""
end

return #render({ kind = "string", raw = "x" }) + total({}, "number") + #misuse({ kind = "number", raw = 1 })
