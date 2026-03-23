package ir

import (
	"sort"
	"strings"
	"sync"
)

// IR represents the complete database schema intermediate representation
type IR struct {
	Metadata Metadata           `json:"metadata"`
	Schemas  map[string]*Schema `json:"schemas"` // schema_name -> Schema
	mu       sync.RWMutex       // Protects concurrent access to Schemas
}

// Metadata contains information about the schema dump
type Metadata struct {
	DatabaseVersion string `json:"database_version"`
}

// Schema represents a single database schema (namespace)
type Schema struct {
	Name  string `json:"name"`
	Owner string `json:"owner"` // Schema owner
	// Note: Indexes, Triggers, and RLS Policies are stored at table level (Table.Indexes, Table.Triggers, Table.Policies)
	Tables                   map[string]*Table        `json:"tables"`                            // table_name -> Table
	Views                    map[string]*View         `json:"views"`                             // view_name -> View
	Functions                map[string]*Function     `json:"functions"`                         // function_name -> Function
	Procedures               map[string]*Procedure    `json:"procedures"`                        // procedure_name -> Procedure
	Aggregates               map[string]*Aggregate    `json:"aggregates"`                        // aggregate_name -> Aggregate
	Sequences                map[string]*Sequence     `json:"sequences"`                         // sequence_name -> Sequence
	Types                    map[string]*Type         `json:"types"`                             // type_name -> Type
	DefaultPrivileges        []*DefaultPrivilege        `json:"default_privileges,omitempty"`        // Default privileges for future objects
	Privileges               []*Privilege               `json:"privileges,omitempty"`                // Explicit privilege grants on objects
	ColumnPrivileges         []*ColumnPrivilege         `json:"column_privileges,omitempty"`         // Column-level privilege grants
	RevokedDefaultPrivileges []*RevokedDefaultPrivilege `json:"revoked_default_privileges,omitempty"` // Explicit revokes of default PUBLIC privileges
	mu                       sync.RWMutex             // Protects concurrent access to all maps
}

// LikeClause represents a LIKE clause in CREATE TABLE statement
type LikeClause struct {
	SourceSchema string `json:"source_schema"`
	SourceTable  string `json:"source_table"`
	Options      string `json:"options"` // e.g., "INCLUDING ALL" or "INCLUDING DEFAULTS EXCLUDING INDEXES"
}

// Table represents a database table
type Table struct {
	Schema            string                 `json:"schema"`
	Name              string                 `json:"name"`
	Type              TableType              `json:"type"` // BASE_TABLE, VIEW, etc.
	IsExternal        bool                   `json:"is_external,omitempty"` // True if table is externally managed (e.g., in ignored schemas)
	Columns           []*Column              `json:"columns"`
	Constraints       map[string]*Constraint `json:"constraints"` // constraint_name -> Constraint
	Indexes           map[string]*Index      `json:"indexes"`     // index_name -> Index
	Triggers          map[string]*Trigger    `json:"triggers"`    // trigger_name -> Trigger
	RLSEnabled        bool                   `json:"rls_enabled"`
	RLSForced         bool                   `json:"rls_forced"`
	Policies          map[string]*RLSPolicy  `json:"policies"` // policy_name -> RLSPolicy
	Dependencies      []TableDependency      `json:"dependencies"`
	Comment           string                 `json:"comment,omitempty"`
	IsPartitioned     bool                   `json:"is_partitioned"`
	PartitionStrategy string                 `json:"partition_strategy,omitempty"` // RANGE, LIST, HASH
	PartitionKey      string                 `json:"partition_key,omitempty"`      // Column(s) used for partitioning
	LikeClauses       []LikeClause           `json:"like_clauses,omitempty"`       // LIKE clauses in CREATE TABLE
}

// Column represents a table column
type Column struct {
	Name           string    `json:"name"`
	Position       int       `json:"position"` // ordinal_position
	DataType       string    `json:"data_type"`
	IsNullable     bool      `json:"is_nullable"`
	DefaultValue   *string   `json:"default_value,omitempty"`
	MaxLength      *int      `json:"max_length,omitempty"`
	Precision      *int      `json:"precision,omitempty"`
	Scale          *int      `json:"scale,omitempty"`
	Comment        string    `json:"comment,omitempty"`
	Identity       *Identity `json:"identity,omitempty"`
	GeneratedExpr  *string   `json:"generated_expr,omitempty"`  // Expression for generated columns
	IsGenerated    bool      `json:"is_generated,omitempty"`    // True if this is a generated column
}

