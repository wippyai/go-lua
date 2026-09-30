package regression

import "testing"

func TestAssertedMemberWriteInvalidatesGuard(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"same_cast", `(raw :: T).f = nil`, "cannot assign"},
		{"string_write", `(raw :: T).f = "new"`, ""},
		{"other_field", `(raw :: T).other = nil`, ""},
		{"no_write", `print("read")`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkBothModes(t, `
type T = {f: string?, other: string?}
local function g(raw: unknown)
 if (raw :: T).f then
 `+tc.body+`
 local s: string = (raw :: T).f
 return s
 end
end
return g`, tc.want)
		})
	}
}

func TestAssertedNestedMemberWriteInvalidatesGuard(t *testing.T) {
	checkBothModes(t, `
type T = {child: {f: string?}}
local function g(raw: unknown)
 if (raw :: T).child.f then
  (raw :: T).child.f = nil
  local s: string = (raw :: T).child.f
  return s
 end
end
return g`, "cannot assign")
}

func TestAssertedRecordMemberUsesDeclaredType(t *testing.T) {
	checkBothModes(t, `
type Blob = {bytes: string, digest: string}
type Request = {[string]: unknown}
local function consume(bytes: string): string return bytes end
local function receipt(input: Request): string
 return consume((input.receipt :: Blob).bytes)
end
return receipt`, "")
}

func TestAssertedNestedRecordMemberUsesDeclaredType(t *testing.T) {
	checkBothModes(t, `
type Envelope = {blob: {bytes: string}}
local function receipt(input: {[string]: unknown}): string
 return (input.receipt :: Envelope).blob.bytes
end
return receipt`, "")
}

func TestAssertedRecordMemberReplacesKnownOperandType(t *testing.T) {
	checkBothModes(t, `
type Blob = {bytes: string}
local function receipt(input: {receipt: {bytes: number}}): string
 return (input.receipt :: Blob).bytes
end
return receipt`, "")
}

func TestAssertedRecordMemberReplacesGuardedAncestorProjection(t *testing.T) {
	checkBothModes(t, `
type Blob = {bytes: string}
local function receipt(input: {receipt: {bytes: number}}?): string
 if not input then return "" end
 return (input.receipt :: Blob).bytes
end
return receipt`, "")
}

func TestAssertedAnyMemberDoesNotRestoreOptionalOperandField(t *testing.T) {
	checkModes(t, `
type Module = {set_meta: (() -> boolean)?}
local function invoke(component: Module?): boolean
 if not component then return false end
 return (component :: any).set_meta()
end
return invoke`, "", "cannot return any, expected boolean")
}

func TestAscribedMemberLeavesOrdinaryDiscriminantReadsIntact(t *testing.T) {
	checkBothModes(t, `
type Shape = {kind: "a", value: string} | {kind: "b", value: number}
local function read(shape: Shape): string
 return shape.kind == "a" and shape.value or ""
end
return read`, "")
}

func TestAssertedRecordMemberKeepsBranchGuard(t *testing.T) {
	checkBothModes(t, `
type Blob = {bytes: string?}
local function receipt(input: {[string]: unknown}): string
 if (input.receipt :: Blob).bytes then
  return (input.receipt :: Blob).bytes
 end
 return ""
end
return receipt`, "")
}

func TestAssertedRecordMemberKeepsLogicalGuard(t *testing.T) {
	checkBothModes(t, `
type Blob = {bytes: string?}
local function consume(bytes: string): boolean return bytes ~= "" end
local function receipt(input: {[string]: unknown}): boolean?
 return (input.receipt :: Blob).bytes and consume((input.receipt :: Blob).bytes)
end
return receipt`, "")
}

func TestAssertedRecordMemberStillRejectsAbsentField(t *testing.T) {
	checkBothModes(t, `
type Blob = {bytes: string?}
local function receipt(input: {[string]: unknown}): string
 return (input.receipt :: Blob).bytes
end
return receipt`, "cannot return string?, expected string")
}

func TestUnassertedRecordMemberStillRejectsUnknown(t *testing.T) {
	checkBothModes(t, `
local function consume(bytes: string): string return bytes end
local function receipt(input: {[string]: unknown}): string
 return consume(input.bytes)
end
return receipt`, "argument 1: expected string, got unknown")
}

func TestAssertedRecordMemberStillRejectsDeclaredWrongType(t *testing.T) {
	checkBothModes(t, `
type Blob = {bytes: number, digest: string}
local function consume(bytes: string): string return bytes end
local function receipt(input: {[string]: unknown}): string
 return consume((input.receipt :: Blob).bytes)
end
return receipt`, "argument 1: expected string, got number")
}
