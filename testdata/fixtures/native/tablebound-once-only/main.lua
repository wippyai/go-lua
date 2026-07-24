-- A stable-table header constructor outside any loop has an exact maximum
-- occurrence count of one.
type Header = { id: number }

local header: Header = { id = 1 }

return header