// Identity represents PostgreSQL identity column configuration
type Identity struct {
	Generation string `json:"generation,omitempty"` // "ALWAYS" or "BY DEFAULT"
	Start      *int64 `json:"start,omitempty"`
	Increment  *int64 `json:"increment,omitempty"`
	Maximum    *int64 `json:"maximum,omitempty"`
	Minimum    *int64 `json:"minimum,omitempty"`
	Cycle      bool   `json:"cycle,omitempty"`
}

// TableType represents different types of table objects
type TableType string

const (
	TableTypeBase TableType = "BASE_TABLE"
	TableTypeView TableType = "VIEW"
	TableTypeTemp TableType = "TEMPORARY"
)

// DependencyType represents different types of database object dependencies
type DependencyType string

const (
	DependencyTypeTable    DependencyType = "TABLE"
	DependencyTypeView     DependencyType = "VIEW"
	DependencyTypeFunction DependencyType = "FUNCTION"
	DependencyTypeSequence DependencyType = "SEQUENCE"
)

// TableDependency represents a dependency between database objects
type TableDependency struct {
	Schema string         `json:"schema"`
	Name   string         `json:"name"`
	Type   DependencyType `json:"type"`
}

// View represents a database view
type View struct {
	Schema       string              `json:"schema"`
	Name         string              `json:"name"`
	Definition   string              `json:"definition"`
	Columns      []string            `json:"columns,omitempty"`   // Ordered list of output column names
	Options      []string            `json:"options,omitempty"`   // View options (e.g., "security_invoker=true", "security_barrier=true")
	Comment      string              `json:"comment,omitempty"`
	Materialized bool                `json:"materialized,omitempty"`
	Indexes      map[string]*Index   `json:"indexes,omitempty"`   // For materialized views only
	Triggers     map[string]*Trigger `json:"triggers,omitempty"`  // For INSTEAD OF triggers on views
}

// Function represents a database function
type Function struct {
	Schema            string       `json:"schema"`
	Name              string       `json:"name"`
	Definition        string       `json:"definition"`
	ReturnType        string       `json:"return_type"`
	Language          string       `json:"language"`
	Parameters        []*Parameter `json:"parameters,omitempty"`
	Comment           string       `json:"comment,omitempty"`
	Volatility        string       `json:"volatility,omitempty"`          // IMMUTABLE, STABLE, VOLATILE
	IsStrict          bool         `json:"is_strict,omitempty"`           // STRICT or null behavior
	IsSecurityDefiner bool         `json:"is_security_definer,omitempty"` // SECURITY DEFINER
	IsLeakproof       bool         `json:"is_leakproof,omitempty"`        // LEAKPROOF
	Parallel          string       `json:"parallel,omitempty"`            // SAFE, UNSAFE, RESTRICTED
	SearchPath        string       `json:"search_path,omitempty"`         // SET search_path value
	Dependencies      []string     `json:"dependencies,omitempty"`        // Function keys (name(args)) this function depends on
}

// GetArguments returns the function arguments string (types only) for function identification.
// This is built dynamically from the Parameters array to ensure it uses normalized types.
// Per PostgreSQL DROP FUNCTION syntax, only input parameters are included (IN, INOUT, VARIADIC).
func (f *Function) GetArguments() string {
	return getInputParameterTypes(f.Parameters)
}

// getInputParameterTypes extracts input parameter types from a parameter list.
// Per PostgreSQL DROP FUNCTION/PROCEDURE syntax, only input parameters are included
// (IN, INOUT, VARIADIC). OUT and TABLE mode parameters are excluded as they're part
// of the return signature.
func getInputParameterTypes(params []*Parameter) string {
	if len(params) == 0 {
		return ""
	}

	var argTypes []string
	for _, param := range params {
		if isInputParameter(param.Mode) {
			argTypes = append(argTypes, param.DataType)
		}
	}

	return strings.Join(argTypes, ", ")
}

