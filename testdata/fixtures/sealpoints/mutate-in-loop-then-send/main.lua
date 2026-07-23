type Row = { n: number }

type Report = {
    rows: {Row},
    total: number,
}

local function accumulate(out: Channel<Report>, rows: {Row})
    local acc: Report = { rows = {}, total = 0 }
    for _, r in ipairs(rows) do
        acc.total = acc.total + r.n
        acc.rows[#acc.rows + 1] = r
    end
    out:send(acc)
end
