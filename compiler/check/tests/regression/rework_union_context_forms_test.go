package regression

import "testing"

func TestOptionalIntersectionUnionKeepsLiteralDiscriminant(t *testing.T) {
	checkBothModes(t, `
type Header = {version: integer}
type Empty = Header & {kind: "empty"}
type Item = Header & {kind: "item", value: string}
type Reply = Empty | Item
local function empty(): Reply? return {version = 1, kind = "empty"} end
local function item(value: string): Reply? return {version = 1, kind = "item", value = value} end
return empty, item`, "")
}

func TestOptionalIntersectionUnionRejectsMissingPayload(t *testing.T) {
	checkBothModes(t, `
type Header = {version: integer}
type Empty = Header & {kind: "empty"}
type Item = Header & {kind: "item", value: string}
type Reply = Empty | Item
local function item(): Reply? return {version = 1, kind = "item"} end
return item`, "cannot return")
}

func TestOptionalUnionWithTwoDiscriminantsKeepsLiteralFields(t *testing.T) {
	checkBothModes(t, `
type Reply = {kind: "env", present: true, encoding: "utf-8", value: string}
 | {kind: "env", present: false, encoding: "utf-8", value: nil}
 | {kind: "file", present: boolean, encoding: "utf-8" | "bytes", value: string?}
local function present(value: string): Reply?
 return {kind = "env", present = true, encoding = "utf-8", value = value}
end
local function absent(): Reply?
 return {kind = "env", present = false, encoding = "utf-8", value = nil}
end
return present, absent`, "")
}

func TestOptionalUnionWithTwoDiscriminantsRejectsWrongPayload(t *testing.T) {
	checkBothModes(t, `
type Reply = {kind: "env", present: true, encoding: "utf-8", value: string}
 | {kind: "env", present: false, encoding: "utf-8", value: nil}
 | {kind: "file", present: boolean, encoding: "utf-8" | "bytes", value: string?}
local function present(): Reply?
 return {kind = "env", present = true, encoding = "utf-8", value = nil}
end
return present`, "cannot return")
}
