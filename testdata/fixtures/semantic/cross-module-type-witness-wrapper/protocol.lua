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

type TextNode = {
    kind: "text",
    value: string,
}

type GroupNode = {
    kind: "group",
    children: {TreeNode},
}

type TreeNode = TextNode | GroupNode

type Source = {
    records: Channel<RawRecord>,
    timers: Channel<Timer>,
    nodes: Channel<Node>,
    trees: Channel<TreeNode>,
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

function M.tree_type(): Type<TreeNode>
    return {
        decode = function(raw: any): TreeNode
            if tostring(raw) == "text" then
                return {
                    kind = "text",
                    value = tostring(raw),
                }
            end
            return {
                kind = "group",
                children = {
                    {
                        kind = "text",
                        value = tostring(raw),
                    },
                },
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
