local provider = {}

function provider.meta(): { name: string }
    return { name = "model" }
end

return provider