// isInputParameter returns true if the parameter mode represents an input parameter.
// PostgreSQL DROP FUNCTION/PROCEDURE syntax only includes input parameters.
func isInputParameter(mode string) bool {
	return mode == "" || mode == "IN" || mode == "INOUT" || mode == "VARIADIC"
}

// Parameter represents a function parameter
type Parameter struct {
	Name         string  `json:"name"`
	DataType     string  `json:"data_type"`
	Mode         string  `json:"mode"` // IN, OUT, INOUT
	Position     int     `json:"position"`
	DefaultValue *string `json:"default_value,omitempty"`
}

// Sequence represents a database sequence
type Sequence struct {
	Schema        string `json:"schema"`
	Name          string `json:"name"`
	DataType      string `json:"data_type"`
	StartValue    int64  `json:"start_value"`
	MinValue      *int64 `json:"min_value,omitempty"`
	MaxValue      *int64 `json:"max_value,omitempty"`
	Increment     int64  `json:"increment"`
	CycleOption   bool   `json:"cycle_option"`
	Cache         *int64 `json:"cache,omitempty"`
	OwnedByTable  string `json:"owned_by_table,omitempty"`
	OwnedByColumn string `json:"owned_by_column,omitempty"`
	Comment       string `json:"comment,omitempty"`
}

// Constraint represents a table constraint
type Constraint struct {
	Schema              string              `json:"schema"`
	Table               string              `json:"table"`
	Name                string              `json:"name"`
	Type                ConstraintType      `json:"type"`
	Columns             []*ConstraintColumn `json:"columns"`
	ReferencedSchema    string              `json:"referenced_schema,omitempty"`
	ReferencedTable     string              `json:"referenced_table,omitempty"`
	ReferencedColumns   []*ConstraintColumn `json:"referenced_columns,omitempty"`
	CheckClause         string              `json:"check_clause,omitempty"`
	ExclusionDefinition string              `json:"exclusion_definition,omitempty"` // Full EXCLUDE definition from pg_get_constraintdef()
	DeleteRule          string              `json:"delete_rule,omitempty"`
	UpdateRule          string              `json:"update_rule,omitempty"`
	Deferrable          bool                `json:"deferrable,omitempty"`
	InitiallyDeferred   bool                `json:"initially_deferred,omitempty"`
	IsValid             bool                `json:"is_valid,omitempty"`
	IsTemporal          bool                `json:"is_temporal,omitempty"` // PG18: temporal constraint (WITHOUT OVERLAPS on PK/UNIQUE, PERIOD on FK)
	Comment             string              `json:"comment,omitempty"`
}

// ConstraintColumn represents a column within a constraint with its position
type ConstraintColumn struct {
	Name     string `json:"name"`
	Position int    `json:"position"` // ordinal_position within the constraint
}

// ConstraintType represents different types of database constraints
type ConstraintType string

const (
	ConstraintTypePrimaryKey ConstraintType = "PRIMARY_KEY"
	ConstraintTypeUnique     ConstraintType = "UNIQUE"
	ConstraintTypeForeignKey ConstraintType = "FOREIGN_KEY"
	ConstraintTypeCheck      ConstraintType = "CHECK"
	ConstraintTypeExclusion  ConstraintType = "EXCLUSION"
)

// Index represents a database index
type Index struct {
	Schema       string         `json:"schema"`
	Table        string         `json:"table"`
	Name         string         `json:"name"`
	Type         IndexType      `json:"type"`
	Method       string         `json:"method"` // btree, hash, gin, gist, etc.
	Columns      []*IndexColumn `json:"columns"`
	IsPartial        bool   `json:"is_partial"`                   // has a WHERE clause
	IsExpression     bool   `json:"is_expression"`                // functional/expression index
	Where            string `json:"where,omitempty"`              // partial index condition
	NullsNotDistinct bool   `json:"nulls_not_distinct,omitempty"` // NULLS NOT DISTINCT (PG15+)
	Comment          string `json:"comment,omitempty"`
}

