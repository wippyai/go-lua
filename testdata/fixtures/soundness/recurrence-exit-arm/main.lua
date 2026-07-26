-- The branch that ends a loop is evaluated once per trip: the arm that stays
-- inside runs on every trip that continued and the arm that leaves runs on the
-- trip that ended. The value one trip carried into the condition therefore
-- decides that trip and never the loop, so both arms of that branch stay
-- reachable. An arm left unselected is an arm never checked, and every
-- statement the exit arm alone reaches stands behind it.

-- The readings that do not depend on a loop condition come first, so what they
-- state is measured on its own.

-- A statement no loop stands behind is decided where it is written.
local before: string = 5 -- expect-error

-- A guard that ends no loop keeps deciding exactly as it did: this body writes
-- the counter once and never re-enters the test, so the arm the single
-- execution refutes carries no obligation.
local straight: integer = 0
if straight > 3 then
    local unreachable: string = 5
end

-- An exit edge the front states in its own right carries the statements behind
-- it. A break is such an edge and a numeric for states its own bounds, so
-- neither rests on what the head condition holds on any one trip.
local from_break: string = ""
while true do
    break
end
from_break = 5 -- expect-error

local from_numeric: string = ""
for _ = 1, 3 do
end
from_numeric = 5 -- expect-error

-- A condition this analysis never decides keeps the reading it already had.
local function scan(more: () -> boolean): string
    local seen: string = ""
    while more() do
        seen = "x"
    end
    local reported: string = 5 -- expect-error
    return reported
end

-- The exit arm keeps the condition it left on: the loop ends where the carrier
-- is nil, and that is what the statements past it read.
local function drain(next_item: () -> string?): string
    local item: string? = next_item()
    while item ~= nil do
        item = next_item()
    end
    local done: string = item -- expect-error
    return done
end

-- A counter reaches its condition holding the value it entered the cycle with.
-- The trips that follow carry other values, so the entering one states nothing
-- about which arm the loop leaves on, and the loop does leave.
local counted: integer = 0
local from_counter: string = ""
while counted < 3 do
    counted = counted + 1
end
from_counter = 5 -- expect-error

-- A flag the body clears reads the same way: the value that opened the loop is
-- one trip's, and the trip that closes the flag is the one that leaves.
local open: boolean = true
local from_flag: string = ""
while open do
    open = false
end
from_flag = 5 -- expect-error

-- A test placed after the body decides the same way: the counter it reads is
-- the one the completed trip left, and the trips that follow leave others.
local repeated: integer = 0
local from_repeat: string = ""
repeat
    repeated = repeated + 1
until repeated >= 3
from_repeat = 5 -- expect-error

-- The same reading in the other direction: an entering value that refutes the
-- condition refutes it for the first trip alone, so the body stays reachable.
local refuted: integer = 0
while refuted > 3 do
    local inside: string = 5 -- expect-error
    refuted = refuted + 1
end

-- A condition that resolves to a constant is that constant on every trip, but
-- this analysis states no non-termination proof: the arm that leaves the loop
-- is one of the executions it must still check, so what stands behind it is
-- checked rather than dropped.
local function truthy(): boolean
    return true
end

local from_call: string = ""
while truthy() do
end
from_call = 5 -- expect-error

return before, straight, from_break, from_numeric, from_repeat, scan, drain,
    from_counter, from_flag, refuted, from_call
