package algebra

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// ObservationSource selects one owner row from an Apply invocation's sealed
// child/source vectors. Child and tuple are physical coordinates in the
// sealed expression; Source is the authored source position inside that
// tuple. None of these coordinates is an evaluation ordinal or an identity.
type ObservationSource struct {
	child  uint32
	tuple  uint32
	source uint32
}

// NewObservationSource declares one positional child/tuple/source selector.
// Bounds and relation membership are checked when the enclosing descriptor
// is admitted against the mounted operation and population.
func NewObservationSource(child, tuple, source uint32) ObservationSource {
	return ObservationSource{child: child, tuple: tuple, source: source}
}

func (source ObservationSource) Child() uint32  { return source.child }
func (source ObservationSource) Tuple() uint32  { return source.tuple }
func (source ObservationSource) Source() uint32 { return source.source }

// ObservationOutput is one exact typed output column copied into an
// observation row. The descriptor owns the output list; runtime never
// discovers or decodes output columns by domain vocabulary.
type ObservationOutput struct {
	column      model.ColumnID
	typeID      model.TypeID
	destination model.DenominatorRef
	cardinality model.Cardinality
}

// NewObservationOutput declares one output column, its sealed type, the
// mounted destination population, and the number of destination rows this
// column may contribute for one parent observation.
func NewObservationOutput(column model.ColumnID, typeID model.TypeID, destination model.DenominatorRef, cardinality model.Cardinality) ObservationOutput {
	return ObservationOutput{column: column, typeID: typeID, destination: destination, cardinality: cardinality}
}

func (output ObservationOutput) Available() bool {
	return output.column.Available() && output.typeID.Available() && output.destination.Available() && output.cardinality.Available()
}
func (output ObservationOutput) Column() model.ColumnID            { return output.column }
func (output ObservationOutput) Type() model.TypeID                { return output.typeID }
func (output ObservationOutput) Destination() model.DenominatorRef { return output.destination }
func (output ObservationOutput) Cardinality() model.Cardinality    { return output.cardinality }

// ObservationContract is the schema-owned terminal observation descriptor.
// It names the operation, one positional source selector, the exact typed
// outputs to copy, and the closed population denominator whose members make
// absence provable. It carries no runtime result, row payload, or domain
// decoder.
type ObservationContract struct {
	dependency model.DependencyID
	operation  signature.Identity
	source     ObservationSource
	outputs    []ObservationOutput
	population model.DenominatorRef
	digest     identity.ContentID
}

// NewObservationContract declares a terminal observation without performing
// mount or population admission. dependency identifies the exact owner-issued
// execution declaration. The checker proves that this declaration contains
// exactly one Apply occurrence for operation; that uniqueness is the sealed
// occurrence identity and avoids a second positional occurrence language.
// The checker/mount owns cross-reference validation; this constructor only
// freezes the caller's vectors and derives their stable declaration digest.
func NewObservationContract(dependency model.DependencyID, operation signature.Identity, source ObservationSource, population model.DenominatorRef, outputs ...ObservationOutput) ObservationContract {
	value := ObservationContract{
		dependency: dependency,
		operation:  operation,
		source:     source,
		outputs:    append([]ObservationOutput(nil), outputs...),
		population: population,
	}
	value.digest = digestObservation(value)
	return value
}

// Available reports whether the descriptor is complete enough for schema
// admission. Exact operation/column membership and positional bounds remain
// checker laws rather than hidden runtime discovery.
func (contract ObservationContract) Available() bool {
	if !contract.dependency.Available() || !contract.operation.Available() || !contract.population.Available() || !contract.digest.Available() {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(contract.outputs))
	for _, output := range contract.outputs {
		if !output.Available() {
			return false
		}
		if _, duplicate := seen[output.column]; duplicate {
			return false
		}
		seen[output.column] = struct{}{}
	}
	return true
}

// Dependency is the owner-issued execution declaration whose sealed
// expression contains the observed Apply occurrence.
func (contract ObservationContract) Dependency() model.DependencyID {
	if !contract.Available() {
		return model.DependencyID{}
	}
	return contract.dependency
}

func (contract ObservationContract) Operation() signature.Identity {
	if !contract.Available() {
		return signature.Identity{}
	}
	return contract.operation
}

func (contract ObservationContract) Source() ObservationSource {
	if !contract.Available() {
		return ObservationSource{}
	}
	return contract.source
}

func (contract ObservationContract) Population() model.DenominatorRef {
	if !contract.Available() {
		return model.DenominatorRef{}
	}
	return contract.population
}

func (contract ObservationContract) Outputs() []ObservationOutput {
	if !contract.Available() {
		return nil
	}
	return append([]ObservationOutput(nil), contract.outputs...)
}

// Digest is the content identity of the sealed descriptor. It is not used as
// an observation row key; row identity remains the owner RowID plus scope.
func (contract ObservationContract) Digest() identity.ContentID {
	if !contract.Available() {
		return identity.ContentID{}
	}
	return contract.digest
}

func digestObservation(contract ObservationContract) identity.ContentID {
	parts := make([][]byte, 0, 6+len(contract.outputs)*7)
	dependencyOwner := contract.dependency.Owner().Content()
	dependencyContent := contract.dependency.Content()
	parts = append(parts, dependencyOwner[:], dependencyContent[:])
	operationOwner := contract.operation.Operation.Owner().Content()
	operationContent := contract.operation.Operation.Content()
	parts = append(parts, operationOwner[:], operationContent[:], uint32Bytes(uint32(contract.operation.Version)), uint32Bytes(uint32(contract.operation.Version>>32)))
	parts = append(parts, uint32Bytes(contract.source.child), uint32Bytes(contract.source.tuple), uint32Bytes(contract.source.source))
	relationOwner := contract.population.Relation().Owner().Content()
	relationContent := contract.population.Relation().Content()
	keyOwner := contract.population.Key().Owner().Content()
	keyContent := contract.population.Key().Content()
	parts = append(parts, relationOwner[:], relationContent[:], keyOwner[:], keyContent[:])
	for _, output := range contract.outputs {
		columnOwner := output.column.Owner().Content()
		columnContent := output.column.Content()
		typeOwner := output.typeID.Owner().Content()
		typeContent := output.typeID.Content()
		destinationRelationOwner := output.destination.Relation().Owner().Content()
		destinationRelationContent := output.destination.Relation().Content()
		destinationKeyOwner := output.destination.Key().Owner().Content()
		destinationKeyContent := output.destination.Key().Content()
		parts = append(parts,
			columnOwner[:], columnContent[:], typeOwner[:], typeContent[:],
			destinationRelationOwner[:], destinationRelationContent[:],
			destinationKeyOwner[:], destinationKeyContent[:],
			[]byte{byte(output.cardinality.Kind())}, uint32Bytes(cardinalityBound(output.cardinality)),
		)
	}
	return derive("analysis/relation/schema/algebra/observation/v1", parts...)
}

func cardinalityBound(value model.Cardinality) uint32 {
	bound, ok := value.Bound()
	if !ok {
		return 0
	}
	return bound
}

func uint32Bytes(value uint32) []byte {
	return []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}
