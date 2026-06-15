# BUG: nil register after Go-function yield inside `pcall` on a resumed thread (register-pressure dependent)

Status: open. Found via the Wippy runtime (go-lua `v1.5.16`, as pinned by `github.com/wippyai/runtime`). Filed against the current `refactor/interproc-facts-domain` line because it lives in the `pcall` + yield-across-boundaries area already being reworked (`pcall_yield_test.go`, `yield_across_boundaries_test.go`).

## Symptom

A Go module function called from Lua **inside a `pcall`**, on a **resumed thread**, where the Go function itself **yields** (returns `-1` and resumes via continuation), reads a `nil` from a register that should hold a live value. The downstream Go code dereferences it and panics:

```
runtime error: invalid memory address or nil pointer dereference
```

go-lua's `threadRun` recover (`vm.go` ~3094) catches the `runtime.Error`, stringifies it (`lv = LString(fmt.Sprint(rcv))`), and a protected frame returns it to Lua — so the host sees a plain error string with **no Go stack**, which makes it very hard to locate from the outside.

## Trigger — all four are required

1. **Resumed thread.** The function runs on a thread that was *resumed* (not the caller's own thread). In Wippy this is `funcs.new():with_actor():with_scope():call(target, ...)` — the call yields a `CallYield`, and a worker resumes a thread to run `target`. Calling the identical Lua **directly** (same thread) never crashes.
2. **A `pcall` frame.** The body runs inside `pcall(body, ...)`.
3. **A yielding Go call inside that pcall.** The body calls a Go module function that suspends on I/O (`return -1` + continuation) and is later resumed. In Wippy this is a SQL write (`db:begin` / `exec` / `commit` all yield).
4. **Register pressure.** Enough locals + table-constructor arguments precede the yielding call that its argument registers sit at a high index.

Remove **any one** and the crash disappears:
- Call the yielding fn directly (no enclosing `pcall`, few locals) → OK.
- Same call but with **one extra statement before it** (even `pcall(function() return 1 + 1 end)`) → OK. This is the tell: the extra statement pre-grows the register array, so no resize happens at the bad moment.

It is fully **deterministic** for a given body (not flaky / not a data race).

## Suspected mechanism (not yet line-pinned)

Register-array growth is via `registry.forceResize` (`registry.go`):

```go
func (rg *registry) forceResize(newSize int) {
    newSlice := make([]LValue, newSize)
    copy(newSlice, rg.array[:rg.top]) // only copies up to top
    rg.array = newSlice
}
```

It copies **only `rg.array[:rg.top]`** — anything above `top` becomes `nil` in the new backing array.

Hypothesis: when the yielding Go function suspends/resumes inside a `pcall` frame on a resumed thread, `rg.top` is restored too low — it does not cover the suspended frame's live register window. A `reg.resize` that fires during the resumed execution (because register pressure pushes past `cap`) then truncates those live registers to `nil`. The resumed Lua then reads a `nil` where an argument table / value is expected, and the Go callee dereferences it.

This points at the interaction of `top` restoration across:
- `LState.ResumeInto` (`state.go` ~2147) → `threadRun` → `mainLoopWithContext`,
- the Go-yield path in `callGFunction` (`vm.go` ~3018, the `gfnret < 0` branch + `switchToParentThread`),
- protected-frame bookkeeping (`handleProtectedError`, the `pcall` sp markers in `baselib.go`),
- and `registry.resize` / `forceResize` copying `[:top]`.

The fix is presumably to ensure `rg.top` (or the saved frame top) covers all live registers of suspended frames across a Go-function yield under a protected call, so a later resize cannot truncate them. (Best confirmed by the Go reproduction below, which prints the real stack.)

## Reproduction (Wippy/Lua level — proven, deterministic)

This is the minimal self-contained reproduction that fails 100% of the time. It does not depend on any app logic — it only needs: a function invoked via `funcs:call` with a fresh actor/scope, an enclosing `pcall`, a yielding DB call, and register pressure.

`target.lua` (registered as a `function.lua`, method `run`; imports a module whose call yields on DB I/O, e.g. `userspace.scheduler.persist:schedule_repo`):

```lua
local security = require("security")
local time = require("time")
local pcall_wrap = require("automations_lib")        -- any helper doing pcall(body, t)
local schedule_calculator = require("schedule_calculator")
local schedule_repo = require("schedule_repo")

local function is_str(v) return type(v) == "string" and v ~= "" end

local function run(input)
    return pcall_wrap.install(function(t)             -- (2) pcall frame
        input = type(input) == "table" and input or {}
        local title = is_str(input.title) and input.title or "repro"
        local schedule_type = "interval"
        local schedule_expression = "1h"
        local agent_id = nil
        local agent_ref = "test_agent"

        local now_str = time.now():format(time.RFC3339)
        local next_run = schedule_calculator.next_interval_run(schedule_expression, nil, now_str)

        local actor = security.actor()
        local actor_id = actor and actor:id() or nil
        local actor_metadata = actor and actor:meta() or {}
        local user_id = is_str(input.user_id) and input.user_id or actor_id or nil
        local actor_scope = nil
        local groups = actor_metadata.security_groups
        if type(groups) == "table" and #groups > 0 then actor_scope = tostring(groups[#groups]) end
        local user_ctx = type(input.context) == "table" and input.context or {}

        local task_args = {                            -- (4) register pressure
            title = title, agent_id = agent_id, agent_ref = agent_ref,
            agent_title = nil, agent_icon = nil, kb_ids = nil, max_iterations = nil,
            actor_id = actor_id, actor_metadata = actor_metadata, actor_scope = actor_scope,
        }

        local schedule_id = schedule_repo.create({     -- (3) yields on DB I/O
            description = title, class = "automation", user_id = user_id,
            actor_id = actor_id, actor_scope = actor_scope, actor_metadata = actor_metadata,
            task_implementation_id = "x:y", task_args = task_args, task_context = user_ctx,
            schedule_type = schedule_type, schedule_expression = schedule_expression,
            next_run_at = next_run, timeout_seconds = 300, max_retries = 3, enabled = true,
        })

        t.rollback("x:y", { schedule_id = schedule_id })
        local state = {}
        for k, v in pairs(user_ctx) do state[k] = v end
        state.schedule_id = schedule_id
        return { state = state, metadata = { title = title } }
    end)
end

return { run = run }
```

Driver (test): call it on a **resumed thread** with a fresh actor + named scope:

```lua
local funcs = require("funcs")
local security = require("security")

local function dispatch(input)
    return funcs.new()                                 -- (1) resumed thread
        :with_actor(security.new_actor("repro_user", {}))
        :with_scope(security.named_scope("wippy.security:process"))
        :with_context({})
        :call("ns:target", input or {})
end

local result, err = dispatch({ title = "first" })
-- err == "runtime error: invalid memory address or nil pointer dereference"
```

Bisection facts established with this harness:
- `dispatch` → `schedule_repo.create` **directly** (no `pcall` wrapper, few locals): **passes**.
- The body above **inside `pcall`** with the full set of locals + big `task_args`: **fails**.
- Adding any extra statement (e.g. `pcall(function() return 1+1 end)`) before the yielding call: **passes**.
- Removing the big `task_args` table (lower register index): expected to pass (register-pressure threshold).

## Reproduction to write at the go-lua level (to pin the exact line)

Port the above to a pure go-lua Go test under `pcall_yield_test.go` / `yield_across_boundaries_test.go`, no Wippy:

1. Register a Go function `yielder(L)` that on first entry pushes a continuation and `return -1` (suspends), and on resume pushes a result table — mimicking a yielding I/O call. (See existing `CallK` / continuation usage.)
2. From Lua, run a body **inside `pcall`** that declares ~12–15 locals and builds a multi-field table literal, then calls `yielder(big_table)` and uses its result.
3. Drive it on a **resumed thread**: create a coroutine / second `LState` via `NewThreadWithContext`, `ResumeInto` it to run the body, and feed the yield/resume cycle.
4. Assert the body's result instead of a `nil`. Without the fix this panics with the nil-deref and the test prints the **full Go stack** — that pinpoints the faulting line (expected around `registry.forceResize` truncation vs `rg.top` restore on the yield/resume path).

A `-race` run is unnecessary (deterministic, single goroutine within the VM).

## Notes / red herrings

- The host-visible stack often shows `logger.loggerError` at `logger.go:176` — that is **unrelated**. zap attaches a stacktrace to every ERROR-level log, and the top frame of any such stacktrace is `loggerError`. Those traces were decorating normal ERROR logs (e.g. `no such table` during boot), not this panic. The real panic is recovered in `threadRun` and never logged with a stack.
- Not context-related: the actor/scope reach the function correctly (`security.actor()` returns the framed actor right before the crash). Context propagation is intact.