// IndexColumn represents a column within an index
type IndexColumn struct {
	Name      string `json:"name"`
	Position  int    `json:"position"`
	Direction string `json:"direction,omitempty"` // ASC, DESC
	Operator  string `json:"operator,omitempty"`  // operator class
}

// IndexType represents different types of database indexes
type IndexType string

const (
	IndexTypeRegular IndexType = "REGULAR"
	IndexTypePrimary IndexType = "PRIMARY"
	IndexTypeUnique  IndexType = "UNIQUE"
)

// Trigger represents a database trigger
type Trigger struct {
	Schema            string         `json:"schema"`
	Table             string         `json:"table"`
	Name              string         `json:"name"`
	Timing            TriggerTiming  `json:"timing"` // BEFORE, AFTER, INSTEAD OF
	Events            []TriggerEvent `json:"events"`                        // INSERT, UPDATE, DELETE
	UpdateColumns     []string       `json:"update_columns,omitempty"`      // Column names for UPDATE OF
	Level             TriggerLevel   `json:"level"`                         // ROW, STATEMENT
	Function          string         `json:"function"`
	Condition         string         `json:"condition,omitempty"` // WHEN condition
	Comment           string         `json:"comment,omitempty"`
	IsConstraint      bool           `json:"is_constraint,omitempty"`       // Whether this is a constraint trigger
	Deferrable        bool           `json:"deferrable,omitempty"`          // Can be deferred until end of transaction
	InitiallyDeferred bool           `json:"initially_deferred,omitempty"`  // Whether deferred by default
	OldTable          string         `json:"old_table,omitempty"`           // REFERENCING OLD TABLE AS name
	NewTable          string         `json:"new_table,omitempty"`           // REFERENCING NEW TABLE AS name
}

// TriggerTiming represents the timing of trigger execution
type TriggerTiming string

const (
	TriggerTimingBefore    TriggerTiming = "BEFORE"
	TriggerTimingAfter     TriggerTiming = "AFTER"
	TriggerTimingInsteadOf TriggerTiming = "INSTEAD OF"
)

// TriggerEvent represents the event that triggers the trigger
type TriggerEvent string

const (
	TriggerEventInsert   TriggerEvent = "INSERT"
	TriggerEventUpdate   TriggerEvent = "UPDATE"
	TriggerEventDelete   TriggerEvent = "DELETE"
	TriggerEventTruncate TriggerEvent = "TRUNCATE"
)

// TriggerLevel represents the level at which the trigger fires
type TriggerLevel string

const (
	TriggerLevelRow       TriggerLevel = "ROW"
	TriggerLevelStatement TriggerLevel = "STATEMENT"
)


// RLSPolicy represents a Row Level Security policy
type RLSPolicy struct {
	Schema     string        `json:"schema"`
	Table      string        `json:"table"`
	Name       string        `json:"name"`
	Command    PolicyCommand `json:"command"` // SELECT, INSERT, UPDATE, DELETE, ALL
	Permissive bool          `json:"permissive"`
	Roles      []string      `json:"roles,omitempty"`
	Using      string        `json:"using,omitempty"`      // USING expression
	WithCheck  string        `json:"with_check,omitempty"` // WITH CHECK expression
	Comment    string        `json:"comment,omitempty"`
}

// PolicyCommand represents the command for which the policy applies
type PolicyCommand string

const (
	PolicyCommandAll    PolicyCommand = "ALL"
	PolicyCommandSelect PolicyCommand = "SELECT"
	PolicyCommandInsert PolicyCommand = "INSERT"
	PolicyCommandUpdate PolicyCommand = "UPDATE"
	PolicyCommandDelete PolicyCommand = "DELETE"
)

// TypeKind represents the kind of PostgreSQL type
type TypeKind string

const (
	TypeKindEnum      TypeKind = "ENUM"
	TypeKindComposite TypeKind = "COMPOSITE"
	TypeKindDomain    TypeKind = "DOMAIN"
)

// TypeColumn represents a column in a composite type
type TypeColumn struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Position int    `json:"position"`
}

// DomainConstraint represents a constraint on a domain
type DomainConstraint struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

