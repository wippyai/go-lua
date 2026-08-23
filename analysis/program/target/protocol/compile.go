package protocol

import "errors"

// Compile freezes, resolves, validates, and canonicalizes protocol rows
// against the sealed operation geometry, then returns one immutable owner
// value. Callback coordinates must already be owner-issued in Input.
func Compile(input Input) (Table, error) {
	opaque, ok := input.Operations.Opaque()
	if !ok || opaque == 0 {
		return Table{}, errors.New("target/protocol: missing opaque operation")
	}
	if input.Operations.OperationCount() != input.Operations.SourceCount()+1 {
		return Table{}, errors.New("target/protocol: opaque operation is outside operation geometry")
	}
	protocols, err := freezeProtocols(input.Protocols)
	if err != nil {
		return Table{}, err
	}
	if err := resolveProtocols(protocols, input.Operations); err != nil {
		return Table{}, err
	}
	if err := resolveProtocolCallbackHolders(protocols, input.Operations); err != nil {
		return Table{}, err
	}
	table := Table{opaque: opaque}
	if err := table.appendProtocols(protocols); err != nil {
		return Table{}, err
	}
	if err := table.sealDemands(input.Operations); err != nil {
		return Table{}, err
	}
	return table, nil
}
