-- A boolean literal field is a two-case discriminant: the branch is a dense
-- bounded-index jump over both variants with no default lane.

type Ok = { ok: true, value: number }
type Err = { ok: false, message: string }
type Result = Ok | Err

local function render(r: Result): string
    if r.ok then
        return tostring(r.value)
    end
    return r.message
end

return render
