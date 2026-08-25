package relcompile

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Compile lowers resolved declaration data into one immutable logical
// ExecutionSchema. It deliberately does not perform the independent
// certificate checks; malformed references are rejected here only when they
// cannot be represented as a closed expression, while cross-entry authority
// belongs to relation/check.
func Compile(declaration Declaration) (plan.ExecutionSchema, error) {
	if !declaration.SchemaID.Available() {
		return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable schema identity")
	}

	relations := make(map[model.RelationID]struct{}, len(declaration.Relations))
	for _, relation := range declaration.Relations {
		if !relation.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable relation schema")
		}
		if _, duplicate := relations[relation.ID()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate relation")
		}
		relations[relation.ID()] = struct{}{}
	}
	columns := make(map[model.ColumnID]struct{}, len(declaration.Columns))
	for _, column := range declaration.Columns {
		if !column.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable column schema")
		}
		if _, duplicate := columns[column.ID()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate column")
		}
		columns[column.ID()] = struct{}{}
	}
	keys := make(map[model.KeyID]struct{}, len(declaration.Keys))
	for _, key := range declaration.Keys {
		if !key.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable key schema")
		}
		if _, duplicate := keys[key.ID()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate key")
		}
		keys[key.ID()] = struct{}{}
	}
	scopes := make(map[model.ScopeID]struct{}, len(declaration.Scopes))
	for _, scope := range declaration.Scopes {
		if !scope.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable scope schema")
		}
		if _, duplicate := scopes[scope.ID()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate scope")
		}
		scopes[scope.ID()] = struct{}{}
	}
	signatures := make(map[signature.Identity]struct{}, len(declaration.Signatures))

	builder := plan.NewBuilder(declaration.SchemaID)
	for _, relation := range declaration.Relations {
		if !builder.AddRelation(relation) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add relation")
		}
	}
	for _, column := range declaration.Columns {
		if !builder.AddColumn(column) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add column")
		}
	}
	for _, key := range declaration.Keys {
		if !builder.AddKey(key) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add key")
		}
	}
	for _, scope := range declaration.Scopes {
		if !builder.AddScope(scope) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add scope")
		}
	}
	for _, semantic := range declaration.Signatures {
		if !semantic.Available() {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: unavailable semantic signature")
		}
		if _, duplicate := signatures[semantic.Identity()]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate semantic signature")
		}
		signatures[semantic.Identity()] = struct{}{}
		if !builder.AddSignature(semantic) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: add semantic signature")
		}
	}
	expressions := make(map[model.ExpressionID]struct{}, len(declaration.Rules))
	dependencies := make(map[model.DependencyID]struct{}, len(declaration.Rules))

	for _, rule := range declaration.Rules {
		expression, reads, writes, err := lowerRule(rule, relations, columns, keys, scopes, signatures)
		if err != nil {
			return plan.ExecutionSchema{}, err
		}
		if _, duplicate := expressions[rule.Expression]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate expression identity")
		}
		expressions[rule.Expression] = struct{}{}
		if _, duplicate := dependencies[rule.ID]; duplicate {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: duplicate dependency identity")
		}
		dependencies[rule.ID] = struct{}{}
		entry := plan.DefineExpressionRef(rule.Expression, expression)
		if !entry.Available() || !builder.AddExpression(entry) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: invalid expression entry")
		}
		dependency := plan.DefineDependency(
			rule.ID,
			rule.Expression,
			relationRefs(reads),
			relationRefs(writes),
			"",
		)
		if !dependency.Available() || !builder.AddDependency(dependency) {
			return plan.ExecutionSchema{}, fmt.Errorf("relcompile: invalid dependency")
		}
	}

	compiled, ok := builder.Build()
	if !ok {
		return plan.ExecutionSchema{}, fmt.Errorf("relcompile: build execution schema")
	}
	return compiled, nil
}

