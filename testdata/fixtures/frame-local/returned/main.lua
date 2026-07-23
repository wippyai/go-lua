type Scratch = {
    value: integer,
}

local function build(): Scratch
    local scratch: Scratch = {
        value = 1,
    }
    return scratch
end

local out: Scratch = build()
