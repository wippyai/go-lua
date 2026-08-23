package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/protocol"
)

// contentIDCodecFence pins the layout version the sealed target digest is
// framed under. Raising contentIDCodecVersion invalidates every persisted
// target ContentID, so the constant moves only together with this fence and
// the version note that says what changed.
const contentIDCodecFence = 29

func TestContentIDCodecVersionIsFenced(t *testing.T) {
	if contentIDCodecVersion != contentIDCodecFence {
		t.Fatalf("contentIDCodecVersion is %d, the fence pins %d; raising it invalidates every persisted target ContentID and must move this fence in the same change",
			contentIDCodecVersion, contentIDCodecFence)
	}
}

// The requirement relation is inside the digest preimage. Its record tag is
// owned by the same target-contract record space every other sealed relation
// is framed under, so no two relations can share a tag and be confused for one
// another in the preimage.
func TestRequirementRecordBelongsToTheContractRecordSpace(t *testing.T) {
	tags := map[uint64]string{
		recordContract: "contract", recordOperation: "operation", recordBinding: "binding",
		recordValues: "values", recordOutcome: "outcome", recordCallback: "callback",
		recordSubedge: "subedge", recordCallbackRelease: "callback release",
		recordSuspension: "suspension", recordSpawn: "spawn", recordResume: "resume",
		recordTransfer: "transfer", recordEffect: "effect", recordProduced: "produced",
		recordCapture: "capture", recordCallbackResult: "callback result",
		recordResultAlias: "result alias", recordProtocol: "protocol", recordState: "state",
		recordAcquisition: "acquisition", recordTransition: "transition",
		recordTransitionOutcome: "transition outcome", recordEscape: "escape",
		recordProtocolCallbackHolder: "protocol callback holder", recordInitialRoot: "initial root",
		recordBootShape: "boot shape", recordInitialEntry: "initial entry",
		recordInitialBinding: "initial binding", recordInitialValue: "initial value",
		recordFreshResult: "fresh result", recordInitialMetatableAttachment: "initial metatable attachment",
		recordOperationSubedgeRelation: "operation subedge relation",
		recordProtocolRequirement:      "protocol requirement",
	}
	if prior, taken := tags[recordSealedColumn]; taken {
		t.Fatalf("the sealed column record tag %d is already the %s tag", recordSealedColumn, prior)
	}
	if prior, taken := tags[recordQualifiedType]; taken {
		t.Fatalf("the qualified type record tag %d is already the %s tag", recordQualifiedType, prior)
	}
	delete(tags, recordProtocolRequirement)
	if prior, taken := tags[recordProtocolRequirement]; taken {
		t.Fatalf("the protocol requirement record tag %d is already the %s tag", recordProtocolRequirement, prior)
	}
	if protocol.RequirementRecord() != recordProtocolRequirement {
		t.Fatalf("the protocol owner frames requirements under record %d, the contract space names %d",
			protocol.RequirementRecord(), recordProtocolRequirement)
	}
}
