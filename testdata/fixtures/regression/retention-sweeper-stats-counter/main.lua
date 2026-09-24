local test = require("test")
local sweeper = require("sweeper")

local function prunes(db)
    local stats, err = sweeper.run(db, { days = 30 })
    test.is_nil(err)
    test.eq(stats.data, 2)
    test.eq(stats.commits, 1)
    local again, again_err = sweeper.run(db, { days = 30 })
    test.is_nil(again_err)
    test.eq(again.data + again.commits, 0)
    local as_text: string = again.data -- expect-error: cannot assign number to string
end

local function protects(db)
    local stats, err = sweeper.run(db, { days = 30 })
    test.is_nil(err)
    test.eq(stats.data + stats.commits, 0)
    local candidates: string = stats.data_candidates -- expect-error: cannot assign integer to string
end

return { prunes = prunes, protects = protects }
