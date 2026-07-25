-- An opaque host callback can invoke clear and mutate the captured table. The
-- following typed read must therefore be rejected rather than trusting cfg.host.
local cfg = {}
cfg.host = "x"
local function clear()
    cfg.host = nil
end
external(clear)
local host: string = cfg.host -- expect-error
return host
