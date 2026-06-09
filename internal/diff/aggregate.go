package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// aggregateArgsClause returns the argument list used inside the parentheses of
// CREATE/DROP/COMMENT AGGREGATE statements. Zero-argument aggregates (e.g. a
// custom count(*)) use "*".
func aggregateArgsClause(aggregate *ir.Aggregate) string {
	if aggregate.Arguments == "" {
		return "*"
	}
	return aggregate.Arguments
}

// generateCreateAggregatesSQL generates CREATE AGGREGATE statements
func generateCreateAggregatesSQL(aggregates []*ir.Aggregate, targetSchema string, collector *diffCollector) {
	// Sort aggregates by name for consistent ordering
	sortedAggregates := make([]*ir.Aggregate, len(aggregates))
	copy(sortedAggregates, aggregates)
	sort.Slice(sortedAggregates, func(i, j int) bool {
		return sortedAggregates[i].Name < sortedAggregates[j].Name
	})

	for _, aggregate := range sortedAggregates {
		sql := generateAggregateSQL(aggregate, targetSchema)

		context := &diffContext{
			Type:                DiffTypeAggregate,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s", aggregate.Schema, aggregate.Name),
			Source:              aggregate,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)

		// Generate COMMENT ON AGGREGATE if the aggregate has a comment
		if aggregate.Comment != "" {
			generateAggregateComment(aggregate, targetSchema, DiffOperationCreate, collector)
		}
	}
}

// generateModifyAggregatesSQL generates DROP and CREATE AGGREGATE statements for modified aggregates.
// PostgreSQL has no ALTER AGGREGATE for the defining properties (SFUNC, STYPE, etc.),
// so any substantive change is expressed as DROP + CREATE.
func generateModifyAggregatesSQL(diffs []*aggregateDiff, targetSchema string, collector *diffCollector) {
	for _, diff := range diffs {
		oldAgg := diff.Old
		newAgg := diff.New

		// Check if only the comment changed (no definitional change)
		if aggregatesEqualExceptComment(oldAgg, newAgg) && oldAgg.Comment != newAgg.Comment {
			generateAggregateComment(newAgg, targetSchema, DiffOperationAlter, collector)
			continue
		}

		dropSQL := generateAggregateDropSQL(oldAgg, targetSchema)
		createSQL := generateAggregateSQL(newAgg, targetSchema)

		alterContext := &diffContext{
			Type:                DiffTypeAggregate,
			Operation:           DiffOperationAlter,
			Path:                fmt.Sprintf("%s.%s", newAgg.Schema, newAgg.Name),
			Source:              diff,
			CanRunInTransaction: true,
		}

		statements := []SQLStatement{
			{SQL: dropSQL, CanRunInTransaction: true},
			{SQL: createSQL, CanRunInTransaction: true},
		}

		collector.collectStatements(alterContext, statements)

		// Re-apply the comment after recreation if one is set
		if newAgg.Comment != "" {
			generateAggregateComment(newAgg, targetSchema, DiffOperationAlter, collector)
		}
	}
}

// generateDropAggregatesSQL generates DROP AGGREGATE statements
func generateDropAggregatesSQL(aggregates []*ir.Aggregate, targetSchema string, collector *diffCollector) {
	// Sort aggregates by name for consistent ordering
	sortedAggregates := make([]*ir.Aggregate, len(aggregates))
	copy(sortedAggregates, aggregates)
	sort.Slice(sortedAggregates, func(i, j int) bool {
		return sortedAggregates[i].Name < sortedAggregates[j].Name
	})

	for _, aggregate := range sortedAggregates {
		sql := generateAggregateDropSQL(aggregate, targetSchema)

		context := &diffContext{
			Type:                DiffTypeAggregate,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s", aggregate.Schema, aggregate.Name),
			Source:              aggregate,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateAggregateDropSQL builds a DROP AGGREGATE statement
func generateAggregateDropSQL(aggregate *ir.Aggregate, targetSchema string) string {
	aggregateName := qualifyEntityName(aggregate.Schema, aggregate.Name, targetSchema)
	return fmt.Sprintf("DROP AGGREGATE IF EXISTS %s(%s);", aggregateName, aggregateArgsClause(aggregate))
}

// generateAggregateSQL generates a CREATE AGGREGATE statement
func generateAggregateSQL(aggregate *ir.Aggregate, targetSchema string) string {
	var stmt strings.Builder

	aggregateName := qualifyEntityName(aggregate.Schema, aggregate.Name, targetSchema)
	stmt.WriteString(fmt.Sprintf("CREATE AGGREGATE %s(%s) (\n", aggregateName, aggregateArgsClause(aggregate)))

	var parts []string

	// SFUNC - the state transition function (qualified relative to the target schema)
	sfunc := qualifyEntityName(aggregate.TransitionFunctionSchema, aggregate.TransitionFunction, targetSchema)
	parts = append(parts, fmt.Sprintf("    SFUNC = %s", sfunc))

	// STYPE - the state value type
	parts = append(parts, fmt.Sprintf("    STYPE = %s", stripSchemaPrefix(aggregate.StateType, targetSchema)))

	// FINALFUNC - the optional final function
	if aggregate.FinalFunction != "" {
		ffunc := qualifyEntityName(aggregate.FinalFunctionSchema, aggregate.FinalFunction, targetSchema)
		parts = append(parts, fmt.Sprintf("    FINALFUNC = %s", ffunc))
	}

	// INITCOND - the optional initial condition
	if aggregate.InitialCondition != "" {
		parts = append(parts, fmt.Sprintf("    INITCOND = %s", quoteString(aggregate.InitialCondition)))
	}

	stmt.WriteString(strings.Join(parts, ",\n"))
	stmt.WriteString("\n);")

	return stmt.String()
}

// generateAggregateComment generates a COMMENT ON AGGREGATE statement
func generateAggregateComment(aggregate *ir.Aggregate, targetSchema string, operation DiffOperation, collector *diffCollector) {
	aggregateName := qualifyEntityName(aggregate.Schema, aggregate.Name, targetSchema)
	argsClause := aggregateArgsClause(aggregate)

	var sql string
	if aggregate.Comment == "" {
		sql = fmt.Sprintf("COMMENT ON AGGREGATE %s(%s) IS NULL;", aggregateName, argsClause)
	} else {
		sql = fmt.Sprintf("COMMENT ON AGGREGATE %s(%s) IS %s;", aggregateName, argsClause, quoteString(aggregate.Comment))
	}

	context := &diffContext{
		Type:                DiffTypeAggregate,
		Operation:           operation,
		Path:                fmt.Sprintf("%s.%s", aggregate.Schema, aggregate.Name),
		Source:              aggregate,
		CanRunInTransaction: true,
	}
	collector.collect(context, sql)
}

// aggregatesEqual compares two aggregates for equality
func aggregatesEqual(old, new *ir.Aggregate) bool {
	return aggregatesEqualExceptComment(old, new) && old.Comment == new.Comment
}

// aggregatesEqualExceptComment compares two aggregates ignoring comment differences
func aggregatesEqualExceptComment(old, new *ir.Aggregate) bool {
	return old.Schema == new.Schema &&
		old.Name == new.Name &&
		old.Arguments == new.Arguments &&
		old.ReturnType == new.ReturnType &&
		old.TransitionFunction == new.TransitionFunction &&
		old.TransitionFunctionSchema == new.TransitionFunctionSchema &&
		old.StateType == new.StateType &&
		old.InitialCondition == new.InitialCondition &&
		old.FinalFunction == new.FinalFunction &&
		old.FinalFunctionSchema == new.FinalFunctionSchema
}
