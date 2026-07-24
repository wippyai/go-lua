-- Every element is a freshly constructed record, so each parent-to-child edge
-- carries its ownership mode and write-barrier obligation.
type Row = { id: number }

local rows: {Row} = {
    { id = 1 },
    { id = 2 },
    { id = 3 },
    { id = 4 },
}

return rows
