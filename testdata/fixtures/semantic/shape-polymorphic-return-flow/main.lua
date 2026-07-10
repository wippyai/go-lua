local helpers = require("helpers")

type Host = {
    host: string,
}

local cfg = {}
cfg.host = "x"
local exact: Host = helpers.id(cfg)

local row = {}
local source_meta = {}
source_meta.route = "local"
row.meta = source_meta
local meta = helpers.get_meta(row)
local route: string = meta.route

local fallback = {}
fallback.other = 1
local mixed: Host = helpers.default_or(cfg, fallback) -- expect-error

local touched: Host = helpers.touch(cfg) -- expect-error
local maybe: Host = helpers.maybe_store(cfg, false) -- expect-error

print(exact.host .. route)