// Type represents a PostgreSQL user-defined type
type Type struct {
	Schema      string              `json:"schema"`
	Name        string              `json:"name"`
	Kind        TypeKind            `json:"kind"`
	Comment     string              `json:"comment,omitempty"`
	EnumValues  []string            `json:"enum_values,omitempty"` // For ENUM types
	Columns     []*TypeColumn       `json:"columns,omitempty"`     // For composite types
	BaseType    string              `json:"base_type,omitempty"`   // For DOMAIN types
	NotNull     bool                `json:"not_null,omitempty"`    // For DOMAIN types
	Default     string              `json:"default,omitempty"`     // For DOMAIN types
	Constraints []*DomainConstraint `json:"constraints,omitempty"` // For DOMAIN types
}

// Aggregate represents a database aggregate function
type Aggregate struct {
	Schema                   string `json:"schema"`
	Name                     string `json:"name"`
	ReturnType               string `json:"return_type"`
	TransitionFunction       string `json:"transition_function"`
	TransitionFunctionSchema string `json:"transition_function_schema,omitempty"`
	StateType                string `json:"state_type"`
	InitialCondition         string `json:"initial_condition,omitempty"`
	FinalFunction            string `json:"final_function,omitempty"`
	FinalFunctionSchema      string `json:"final_function_schema,omitempty"`
	Comment                  string `json:"comment,omitempty"`
}

// Procedure represents a database procedure
type Procedure struct {
	Schema     string       `json:"schema"`
	Name       string       `json:"name"`
	Definition string       `json:"definition"`
	Language   string       `json:"language"`
	Parameters []*Parameter `json:"parameters,omitempty"`
	Comment    string       `json:"comment,omitempty"`
}

// GetArguments returns the procedure arguments string (types only) for procedure identification.
// This is built dynamically from the Parameters array to ensure it uses normalized types.
// Per PostgreSQL DROP PROCEDURE syntax, only input parameters are included (IN, INOUT, VARIADIC).
func (p *Procedure) GetArguments() string {
	return getInputParameterTypes(p.Parameters)
}

// DefaultPrivilegeObjectType represents the object type for default privileges
type DefaultPrivilegeObjectType string

const (
	DefaultPrivilegeObjectTypeTables    DefaultPrivilegeObjectType = "TABLES"
	DefaultPrivilegeObjectTypeSequences DefaultPrivilegeObjectType = "SEQUENCES"
	DefaultPrivilegeObjectTypeFunctions DefaultPrivilegeObjectType = "FUNCTIONS"
	DefaultPrivilegeObjectTypeTypes     DefaultPrivilegeObjectType = "TYPES"
)

// DefaultPrivilege represents an ALTER DEFAULT PRIVILEGES setting
type DefaultPrivilege struct {
	OwnerRole       string                     `json:"owner_role"`        // Role that owns the default privilege
	ObjectType      DefaultPrivilegeObjectType `json:"object_type"`       // TABLES, SEQUENCES, FUNCTIONS, TYPES
	Grantee         string                     `json:"grantee"`           // Role name or "PUBLIC"
	Privileges      []string                   `json:"privileges"`        // SELECT, INSERT, UPDATE, etc.
	WithGrantOption bool                       `json:"with_grant_option"` // Can grantee grant to others?
}

// GetObjectName returns a unique identifier for the default privilege
func (d *DefaultPrivilege) GetObjectName() string {
	return d.OwnerRole + ":" + string(d.ObjectType) + ":" + d.Grantee
}

// PrivilegeObjectType represents the object type for explicit privilege grants
type PrivilegeObjectType string

const (
	PrivilegeObjectTypeTable     PrivilegeObjectType = "TABLE"
	PrivilegeObjectTypeView      PrivilegeObjectType = "VIEW"
	PrivilegeObjectTypeSequence  PrivilegeObjectType = "SEQUENCE"
	PrivilegeObjectTypeFunction  PrivilegeObjectType = "FUNCTION"
	PrivilegeObjectTypeProcedure PrivilegeObjectType = "PROCEDURE"
	PrivilegeObjectTypeType      PrivilegeObjectType = "TYPE"
)

