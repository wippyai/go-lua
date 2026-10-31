type Msg = { field: string }

local function pass(ch: channel.Channel<Msg>): channel.Channel<Msg>
    return ch
end

local typed = channel.new(1) :: channel.Channel<Msg>
local held: channel.Channel<Msg> = pass(typed)
local r = channel.select({ held:case_receive() })
local field: string = r.value.field
local wrong: number = r.value.field -- expect-error: cannot assign string
return field
