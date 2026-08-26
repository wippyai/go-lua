// Package carriertype adapts one sealed schema carrier authority to the
// relation model's owner-qualified TypeID.
//
// The adapter has no registry or fallback policy. It accepts an authority only
// after replaying carrier issuance against the exact authority.Owner.Entry;
// the owner's model identity and the carrier's issued content are then
// carried unchanged into model.IssueTypeID.
package carriertype
