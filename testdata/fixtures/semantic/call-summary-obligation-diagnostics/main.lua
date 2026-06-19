local function invoke(provider, payload)
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "bad")
