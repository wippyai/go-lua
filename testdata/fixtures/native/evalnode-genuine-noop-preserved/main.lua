-- Contract: a structurally empty program point stays a noop; the evaluation-node
-- patch must not relabel every noop as an operation.

local total: number = 0

if total > 0 then
end

do
end

return total
