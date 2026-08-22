-- Contract: the Wippy v1 expr module as the runtime declares it. Both module
-- members and the compiled Program method are called, each read on both arms
-- of its trailing error, and each in both its one-argument and its
-- context-carrying form.

local expr = require("expr")

local context = { threshold = 10, label = "alpha" }

local program, compile_err = expr.compile("threshold > 5")
if compile_err ~= nil then
    return compile_err:kind()
end

local seeded, seeded_err = expr.compile("threshold > 5", context)
if seeded_err ~= nil then
    return seeded_err:message()
end

local result, run_err = program:run()
if run_err ~= nil then
    return run_err:message()
end

local seeded_result, seeded_run_err = seeded:run(context)
if seeded_run_err ~= nil then
    return seeded_run_err:message()
end

local evaluated, eval_err = expr.eval("label")
if eval_err ~= nil then
    return eval_err:message()
end

local scoped, scoped_err = expr.eval("label", context)
if scoped_err ~= nil then
    return scoped_err:message()
end

return tostring(result) .. tostring(seeded_result) ..
    tostring(evaluated) .. tostring(scoped)
