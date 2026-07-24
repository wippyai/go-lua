-- EFFECT ROW: a function replacement passed to string.gsub composes its own row
-- into the call site, and the site allocates the rebuilt string.
local function shout(word: string): string
    return word:upper()
end

local text: string = "alpha beta"
local loud, count = string.gsub(text, "%a+", shout)

return loud, count
