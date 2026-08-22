local channel = require("channel")

local function send_then_receive_then_close()
    local ch = channel.new()
    ch:send("ready")
    local _, _ = ch:receive()
    ch:close()
end

local function receive_after_close_is_permitted()
    local ch = channel.new()
    ch:send("last")
    ch:close()
    local _, _ = ch:receive()
end

local function alias_sends_before_the_single_close()
    local ch = channel.new()
    local alias = ch
    alias:send("via alias")
    ch:send("via origin")
    ch:close()
end

local function conditional_send_before_close(flag)
    local ch = channel.new()
    if flag then
        ch:send("conditional")
    end
    ch:close()
end
