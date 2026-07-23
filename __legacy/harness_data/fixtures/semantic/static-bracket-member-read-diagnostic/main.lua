type Dog = {kind: "dog", bark: string}
type Cat = {kind: "cat", meow: string}
type Animal = Dog | Cat

local function read_static(a: Animal): ()
    if a.kind == "dog" then
        local bad = a["meow"]
        local ok = a["bark"]
    else
        local ok = a["meow"]
    end
end

local function read_dynamic(a: Animal, key: string): ()
    if a.kind == "dog" then
        local unknown = a[key]
    end
end