// Privilege represents an explicit privilege grant on a schema object
type Privilege struct {
	ObjectType      PrivilegeObjectType `json:"object_type"`       // TABLE, VIEW, SEQUENCE, FUNCTION, PROCEDURE, TYPE
	ObjectName      string              `json:"object_name"`       // table name or function signature
	Grantee         string              `json:"grantee"`           // role name or "PUBLIC"
	Privileges      []string            `json:"privileges"`        // [SELECT, INSERT, UPDATE, ...] or [EXECUTE] or [USAGE]
	WithGrantOption bool                `json:"with_grant_option"` // Can grantee grant to others?
}

// GetObjectKey returns a unique identifier for the privilege (object + grantee)
// Note: This intentionally excludes WithGrantOption so that privilege modifications
// (e.g., adding or removing GRANT OPTION) are detected as modifications, not as
// separate add/drop operations.
func (p *Privilege) GetObjectKey() string {
	return string(p.ObjectType) + ":" + p.ObjectName + ":" + p.Grantee
}

// GetFullKey returns a unique identifier including WithGrantOption.
// Use this when you need to distinguish between the same privilege with different grant options.
func (p *Privilege) GetFullKey() string {
	grantOption := "0"
	if p.WithGrantOption {
		grantOption = "1"
	}
	return string(p.ObjectType) + ":" + p.ObjectName + ":" + p.Grantee + ":" + grantOption
}

// GetObjectName returns the object name for the privilege
func (p *Privilege) GetObjectName() string {
	return p.ObjectName
}

// RevokedDefaultPrivilege represents an explicit revoke of a default PUBLIC privilege
// This is used to track when default PUBLIC grants (e.g., EXECUTE on functions) are revoked
type RevokedDefaultPrivilege struct {
	ObjectType PrivilegeObjectType `json:"object_type"` // FUNCTION, PROCEDURE, TYPE
	ObjectName string              `json:"object_name"` // function signature or type name
	Privileges []string            `json:"privileges"`  // [EXECUTE] or [USAGE]
}

// GetObjectKey returns a unique identifier for the revoked default privilege
func (r *RevokedDefaultPrivilege) GetObjectKey() string {
	return string(r.ObjectType) + ":" + r.ObjectName
}

// GetObjectName returns the object name for the revoked default privilege
func (r *RevokedDefaultPrivilege) GetObjectName() string {
	return r.ObjectName
}

// ColumnPrivilege represents a column-level privilege grant on a table
// Column-level grants allow fine-grained access control on specific columns
// rather than the entire table. Stored in pg_attribute.attacl.
type ColumnPrivilege struct {
	TableName       string   `json:"table_name"`        // table containing the columns
	Columns         []string `json:"columns"`           // columns for this grant (sorted alphabetically)
	Grantee         string   `json:"grantee"`           // role name or "PUBLIC"
	Privileges      []string `json:"privileges"`        // SELECT, INSERT, UPDATE, REFERENCES only
	WithGrantOption bool     `json:"with_grant_option"` // Can grantee grant to others?
}

// GetObjectKey returns a unique identifier for the column privilege.
// MUST include sorted columns - two grants on same table/role with different columns are different objects.
func (cp *ColumnPrivilege) GetObjectKey() string {
	sortedCols := make([]string, len(cp.Columns))
	copy(sortedCols, cp.Columns)
	sort.Strings(sortedCols)
	colKey := strings.Join(sortedCols, ",")
	return "COLUMN:" + cp.TableName + ":" + colKey + ":" + cp.Grantee
}

// GetFullKey returns a unique identifier including grant option.
// Use this when you need to distinguish between the same privilege with different grant options.
func (cp *ColumnPrivilege) GetFullKey() string {
	grantOption := "0"
	if cp.WithGrantOption {
		grantOption = "1"
	}
	return cp.GetObjectKey() + ":" + grantOption
}

// GetObjectName returns the table name for the column privilege
func (cp *ColumnPrivilege) GetObjectName() string {
	return cp.TableName
}

// NewIR creates a new empty catalog IR
func NewIR() *IR {
	return &IR{
		Schemas: make(map[string]*Schema),
	}
}

// GetSchema retrieves a schema by name with thread safety.
// Returns the schema and true if found, or nil and false if not found.
func (c *IR) GetSchema(name string) (*Schema, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	schema, ok := c.Schemas[name]
	return schema, ok
}

