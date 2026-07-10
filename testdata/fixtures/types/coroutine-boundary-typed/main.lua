type Frame = { pc: number, label: string }
local function stepper(start: Frame): (number) -> Frame
    local cur: Frame = start
    return function(advance: number): Frame
        cur = { pc = cur.pc + advance, label = cur.label }
        return cur
    end
end
local step = stepper({ pc = 0, label = "main" })
local f: Frame = step(2)
return f.pc
