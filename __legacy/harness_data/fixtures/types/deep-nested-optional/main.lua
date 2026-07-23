type Deep = { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: { next: number? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }? }?
local d: Deep = nil
local function f(x: Deep): Deep return x end
return f(d)
