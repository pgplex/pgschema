package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// aggregateSortKey produces a fully-qualified, overload-aware key so that aggregates
// sharing a name (overloads, or the same name across schemas) order deterministically.
func aggregateSortKey(a *ir.Aggregate) string {
	return a.Schema + "." + a.Name + "(" + a.Arguments + ")"
}

// aggregateArgs returns the argument list used inside the parentheses of a
// CREATE/DROP/COMMENT AGGREGATE statement. A zero-argument aggregate (e.g. a
// custom count(*)) has an empty list and is rendered as "*". Ordered-set and
// hypothetical-set aggregates already carry their "... ORDER BY ..." form.
func aggregateArgs(args string) string {
	if args == "" {
		return "*"
	}
	return args
}

// finalFuncModifyKeyword maps the catalog code ('r'/'s'/'w') to the CREATE
// AGGREGATE keyword. READ_ONLY ('r') is the default and is never emitted.
func finalFuncModifyKeyword(code string) string {
	switch code {
	case "s":
		return "SHAREABLE"
	case "w":
		return "READ_WRITE"
	default:
		return ""
	}
}

// parallelKeyword maps pg_proc.proparallel ('s'/'r'/'u') to the CREATE AGGREGATE
// keyword. UNSAFE ('u') is the default and is never emitted.
func parallelKeyword(code string) string {
	switch code {
	case "s":
		return "SAFE"
	case "r":
		return "RESTRICTED"
	default:
		return ""
	}
}

