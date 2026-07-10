type Type<T> = {
    decode: (any) -> T,
}

type Payload = {
    id: string,
    count: number,
}

type Timer = {
    elapsed: number,
}

local PayloadType: Type<Payload> = {
    decode = function(raw: any): Payload
        return {
            id = tostring(raw),
            count = 1,
        }
    end,
}

local TimerType: Type<Timer> = {
    decode = function(raw: any): Timer
        return {
            elapsed = 1,
        }
    end,
}

local PayloadArrayType: Type<{Payload}> = {
    decode = function(raw: any): {Payload}
        return {
            {
                id = tostring(raw),
                count = 1,
            },
        }
    end,
}

local json = {}

function json.decode<T>(data: string, witness: Type<T>): T
    return witness.decode(data)
end

local payload = json.decode("{}", PayloadType)
local id: string = payload.id
local bad_id: number = payload.id -- expect-error
local count: number = payload.count
local bad_count: string = payload.count -- expect-error

local timer = json.decode("{}", TimerType)
local elapsed: number = timer.elapsed
local bad_elapsed: string = timer.elapsed -- expect-error

local rows = json.decode("[]", PayloadArrayType)
local accepted_rows: {Payload} = rows
local wrong_rows: {Timer} = rows -- expect-error

for _, row in ipairs(rows) do
    local row_id: string = row.id
    local bad_row_id: number = row.id -- expect-error
    local row_count: number = row.count
    local bad_row_count: string = row.count -- expect-error
end

local wrong_payload: Payload = json.decode("{}", TimerType) -- expect-error

local raw: any = {
    id = "from-ai",
    count = "bad",
}

local untrusted: Payload = raw -- expect-error

return payload
