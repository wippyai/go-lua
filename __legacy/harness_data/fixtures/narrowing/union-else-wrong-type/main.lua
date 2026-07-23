type ChanInt = {__tag: "int"}
type ChanStr = {__tag: "str"}
type SelResult =
    {channel: ChanInt, value: number, ok: boolean} |
    {channel: ChanStr, value: string, ok: boolean}

function get_result(a: ChanInt, b: ChanStr, use_str: boolean): SelResult
    if use_str then
        return {channel = b, value = "ready", ok = true}
    end
    return {channel = a, value = 42, ok = true}
end

function f(ch1: ChanInt, ch2: ChanStr, use_str: boolean)
    local result = get_result(ch1, ch2, use_str)
    if result.channel == ch1 then
        local n: number = result.value
    else
        local n: number = result.value -- expect-error
    end
end