// generateCreateAggregatesSQL generates CREATE AGGREGATE statements
func generateCreateAggregatesSQL(aggregates []*ir.Aggregate, targetSchema string, collector *diffCollector) {
	// Sort aggregates by name for consistent ordering
	sortedAggregates := make([]*ir.Aggregate, len(aggregates))
	copy(sortedAggregates, aggregates)
	sort.Slice(sortedAggregates, func(i, j int) bool {
		return aggregateSortKey(sortedAggregates[i]) < aggregateSortKey(sortedAggregates[j])
	})

	for _, aggregate := range sortedAggregates {
		sql := generateAggregateSQL(aggregate, targetSchema, collector.qualifySchema)

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
		createSQL := generateAggregateSQL(newAgg, targetSchema, collector.qualifySchema)

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
		return aggregateSortKey(sortedAggregates[i]) < aggregateSortKey(sortedAggregates[j])
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
	return fmt.Sprintf("DROP AGGREGATE IF EXISTS %s(%s);", aggregateName, aggregateArgs(aggregate.Arguments))
}

// generateAggregateSQL generates a CREATE AGGREGATE statement, emitting options in the
// same order as pg_dump and only when they differ from their defaults. Support-function
// references are stored pre-qualified relative to the aggregate's schema, so they are
// emitted verbatim.
func generateAggregateSQL(aggregate *ir.Aggregate, targetSchema string, qualifySchema bool) string {
	var stmt strings.Builder

	aggregateName := qualifyEntityNameMode(aggregate.Schema, aggregate.Name, targetSchema, qualifySchema)
	stmt.WriteString(fmt.Sprintf("CREATE AGGREGATE %s(%s) (\n", aggregateName, aggregateArgs(aggregate.Signature)))

	var parts []string
	add := func(format string, args ...interface{}) {
		parts = append(parts, "    "+fmt.Sprintf(format, args...))
	}

	// SFUNC / STYPE are always present.
	add("SFUNC = %s", aggregate.TransitionFunction)
	add("STYPE = %s", stripSchemaPrefixMode(aggregate.StateType, targetSchema, qualifySchema))
	if aggregate.StateSpace != 0 {
		add("SSPACE = %d", aggregate.StateSpace)
	}

	// Final function group.
	if aggregate.FinalFunction != "" {
		add("FINALFUNC = %s", aggregate.FinalFunction)
	}
	if aggregate.FinalFuncExtra {
		add("FINALFUNC_EXTRA")
	}
	if kw := finalFuncModifyKeyword(aggregate.FinalFuncModify); kw != "" {
		add("FINALFUNC_MODIFY = %s", kw)
	}

	// Parallel-aggregation support functions.
	if aggregate.CombineFunction != "" {
		add("COMBINEFUNC = %s", aggregate.CombineFunction)
	}
	if aggregate.SerialFunction != "" {
		add("SERIALFUNC = %s", aggregate.SerialFunction)
	}
	if aggregate.DeserialFunction != "" {
		add("DESERIALFUNC = %s", aggregate.DeserialFunction)
	}

	if aggregate.InitialCondition != nil {
		add("INITCOND = %s", quoteString(*aggregate.InitialCondition))
	}

	// Moving-aggregate group (gated on a moving transition function).
	if aggregate.MTransitionFunction != "" {
		add("MSFUNC = %s", aggregate.MTransitionFunction)
		if aggregate.MInvTransitionFunction != "" {
			add("MINVFUNC = %s", aggregate.MInvTransitionFunction)
		}
		if aggregate.MStateType != "" {
			add("MSTYPE = %s", stripSchemaPrefixMode(aggregate.MStateType, targetSchema, qualifySchema))
		}
		if aggregate.MStateSpace != 0 {
			add("MSSPACE = %d", aggregate.MStateSpace)
		}
	}
	if aggregate.MFinalFunction != "" {
		add("MFINALFUNC = %s", aggregate.MFinalFunction)
	}
	if aggregate.MFinalFuncExtra {
		add("MFINALFUNC_EXTRA")
	}
	if kw := finalFuncModifyKeyword(aggregate.MFinalFuncModify); kw != "" {
		add("MFINALFUNC_MODIFY = %s", kw)
	}
	if aggregate.MInitialCondition != nil {
		add("MINITCOND = %s", quoteString(*aggregate.MInitialCondition))
	}

	// SORTOP is only emitted for normal aggregates.
	if aggregate.SortOperator != "" && aggregate.Kind == "n" {
		add("SORTOP = %s", aggregate.SortOperator)
	}

	if kw := parallelKeyword(aggregate.Parallel); kw != "" {
		add("PARALLEL = %s", kw)
	}

	// HYPOTHETICAL flag for hypothetical-set aggregates.
	if aggregate.Kind == "h" {
		add("HYPOTHETICAL")
	}

	stmt.WriteString(strings.Join(parts, ",\n"))
	stmt.WriteString("\n);")

	return stmt.String()
}

// generateAggregateComment generates a COMMENT ON AGGREGATE statement
func generateAggregateComment(aggregate *ir.Aggregate, targetSchema string, operation DiffOperation, collector *diffCollector) {
	aggregateName := qualifyEntityNameMode(aggregate.Schema, aggregate.Name, targetSchema, collector.qualifySchema)
	argsClause := aggregateArgs(aggregate.Arguments)

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
		old.Signature == new.Signature &&
		old.Kind == new.Kind &&
		old.ReturnType == new.ReturnType &&
		old.Parallel == new.Parallel &&
		old.TransitionFunction == new.TransitionFunction &&
		old.StateType == new.StateType &&
		old.StateSpace == new.StateSpace &&
		stringPtrEqual(old.InitialCondition, new.InitialCondition) &&
		old.FinalFunction == new.FinalFunction &&
		old.FinalFuncExtra == new.FinalFuncExtra &&
		old.FinalFuncModify == new.FinalFuncModify &&
		old.CombineFunction == new.CombineFunction &&
		old.SerialFunction == new.SerialFunction &&
		old.DeserialFunction == new.DeserialFunction &&
		old.MTransitionFunction == new.MTransitionFunction &&
		old.MInvTransitionFunction == new.MInvTransitionFunction &&
		old.MStateType == new.MStateType &&
		old.MStateSpace == new.MStateSpace &&
		old.MFinalFunction == new.MFinalFunction &&
		old.MFinalFuncExtra == new.MFinalFuncExtra &&
		old.MFinalFuncModify == new.MFinalFuncModify &&
		stringPtrEqual(old.MInitialCondition, new.MInitialCondition) &&
		old.SortOperator == new.SortOperator
}

// stringPtrEqual compares two optional strings, distinguishing nil (NULL) from a
// pointer to the empty string.
func stringPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
