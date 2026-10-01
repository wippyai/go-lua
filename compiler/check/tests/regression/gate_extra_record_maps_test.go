package regression

import "testing"

func TestExtraGateOpenPayloadFitsOptionalMap(t *testing.T) {
	checkBothModes(t, `
type Command = {type: string, payload: {[string]: any}?}
local function dispatch(command: Command) end
local function run(copied)
 local command = {type = "UPDATE" :: string, payload = copied}
 dispatch(command)
end
return run({component_id = "id"})`, "")
}

func TestExtraGateNestedSchemaFitsOptionalMap(t *testing.T) {
	checkBothModes(t, `
type Delegate = {enabled: boolean, default_schema: {
 type: string, properties: {[string]: any}?, required: any?
}}
type Config = {delegate_tools: Delegate?}
local schema = {type = "object", properties = {
 message = {type = "string", description = "Message"}
}, required = {"message"}}
local delegate = {enabled = true, default_schema = schema}
local function build(delegate)
 local config: Config = {delegate_tools = delegate}
 return config
end
return build(delegate)`, "")
}

func TestExtraGateMapWideningRejectsWrongValues(t *testing.T) {
	checkBothModes(t, `
type Command = {payload: {[string]: string}?}
local function dispatch(command: Command) end
dispatch({payload = {count = 1}})`, "expected Command")
}

func TestExtraGateMapWideningRejectsWrongKeys(t *testing.T) {
	checkBothModes(t, `
type Command = {payload: {[string]: any}?}
local function dispatch(command: Command) end
local function run(payload: {[integer]: string})
 dispatch({payload = payload})
end
return run`, "expected Command")
}
