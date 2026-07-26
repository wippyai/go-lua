-- A store at an integer index occupies a sequence position of its container.
-- Every proof measured against that sequence -- the length floor a guard
-- established and the border presence a read at the container's own length
-- consumes -- describes the container as it stood before the store, so the
-- store revokes them. A literal key addresses the same sequence position a
-- variable key does, and the revocation follows the container the index
-- addresses rather than the spelling of the key.

-- The proof stands where nothing writes: the guard's floor makes the border the
-- length operator returns name an occupied slot.
local function baseline(xs: {string}): string
    if #xs >= 1 then
        local n = #xs
        local v: string = xs[n]
        return v
    end
    return ""
end

-- The store clears the slot the floor was measured over. The border may now be
-- zero and the read at the captured length is nil.
local function cleared(xs: {string}): string
    if #xs >= 1 then
        local n = #xs
        xs[1] = nil
        local v: string = xs[n] -- expect-error
        return v
    end
    return ""
end

-- The same store through a variable key states the same thing.
local function cleared_through_variable(xs: {string}): string
    if #xs >= 1 then
        local n = #xs
        local k = 1
        xs[k] = nil
        local v: string = xs[n] -- expect-error
        return v
    end
    return ""
end

-- The revocation names one container: a store into another leaves this one's
-- floor standing.
local function other_container(xs: {string}, ys: {string}): string
    if #xs >= 1 then
        local n = #xs
        ys[1] = nil
        local v: string = xs[n]
        return v
    end
    return ""
end

-- A store at a named slot holds no sequence position, so it moves no border.
local function named_slot(xs: {string}, box: {tag: string}): string
    if #xs >= 1 then
        local n = #xs
        box.tag = "x"
        local v: string = xs[n]
        return v
    end
    return ""
end

return baseline, cleared, cleared_through_variable, other_container, named_slot
