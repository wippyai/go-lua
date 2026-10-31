package regression

import "testing"

func TestOptionalDiscriminatedUnionKeepsLiteralTag(t *testing.T) {
	checkBothModes(t, `
type Reply = {kind: "empty"} | {kind: "item", value: string}
local function empty(): Reply? return {kind = "empty"} end
local function item(value: string): Reply? return {kind = "item", value = value} end
return empty, item`, "")
}

func TestOptionalDiscriminatedUnionRejectsMissingPayload(t *testing.T) {
	checkBothModes(t, `
type Reply = {kind: "empty"} | {kind: "item", value: string}
local function item(): Reply? return {kind = "item"} end
return item`, "cannot return")
}
