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

local json = {}

function json.unmarshal<T>(data: string, witness: Type<T>): T
    return witness.decode(data)
end

local payload = json.unmarshal("{}", PayloadType)
local id: string = payload.id
local bad_id: number = payload.id -- expect-error
local count: number = payload.count
local bad_count: string = payload.count -- expect-error

local timer = json.unmarshal("{}", TimerType)
local elapsed: number = timer.elapsed
local bad_elapsed: string = timer.elapsed -- expect-error

local wrong_payload: Payload = json.unmarshal("{}", TimerType) -- expect-error

local raw: any = {
    id = "from-ai",
    count = "bad",
}

local untrusted: Payload = raw -- expect-error

return payload
