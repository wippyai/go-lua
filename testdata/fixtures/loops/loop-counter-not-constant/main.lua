-- A loop counter is not the value one iteration would leave behind. The trip
-- count is a property of the loop's bound, not of the counter's initializer, so
-- everything the counter reaches has to hold for every trip count the loop
-- admits -- including the branches that are only reachable after several trips.

-- The counter starts at zero and rises by one per trip. Ten trips are possible,
-- so `total >= 2` is a live arm and the annotation inside it is checked.
local total: integer = 0
for _ = 1, math.random(10) do
    total = total + 1
end
if total >= 2 then
    local reached: string = 1 -- expect-error
    print(reached)
end

-- The counter's representation survives the loop: integer addition is closed,
-- so the declared integer still holds however many trips ran.
local kept: integer = total
print(kept)

-- A counter the loop never touches keeps its exact value, so an arm the
-- initializer alone decides against stays unreachable and is not analysed.
local untouched: integer = 0
for _ = 1, math.random(10) do
    print(untouched)
end
if untouched >= 2 then
    local unreachable: string = 1
    print(unreachable)
end
