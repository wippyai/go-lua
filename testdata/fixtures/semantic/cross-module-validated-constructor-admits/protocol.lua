type Config = {
    id: string,
    retries: number,
}

type DecodeResult = {ok: true, value: Config} | {ok: false, error: string}

local M = {}
M.Config = Config
M.DecodeResult = DecodeResult

return M
