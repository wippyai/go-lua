local function label(maybe: string?): string
    return "prefix:" .. maybe
end

return label(nil)
