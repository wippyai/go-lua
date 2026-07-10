type A = {tag: "a", value: string}
type B = {tag: "b", value: number}

local function check(r: A | B)
    if r.tag == "a" then
    else
        local s: string = r.value -- expect-error: cannot assign
    end
end
