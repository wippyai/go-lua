package target

import (
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// operationQueryInput is the one-way value handoff from Target's staging
// columns to the immutable operation owner. It deliberately copies scalar
// rows and ranges into operation-owned values; no Contract or draft callback
// crosses this boundary.
func operationQueryInput(c *Contract) operationvalue.QueryInput {
	input := operationvalue.QueryInput{
		Types:      make([]operationvalue.TypeInput, len(c.types)),
		Values:     make([]operationvalue.ValuesInput, len(c.values)),
		Effects:    make([]operationvalue.EffectInput, len(c.effects)),
		Operations: make([]operationvalue.QueryOperationInput, len(c.operations)),
	}
	for index, row := range c.types {
		input.Types[index] = operationvalue.TypeInput{
			Handle: vocabulary.Type(index + 1), Declaration: row.declaration,
		}
	}
	for index, row := range c.values {
		values := operationvalue.ValuesInput{
			Handle: vocabulary.Values(index + 1), Owner: row.owner, Tail: row.tail, VarID: row.varID,
		}
		for position := 0; position < row.types.len(); position++ {
			values.Types = append(values.Types, c.valueTypes[row.types.start+uint32(position)])
		}
		for position := 0; position < row.suffix.len(); position++ {
			values.Suffix = append(values.Suffix, c.valueTypes[row.suffix.start+uint32(position)])
		}
		input.Values[index] = values
	}
	for index, row := range c.effects {
		item := operationvalue.EffectInput{
			Target: row.target, HasPublication: row.hasPublication,
			Values:    append([]vocabulary.ValueFormal(nil), c.effectVals[row.values.start:row.values.end]...),
			Types:     append([]vocabulary.TypeFormal(nil), c.effectType[row.types.start:row.types.end]...),
			ValuesVar: append([]vocabulary.ValuesVar(nil), c.effectVars[row.valuesVar.start:row.valuesVar.end]...),
			Rows:      append([]vocabulary.RowVar(nil), c.effectRows[row.rows.start:row.rows.end]...),
		}
		if row.hasPublication {
			item.Publication = vocabulary.PublicationEffectSpec{
				Kind: row.publication.Kind(), Subject: row.publication.Subject(),
				Destination: row.publication.DestinationRole(), Context: row.publication.Context(),
				Escape: row.publication.Escape(), Mutability: row.publication.Mutability(),
				Lifetime: row.publication.Lifetime(),
			}
		}
		input.Effects[index] = item
	}
	for index, row := range c.operations {
		item := operationvalue.QueryOperationInput{
			Input:       row.input,
			ValuesTypes: append([]vocabulary.Type(nil), c.valuesVarTypes[row.valuesTypes.start:row.valuesTypes.end]...),
			RowFormals:  row.rowFormals, EffectTail: row.effectTail, EffectVar: row.effectVar,
			EffectIndices: make([]int, row.effects.len()),
			TypeFormals:   append([]vocabulary.Type(nil), c.formals[row.typeFormals.start:row.typeFormals.end]...),
		}
		for effect := range item.EffectIndices {
			item.EffectIndices[effect] = int(row.effects.start) + effect
		}
		for outcome := 0; outcome < row.outcomes.len(); outcome++ {
			value := c.outcomes[row.outcomes.start+uint32(outcome)]
			item.Outcomes = append(item.Outcomes, operationvalue.QueryOutcomeInput{Kind: value.kind, Values: value.values})
		}
		for behavior := 0; behavior < row.behavior.len(); behavior++ {
			value := c.behaviorResults[row.behavior.start+uint32(behavior)]
			item.Behavior = append(item.Behavior, operationvalue.BehaviorResultInput{
				Outcome: value.outcome, Result: value.result, Source: value.source, Relation: value.relation,
			})
		}
		for predicate := 0; predicate < row.behaviorPredicates.len(); predicate++ {
			value := c.behaviorPredicates[row.behaviorPredicates.start+uint32(predicate)]
			item.BehaviorPredicates = append(item.BehaviorPredicates, operationvalue.BehaviorPredicateInput{
				Outcome: value.outcome, Result: value.result, Subject: value.subject, Relation: value.relation,
			})
		}
		for transfer := 0; transfer < row.transfers.len(); transfer++ {
			value := c.transfers[row.transfers.start+uint32(transfer)]
			transferInput := operationvalue.TransferInput{
				Endpoint: value.endpoint, Payload: value.payload, Alias: value.alias,
				Identity: value.identity, Capabilities: value.capabilities,
				Outcomes: append([]vocabulary.TransferPossibility(nil), c.transferOutcomes[value.outcomes.start:value.outcomes.end]...),
			}
			item.Transfers = append(item.Transfers, transferInput)
		}
		input.Operations[index] = item
	}
	return input
}
