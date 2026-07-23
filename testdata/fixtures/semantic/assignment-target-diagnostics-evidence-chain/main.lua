type Bag = {name: string}

function update(bag: Bag?)
    bag.name = "ok" -- expect-error
end

type Point = {x: number, y: number}
local p: Point = {x = 10} -- expect-error
