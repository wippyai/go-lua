local function build(): integer
    local scratch = {
        value = 1,
    }
    local function read(): integer
        return scratch.value
    end
    return read()
end

local out: integer = build()
