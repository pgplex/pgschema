---
name: postgres_syntax
description: Consult PostgreSQL's parser and grammar (gram.y) to understand SQL syntax, DDL statement structure, and parsing rules when implementing pgschema features. Use this skill when generating DDL in internal/diff/*.go, validating SQL syntax, understanding keyword precedence, or learning how PostgreSQL handles specific constructs like triggers, indexes, generated columns, or constraint triggers.
---

# PostgreSQL Syntax Reference

Reference PostgreSQL's grammar to understand SQL syntax and generate correct DDL.

## Source Files

**Local copies** (preferred):
- `internal/gram.y` - Yacc/Bison grammar defining all PostgreSQL SQL syntax
- `internal/scan.l` - Flex lexer for tokenization

**Searching the grammar**:
```bash
grep -n "CreateTrigStmt:" internal/gram.y     # Find statement rule
grep -A 10 "TriggerWhen:" internal/gram.y     # Understand an option
```

## Statement Types → Grammar Rules

| Statement | Grammar Rule | Key Sub-rules |
|-----------|-------------|---------------|
| CREATE TABLE | `CreateStmt` | `columnDef`, `TableConstraint`, `TableLikeClause` |
| ALTER TABLE | `AlterTableStmt` | `alter_table_cmd` |
| CREATE INDEX | `IndexStmt` | `index_elem` (column, function, expression) |
| CREATE TRIGGER | `CreateTrigStmt` | `TriggerActionTime`, `TriggerEvents`, `TriggerWhen` |
| CREATE FUNCTION | `CreateFunctionStmt` | `func_args`, `createfunc_opt_list` |
| CREATE VIEW | `ViewStmt` | `SelectStmt` |
| CREATE SEQUENCE | `CreateSeqStmt` | `OptSeqOptList` |
| CREATE TYPE | `CreateEnumStmt`, `CompositeTypeStmt`, `CreateDomainStmt` | |
| CREATE POLICY | `CreatePolicyStmt` | `row_security_cmd` |

## Grammar Syntax Guide

gram.y uses Yacc/Bison notation:
- **UPPERCASE**: Terminal tokens (keywords like `CREATE`, `TRIGGER`)
- **lowercase**: Non-terminal rules (references to other grammar rules)
- **`|`**: Alternative syntax options
- **`opt_*`**: Optional elements (can be empty)
- **`*_list`**: Recursive list constructs

Example:
```yacc
CreateTrigStmt:
    CREATE opt_or_replace TRIGGER name TriggerActionTime TriggerEvents ON
    qualified_name TriggerReferencing TriggerForSpec TriggerWhen
    EXECUTE FUNCTION_or_PROCEDURE func_name '(' TriggerFuncArgs ')'
```

## Key Constructs for pgschema

### Column Definitions
- Regular: `column_name type [constraints]`
- Generated: `column_name type GENERATED ALWAYS AS (expr) STORED`
- Identity: `column_name type GENERATED {ALWAYS|BY DEFAULT} AS IDENTITY`

### Index Elements
Three forms — note extra parens for arbitrary expressions:
1. Column: `CREATE INDEX idx ON t (col)`
2. Function: `CREATE INDEX idx ON t (lower(col))`
3. Expression: `CREATE INDEX idx ON t ((col + 1))`

### Trigger WHEN Clause
```yacc
TriggerWhen:
    WHEN '(' a_expr ')'
    | /* EMPTY */
```

### Constraint Triggers
```yacc
CREATE opt_or_replace CONSTRAINT TRIGGER name ...
    -- Can be DEFERRABLE / NOT DEFERRABLE
    -- Can be INITIALLY DEFERRED / INITIALLY IMMEDIATE
```

### Table LIKE Clause
```yacc
LIKE qualified_name [INCLUDING|EXCLUDING] {COMMENTS|CONSTRAINTS|DEFAULTS|IDENTITY|GENERATED|INDEXES|STATISTICS|STORAGE|ALL}
```

## Operator Precedence (from gram.y top)

```
%left OR
%left AND
%right NOT
%nonassoc IS ISNULL NOTNULL
%nonassoc '<' '>' '=' LESS_EQUALS GREATER_EQUALS NOT_EQUALS
```

## Keywords

- **Reserved**: Cannot be identifiers without quoting (`SELECT`, `TABLE`, `CREATE`)
- **Unreserved**: Can be used as identifiers (`ABORT`, `ACCESS`, `ACTION`)

When generating DDL, quote identifiers that match reserved keywords.

## Version Differences (14-18)

- PG 14: `COMPRESSION` clause for tables
- PG 15: `UNIQUE NULLS NOT DISTINCT`
- PG 16: SQL/JSON functions
- PG 17: `MERGE` enhancements

Check gram.y git history to see when features were added. Add version detection in pgschema if needed.

## Applying to pgschema

When generating DDL in `internal/diff/*.go`:
- Follow gram.y syntax exactly for keyword ordering
- Include all required elements
- Quote identifiers correctly via `ir/quote.go`
- Test generated DDL against real PostgreSQL via integration tests
