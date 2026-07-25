-- The residue of a positive constant modulus is bounded by that modulus: for
-- integer i, Lua's floor modulo puts i % 3 in {0, 1, 2} whatever the sign of i,
-- so (i % 3) + 1 indexes 1..3 and a proven length floor of 3 puts the read in
-- range. The i >= 0 guard carries no part of that proof; it only keeps the
-- index in the relation the read consults.
local function const_modulus(xs: {number}, i: integer): number
    if i >= 0 and #xs >= 3 then
        local v: number = xs[(i % 3) + 1]
        return v
    end
    return 0
end

-- The modulus exceeds the proven floor: i % 5 reaches 4, so the read can be
-- zs[5] on a three-element array and stays optional.
local function modulus_above_floor(zs: {number}, i: integer): number
    if i >= 0 and #zs >= 3 then
        local v: number = zs[(i % 5) + 1] -- expect-error
        return v
    end
    return 0
end

-- The offset moves the whole residue window past the floor: (i % 3) + 2 reaches
-- 4 on an array whose only proven length is 3.
local function offset_above_floor(ws: {number}, i: integer): number
    if i >= 0 and #ws >= 3 then
        local v: number = ws[(i % 3) + 2] -- expect-error
        return v
    end
    return 0
end

-- A negative modulus flips the residue sign: i % -3 lands in {-2, -1, 0}, so
-- the index reaches 1 at best and -1 at worst.
local function negative_modulus(ns: {number}, i: integer): number
    if i >= 0 and #ns >= 3 then
        local v: number = ns[(i % -3) + 1] -- expect-error
        return v
    end
    return 0
end

-- A float dividend gives a float residue: 5.5 % 3 is 2.5 and fs[3.5] is nil,
-- so the residue window only bounds an integer dividend.
local function float_dividend(fs: {number}, x: number): number
    if x >= 0 and #fs >= 3 then
        local v: number = fs[(x % 3) + 1] -- expect-error
        return v
    end
    return 0
end

return const_modulus, modulus_above_floor, offset_above_floor, negative_modulus, float_dividend
