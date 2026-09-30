package regression

import "testing"

func TestGateRequestErrorNilAdmittingFields(t *testing.T) {
	checkBothModes(t, `
type RequestError = {message: string, code: any?, error: any?, metadata: any?, detailed_message: string?}
local function status(err: RequestError) end
local function run(err: {message: string, code?: any, error?: any, metadata?: {request_id?: unknown}, detailed_message?: string?})
 status(err)
end
return run`, "")
}

func TestGateDependenciesOptionalTopField(t *testing.T) {
	checkBothModes(t, `
type Contract = {run: () -> ()}
type Dependencies = {schedulable_contract: any, scope_resolver: any?}
local function build(deps: {schedulable_contract?: Contract, scope_resolver: Contract?}): Dependencies
 return deps
end
return build`, "")
}

func TestGateEnvelopeFitsUnknownErrorSlot(t *testing.T) {
	checkBothModes(t, `
type DataError = {code: string, auth_expired: boolean?}
type Envelope = {success: boolean, error: DataError, retry_after_ms: number?}
type WriteResult = {success: boolean, result: string?, error: unknown?, retry_after_ms: number?}
local function write(envelope: Envelope): WriteResult
 return envelope
end
return write`, "")
}

func TestGateNilAdmittingFieldsRejectWrongPresentValues(t *testing.T) {
	checkBothModes(t, `
type RequestError = {message: string, code: any?, detailed_message: string?}
local function status(err: RequestError) end
local function run(err: {message: string, code?: any, detailed_message?: number?})
 status(err)
end
return run`, "expected RequestError")
}

func TestGateEnvelopeRejectsConcreteErrorMismatch(t *testing.T) {
	checkBothModes(t, `
type Envelope = {success: boolean, error: {code: string}}
type WriteResult = {success: boolean, error: string?}
local function write(envelope: Envelope): WriteResult
 return envelope
end
return write`, "cannot return Envelope")
}
