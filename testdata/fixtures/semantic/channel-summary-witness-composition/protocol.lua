type Ready = {
    kind: "ready",
    payload: {
        id: string,
    },
}

type Failed = {
    kind: "failed",
    reason: string,
}

type Message = Ready | Failed

type Tick = {
    elapsed: number,
}

type Source = {
    messages: Channel<Message>,
    ticks: Channel<Tick>,
}

type SourceBox = {
    value: Source,
}

local M = {}

function M.box_source(value: Source): SourceBox
    return {
        value = value,
    }
end

return M
