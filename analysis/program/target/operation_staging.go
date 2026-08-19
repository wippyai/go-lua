package target

// discardOperationQueryStaging releases the root construction columns after
// their immutable value handoff to operation.Core. The published Contract
// keeps operation relations only where a remaining Target vertical still
// owns them; operation declarations, Values, types, behavior, and transfer
// query data are read from Core.
func (c *Contract) discardOperationQueryStaging() {
	c.types = nil
	c.values = nil
	c.valueTypes = nil
	c.behaviorResults = nil
	c.behaviorPredicates = nil
	c.formals = nil
	c.valuesVarTypes = nil
	c.transfers = nil
	c.transferOutcomes = nil
}
