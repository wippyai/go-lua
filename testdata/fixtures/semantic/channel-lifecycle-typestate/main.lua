local channel = require("channel")

local function clean()
    local ch = channel.new()
    ch:send("ready")
    local _, _ = ch:receive()
    ch:close()
end

local function receive_after_close_is_done()
    local ch = channel.new()
    ch:close()
    local _, _ = ch:receive()
end

local function send_after_close()
    local ch = channel.new()
    ch:close()
    ch:send("late")
end

local function send_after_alias_close()
    local ch = channel.new()
    local alias = ch
    alias:close()
    ch:send("late")
end

type ChannelWriter = (Channel<any>) -> ()

local function escaped_before_send(writer: ChannelWriter)
    local ch = channel.new()
    writer(ch)
    ch:send("after escape")
end

local function double_close()
    local ch = channel.new()
    ch:close()
    ch:close()
end

local function escaped_after_close(writer: ChannelWriter)
    local ch = channel.new()
    ch:close()
    writer(ch)
    ch:send("late but escaped")
end