// CreateSchema creates a new schema with the given name.
// If the schema already exists, it returns the existing schema.
func (c *IR) CreateSchema(name string) *Schema {
	return c.getOrCreateSchema(name)
}

// GetOrCreateSchema gets or creates a database schema by name with thread safety.
// This is an exported version of the internal getOrCreateSchema method.
func (c *IR) GetOrCreateSchema(name string) *Schema {
	return c.getOrCreateSchema(name)
}

// getOrCreateSchema gets or creates a database schema by name (internal method)
func (c *IR) getOrCreateSchema(name string) *Schema {
	c.mu.Lock()
	defer c.mu.Unlock()

	if schema, exists := c.Schemas[name]; exists {
		return schema
	}

	schema := &Schema{
		Name:       name,
		Tables:     make(map[string]*Table),
		Views:      make(map[string]*View),
		Functions:  make(map[string]*Function),
		Procedures: make(map[string]*Procedure),
		Aggregates: make(map[string]*Aggregate),
		Sequences:  make(map[string]*Sequence),
		Types:      make(map[string]*Type),
	}
	c.Schemas[name] = schema
	return schema
}

// Thread-safe getter and setter methods for Schema

// GetTable retrieves a table from the schema with thread safety
func (s *Schema) GetTable(name string) (*Table, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	table, ok := s.Tables[name]
	return table, ok
}

// SetTable sets a table in the schema with thread safety
func (s *Schema) SetTable(name string, table *Table) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tables[name] = table
}

// GetView retrieves a view from the schema with thread safety
func (s *Schema) GetView(name string) (*View, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	view, ok := s.Views[name]
	return view, ok
}

// SetView sets a view in the schema with thread safety
func (s *Schema) SetView(name string, view *View) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Views[name] = view
}

// GetFunction retrieves a function from the schema with thread safety
func (s *Schema) GetFunction(name string) (*Function, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	function, ok := s.Functions[name]
	return function, ok
}

// SetFunction sets a function in the schema with thread safety
func (s *Schema) SetFunction(name string, function *Function) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Functions[name] = function
}

// GetProcedure retrieves a procedure from the schema with thread safety
func (s *Schema) GetProcedure(name string) (*Procedure, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	procedure, ok := s.Procedures[name]
	return procedure, ok
}

// SetProcedure sets a procedure in the schema with thread safety
func (s *Schema) SetProcedure(name string, procedure *Procedure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Procedures[name] = procedure
}

// GetAggregate retrieves an aggregate from the schema with thread safety
func (s *Schema) GetAggregate(name string) (*Aggregate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	aggregate, ok := s.Aggregates[name]
	return aggregate, ok
}

// SetAggregate sets an aggregate in the schema with thread safety
func (s *Schema) SetAggregate(name string, aggregate *Aggregate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Aggregates[name] = aggregate
}

// GetSequence retrieves a sequence from the schema with thread safety
func (s *Schema) GetSequence(name string) (*Sequence, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sequence, ok := s.Sequences[name]
	return sequence, ok
}

// SetSequence sets a sequence in the schema with thread safety
func (s *Schema) SetSequence(name string, sequence *Sequence) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sequences[name] = sequence
}

// GetType retrieves a type from the schema with thread safety
func (s *Schema) GetType(name string) (*Type, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	typ, ok := s.Types[name]
	return typ, ok
}

// SetType sets a type in the schema with thread safety
func (s *Schema) SetType(name string, typ *Type) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Types[name] = typ
}

// GetObjectName implementations for DiffSource interface
func (t *Table) GetObjectName() string      { return t.Name }
func (c *Column) GetObjectName() string     { return c.Name }
func (c *Constraint) GetObjectName() string { return c.Name }
func (i *Index) GetObjectName() string      { return i.Name }
func (t *Trigger) GetObjectName() string    { return t.Name }
func (p *RLSPolicy) GetObjectName() string  { return p.Name }
func (f *Function) GetObjectName() string   { return f.Name }
func (p *Procedure) GetObjectName() string  { return p.Name }
func (v *View) GetObjectName() string       { return v.Name }
func (s *Sequence) GetObjectName() string   { return s.Name }
func (t *Type) GetObjectName() string       { return t.Name }

