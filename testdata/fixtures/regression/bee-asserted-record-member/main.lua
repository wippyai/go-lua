-- Bee runtime-pin checker regression: bee.gov.persist:activation_store:455
-- Expected: The asserted Blob has string bytes (Bee checked_blob validates the original input).
-- Actual: argument 1: expected string, got unknown?.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.2 fails (1 diagnostic).
-- Check using pinlint2-driver/v1524/checker or pinlint2-driver/v162/checker.
type Blob = {bytes: string, digest: string}
type Request = {[string]: unknown}
local function consume(bytes: string): string return bytes end
local function receipt(input: Request): string
 return consume((input.receipt :: Blob).bytes)
end
return receipt
