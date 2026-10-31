-- Bee runtime-pin checker regression: bee.application:forms:700
-- Expected: Typed Field remains Field when reset mutates its optional text widget.
-- Actual: argument 1: expected Field, got Field.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.2 fails (1 diagnostic).
-- Check using pinlint2-driver/v1524/checker or pinlint2-driver/v162/checker.
type TextField = {value: string}
type Field = {kind: "text" | "number", text: TextField?, error: string?, baseline: string, validate: ((Field) -> string?)?}
type Form = {fields: {Field}}
local function text_set(field: TextField, value: string) field.value = value end
local function reset_field(field: Field)
 if field.kind == "text" and field.text then text_set(field.text, field.baseline) end
 field.error = nil
end
local function reset(form: Form)
 for _, field in ipairs(form.fields) do reset_field(field) end
end
return reset
