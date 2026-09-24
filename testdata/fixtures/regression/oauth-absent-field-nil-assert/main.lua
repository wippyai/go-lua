-- From kickside.oauth.binding:get_connector_contract_func failure return and
-- kickside.oauth.discovery:discovery_miss_test absence assertion.
local test = require("test")
local result = { success = false, error = "Connector contract not found" }
test.is_nil(result.implementation_id, "no implementation must leak on a miss")
return result