func lowerRule(rule Rule, relations map[model.RelationID]struct{}, columns map[model.ColumnID]struct{}, keys map[model.KeyID]struct{}, scopes map[model.ScopeID]struct{}, signatures map[signature.Identity]struct{}) (algebra.Expression, []model.RelationID, []model.RelationID, error) {
	if !rule.ID.Available() || !rule.Expression.Available() || !rule.Candidate.Available() {
		return nil, nil, nil, fmt.Errorf("relcompile: incomplete rule identity")
	}
	if !containsRelation(relations, rule.Candidate) {
		return nil, nil, nil, fmt.Errorf("relcompile: candidate relation is not declared")
	}
	if rule.Scope.Available() {
		if _, ok := scopes[rule.Scope]; !ok {
			return nil, nil, nil, fmt.Errorf("relcompile: scope is not declared")
		}
	}
	if rule.Complete != nil && !rule.Complete.Available() {
		return nil, nil, nil, fmt.Errorf("relcompile: malformed completion denominator")
	}
	if rule.Complete != nil {
		if !containsRelation(relations, rule.Complete.Relation()) {
			return nil, nil, nil, fmt.Errorf("relcompile: completion relation is not declared")
		}
		if _, ok := keys[rule.Complete.Key()]; !ok || rule.Complete.Key().Relation() != rule.Complete.Relation() {
			return nil, nil, nil, fmt.Errorf("relcompile: completion key is not declared by denominator relation")
		}
	}
	if rule.Publish != nil {
		if !containsRelation(relations, rule.Publish.Relation) {
			return nil, nil, nil, fmt.Errorf("relcompile: publication relation is not declared")
		}
		if _, ok := keys[rule.Publish.Key]; !ok || rule.Publish.Key.Relation() != rule.Publish.Relation {
			return nil, nil, nil, fmt.Errorf("relcompile: publication key is not declared by destination")
		}
	}

	expression := algebra.Expression(algebra.NewInput(rule.Candidate))
	reads := []model.RelationID{rule.Candidate}
	for index, join := range rule.Joins {
		if !containsRelation(relations, join.Relation) {
			return nil, nil, nil, fmt.Errorf("relcompile: join %d relation is not declared", index)
		}
		if !validColumns(join.LeftColumns, columns) || !validColumns(join.RightColumns, columns) || len(join.LeftColumns) == 0 || len(join.LeftColumns) != len(join.RightColumns) {
			return nil, nil, nil, fmt.Errorf("relcompile: join %d has incompatible typed columns", index)
		}
		for _, column := range join.RightColumns {
			if column.Relation() != join.Relation {
				return nil, nil, nil, fmt.Errorf("relcompile: join %d right column is not owned by joined relation", index)
			}
		}
		joined := algebra.Expression(algebra.NewInput(join.Relation))
		if join.Scope.Available() {
			if _, ok := scopes[join.Scope]; !ok {
				return nil, nil, nil, fmt.Errorf("relcompile: join %d scope is not declared", index)
			}
			joined = algebra.NewSelect(joined, algebra.NewSelectContract(algebra.SelectByScope, join.Scope))
		}
		if join.Complete != nil {
			if !join.Complete.Available() {
				return nil, nil, nil, fmt.Errorf("relcompile: join %d has a malformed completion denominator", index)
			}
			if !containsRelation(relations, join.Complete.Relation()) {
				return nil, nil, nil, fmt.Errorf("relcompile: join %d completion relation is not declared", index)
			}
			if _, ok := keys[join.Complete.Key()]; !ok {
				return nil, nil, nil, fmt.Errorf("relcompile: join %d completion key is not declared by denominator relation", index)
			}
			joined = algebra.NewComplete(joined, *join.Complete)
			reads = appendUniqueRelation(reads, join.Complete.Relation())
		}
		contract := algebra.NewJoinContract(join.LeftColumns, join.RightColumns)
		expression = algebra.NewJoin(expression, joined, contract)
		reads = appendUniqueRelation(reads, join.Relation)
	}
	if rule.Scope.Available() {
		expression = algebra.NewSelect(expression, algebra.NewSelectContract(algebra.SelectByScope, rule.Scope))
	}
	if rule.Complete != nil {
		expression = algebra.NewComplete(expression, *rule.Complete)
	}
	if rule.Apply.Operation.Available() || rule.Apply.Version != 0 {
		if !rule.Apply.Available() {
			return nil, nil, nil, fmt.Errorf("relcompile: malformed semantic operation identity")
		}
		if _, ok := signatures[rule.Apply]; !ok {
			return nil, nil, nil, fmt.Errorf("relcompile: semantic operation is not declared")
		}
		expression = algebra.NewApply([]algebra.Expression{expression}, algebra.NewApplyContract(rule.Apply))
	}
	if rule.Carry != nil {
		if rule.Publish == nil {
			return nil, nil, nil, fmt.Errorf("relcompile: a carried derivation has no publication to merge under")
		}
		if !containsRelation(relations, rule.Carry.Relation) {
			return nil, nil, nil, fmt.Errorf("relcompile: carried relation is not declared")
		}
		carried := algebra.Expression(algebra.NewInput(rule.Carry.Relation))
		if rule.Carry.Scope.Available() {
			if _, ok := scopes[rule.Carry.Scope]; !ok {
				return nil, nil, nil, fmt.Errorf("relcompile: carried scope is not declared")
			}
			carried = algebra.NewSelect(carried, algebra.NewSelectContract(algebra.SelectByScope, rule.Carry.Scope))
		}
		if rule.Carry.Transform != nil {
			if _, ok := signatures[*rule.Carry.Transform]; !ok {
				return nil, nil, nil, fmt.Errorf("relcompile: carry transform operation is not declared")
			}
			carried = algebra.NewApply([]algebra.Expression{carried}, algebra.NewApplyContract(*rule.Carry.Transform))
		}
		expression = algebra.NewMerge([]algebra.Expression{expression, carried}, algebra.NewMergeContract(rule.Publish.Key))
		reads = appendUniqueRelation(reads, rule.Carry.Relation)
	}
	if rule.Publish != nil {
		expression = algebra.NewPublish(expression, algebra.NewPublishContract(rule.Publish.Relation, rule.Publish.Key))
		writes := []model.RelationID{rule.Publish.Relation}
		return expression, reads, writes, nil
	}
	return expression, reads, nil, nil
}

func validColumns(values []model.ColumnID, columns map[model.ColumnID]struct{}) bool {
	for _, column := range values {
		if _, ok := columns[column]; !ok {
			return false
		}
	}
	return true
}

func containsRelation(relations map[model.RelationID]struct{}, relation model.RelationID) bool {
	_, ok := relations[relation]
	return ok
}

func appendUniqueRelation(values []model.RelationID, relation model.RelationID) []model.RelationID {
	for _, existing := range values {
		if existing == relation {
			return values
		}
	}
	return append(values, relation)
}

func relationRefs(values []model.RelationID) []plan.RelationRef {
	refs := make([]plan.RelationRef, 0, len(values))
	for _, relation := range values {
		ref, ok := plan.NewRelationRef(relation)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs
}
