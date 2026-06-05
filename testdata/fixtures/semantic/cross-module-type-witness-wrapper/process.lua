type ListenOptions<T> = {
    channel: Channel<T>,
    decode: {
        decode: (any) -> T,
    },
}

type NestedListenOptions<T> = {
    channel: Channel<T>,
    schema: {
        witness: {
            decode: (any) -> T,
        },
    },
}

local M = {}

function M.listen<T>(topic: string, options: ListenOptions<T>): Channel<T>
    return options.channel
end

function M.listen_nested<T>(topic: string, options: NestedListenOptions<T>): Channel<T>
    return options.channel
end

return M
