local function need_string(value: string): string
    return value
end

local function need_integer(value: integer): integer
    return value
end

local function match_guard(s: string): string
    local m = s:match("%d+")
    if m == nil then
        return ""
    end
    return need_string(m)
end

local function match_unguarded(s: string): string
    local m = s:match("%d+")
    return need_string(m) -- expect-error: may be nil
end

local function match_captures(s: string): string
    local key, value = s:match("(%w+)=(%w+)")
    if key == nil or value == nil then
        return ""
    end
    return need_string(key) .. need_string(value)
end

local function match_position_capture(s: string): integer
    local pos = s:match("()id")
    if pos == nil then
        return 0
    end
    return need_integer(pos)
end

local function find_captures(s: string): integer
    local start_pos, end_pos, id = s:find("id=(%d+)")
    if start_pos == nil or end_pos == nil or id == nil then
        return 0
    end
    return need_integer(start_pos) + need_integer(end_pos) + #need_string(id)
end

local function gsub_destructure(s: string): string
    local replaced, count = s:gsub("%s+", " ")
    local exact_text: string = replaced
    local exact_count: integer = count
    return exact_text .. tostring(exact_count)
end

local function gsub_wrong_count(s: string): string
    local _, count = s:gsub("x", "y")
    local bad: string = count -- expect-error: count
    return bad
end

local function byte_guard(s: string): integer
    local b = s:byte(1)
    if b == nil then
        return 0
    end
    return need_integer(b)
end

local function byte_unguarded(s: string): integer
    local b = s:byte(999)
    return need_integer(b) -- expect-error: may be nil
end

local function gmatch_words(s: string): {string}
    local words: {string} = {}
    for word in s:gmatch("%a+") do
        table.insert(words, word)
    end
    return words
end

local function gmatch_captures(s: string): string
    local out = ""
    for key, value in s:gmatch("(%w+)=(%w+)") do
        if value == nil then
            value = ""
        end
        out = out .. need_string(key) .. need_string(value)
    end
    return out
end

local function gmatch_position_captures(s: string): integer
    local total = 0
    for pos, word in s:gmatch("()(%w+)") do
        if word == nil then
            word = ""
        end
        total = total + need_integer(pos) + #need_string(word)
    end
    return total
end

local function no_annotation_needed(s: string)
    local normalized, replacements = s:lower():gsub("%s+", "-")
    local token = normalized:match("^([%w-]+)$")
    if token == nil then
        token = ""
    end
    return {
        text = token:upper():reverse():sub(1, 10):rep(1),
        count = replacements,
        code = string.char(65),
        formatted = ("%s:%d"):format(token, replacements),
    }
end

return {
    match_guard = match_guard,
    match_unguarded = match_unguarded,
    match_captures = match_captures,
    match_position_capture = match_position_capture,
    find_captures = find_captures,
    gsub_destructure = gsub_destructure,
    gsub_wrong_count = gsub_wrong_count,
    byte_guard = byte_guard,
    byte_unguarded = byte_unguarded,
    gmatch_words = gmatch_words,
    gmatch_captures = gmatch_captures,
    gmatch_position_captures = gmatch_position_captures,
    no_annotation_needed = no_annotation_needed,
}
