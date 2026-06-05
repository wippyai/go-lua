type Type<T> = {
    decode: (any) -> T,
}

type RawRecord = {
    id: string,
    amount: number,
}

type Timer = {
    elapsed: number,
}

type Node = {
    id: string,
    children: {Node},
}

type Source = {
    records: Channel<RawRecord>,
    timers: Channel<Timer>,
    nodes: Channel<Node>,
}

type ListenOptions<T> = {
    channel: Channel<T>,
    decode: Type<T>,
}

local M = {}

function M.raw_record_type(): Type<RawRecord>
    return {
        decode = function(raw: any): RawRecord
            return {
                id = tostring(raw),
                amount = 1,
            }
        end,
    }
end

function M.raw_record_array_type(): Type<{RawRecord}>
    return {
        decode = function(raw: any): {RawRecord}
            return {
                {
                    id = tostring(raw),
                    amount = 1,
                },
            }
        end,
    }
end

function M.timer_type(): Type<Timer>
    return {
        decode = function(_raw: any): Timer
            return {
                elapsed = 1,
            }
        end,
    }
end

function M.node_type(): Type<Node>
    return {
        decode = function(raw: any): Node
            return {
                id = tostring(raw),
                children = {},
            }
        end,
    }
end

M.RawRecordType = {
    decode = function(raw: any): RawRecord
        return {
            id = tostring(raw),
            amount = 1,
        }
    end,
} :: Type<RawRecord>

M.RawRecordArrayType = {
    decode = function(raw: any): {RawRecord}
        return {
            {
                id = tostring(raw),
                amount = 1,
            },
        }
    end,
} :: Type<{RawRecord}>

M.TimerType = {
    decode = function(_raw: any): Timer
        return {
            elapsed = 1,
        }
    end,
} :: Type<Timer>

return M
