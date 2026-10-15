package diff

import "github.com/pgplex/pgschema/ir"

type OwnerIdentityKind string

const (
	OwnerIdentityRelation         OwnerIdentityKind = "relation"
	OwnerIdentityType             OwnerIdentityKind = "type"
	OwnerIdentityRoutine          OwnerIdentityKind = "routine"
	OwnerIdentityDefaultPrivilege OwnerIdentityKind = "default_privilege"
)

type OwnerIdentity struct {
	Kind                   OwnerIdentityKind
	Schema                 string
	Name                   string
	Arguments              string
	RoutineKind            string
	CatalogRoutineIdentity bool
	Role                   string
}

func ExecutionOwnerIdentity(d Diff) OwnerIdentity {
	if d.OwnerIdentity.Kind != "" {
		return d.OwnerIdentity
	}
	switch source := d.Source.(type) {
	case *ir.Function:
		return functionOwnerIdentity(source)
	case *functionDiff:
		return functionOwnerIdentity(source.Old)
	case *ir.Procedure:
		return procedureOwnerIdentity(source)
	case *procedureDiff:
		return procedureOwnerIdentity(source.Old)
	case *ir.Aggregate:
		return aggregateOwnerIdentity(source)
	case *aggregateDiff:
		return aggregateOwnerIdentity(source.Old)
	case *ir.Table:
		return relationOwnerIdentity(source.Schema, source.Name)
	case *tableDiff:
		return relationOwnerIdentity(source.Table.Schema, source.Table.Name)
	case *ir.View:
		return relationOwnerIdentity(source.Schema, source.Name)
	case *viewDiff:
		return relationOwnerIdentity(source.Old.Schema, source.Old.Name)
	case *ir.Sequence:
		return relationOwnerIdentity(source.Schema, source.Name)
	case *sequenceDiff:
		return relationOwnerIdentity(source.Old.Schema, source.Old.Name)
	case *ir.Constraint:
		return relationOwnerIdentity(source.Schema, source.Table)
	case *ConstraintDiff:
		return relationOwnerIdentity(source.Old.Schema, source.Old.Table)
	case *ir.Index:
		return relationOwnerIdentity(source.Schema, source.Table)
	case *IndexDiff:
		return relationOwnerIdentity(source.Old.Schema, source.Old.Table)
	case *ir.Trigger:
		return relationOwnerIdentity(source.Schema, source.Table)
	case *triggerDiff:
		return relationOwnerIdentity(source.Old.Schema, source.Old.Table)
	case *ir.RLSPolicy:
		return relationOwnerIdentity(source.Schema, source.Table)
	case *policyDiff:
		return relationOwnerIdentity(source.Old.Schema, source.Old.Table)
	case *rlsChange:
		return relationOwnerIdentity(source.Table.Schema, source.Table.Name)
	case *ir.Type:
		return typeOwnerIdentity(source.Schema, source.Name)
	case *typeDiff:
		return typeOwnerIdentity(source.Old.Schema, source.Old.Name)
	case *ir.DefaultPrivilege:
		return OwnerIdentity{Kind: OwnerIdentityDefaultPrivilege, Role: source.OwnerRole}
	case *defaultPrivilegeDiff:
		return OwnerIdentity{Kind: OwnerIdentityDefaultPrivilege, Role: source.Old.OwnerRole}
	case *ir.Privilege:
		return privilegeOwnerIdentity(source)
	case *privilegeDiff:
		return privilegeOwnerIdentity(source.Old)
	case *ir.RevokedDefaultPrivilege:
		return revokedDefaultPrivilegeOwnerIdentity(source)
	case *ir.ColumnPrivilege:
		return relationOwnerIdentity("", source.TableName)
	case *columnPrivilegeDiff:
		return relationOwnerIdentity("", source.Old.TableName)
	default:
		return OwnerIdentity{}
	}
}

func RelationOwnerIdentity(schema, name string) OwnerIdentity {
	return relationOwnerIdentity(schema, name)
}

func relationOwnerIdentity(schema, name string) OwnerIdentity {
	return OwnerIdentity{Kind: OwnerIdentityRelation, Schema: schema, Name: name}
}

func typeOwnerIdentity(schema, name string) OwnerIdentity {
	return OwnerIdentity{Kind: OwnerIdentityType, Schema: schema, Name: name}
}

func functionOwnerIdentity(function *ir.Function) OwnerIdentity {
	if function == nil {
		return OwnerIdentity{}
	}
	return routineOwnerIdentity(function.Schema, function.Name, function.GetArguments(), "f")
}

func procedureOwnerIdentity(procedure *ir.Procedure) OwnerIdentity {
	if procedure == nil {
		return OwnerIdentity{}
	}
	return routineOwnerIdentity(procedure.Schema, procedure.Name, procedure.GetArguments(), "p")
}

func aggregateOwnerIdentity(aggregate *ir.Aggregate) OwnerIdentity {
	if aggregate == nil {
		return OwnerIdentity{}
	}
	return OwnerIdentity{
		Kind:                   OwnerIdentityRoutine,
		Schema:                 aggregate.Schema,
		Name:                   aggregate.Name + "(" + aggregate.Arguments + ")",
		RoutineKind:            "a",
		CatalogRoutineIdentity: true,
	}
}

func routineOwnerIdentity(schema, name, arguments, kind string) OwnerIdentity {
	return OwnerIdentity{
		Kind: OwnerIdentityRoutine, Schema: schema, Name: name, Arguments: arguments, RoutineKind: kind,
	}
}

func privilegeOwnerIdentity(privilege *ir.Privilege) OwnerIdentity {
	if privilege == nil {
		return OwnerIdentity{}
	}
	switch privilege.ObjectType {
	case ir.PrivilegeObjectTypeTable, ir.PrivilegeObjectTypeView, ir.PrivilegeObjectTypeSequence:
		return relationOwnerIdentity("", privilege.ObjectName)
	case ir.PrivilegeObjectTypeFunction:
		return OwnerIdentity{Kind: OwnerIdentityRoutine, Name: privilege.ObjectName, RoutineKind: "f", CatalogRoutineIdentity: true}
	case ir.PrivilegeObjectTypeProcedure:
		return OwnerIdentity{Kind: OwnerIdentityRoutine, Name: privilege.ObjectName, RoutineKind: "p", CatalogRoutineIdentity: true}
	case ir.PrivilegeObjectTypeType:
		return typeOwnerIdentity("", privilege.ObjectName)
	default:
		return OwnerIdentity{}
	}
}

func revokedDefaultPrivilegeOwnerIdentity(privilege *ir.RevokedDefaultPrivilege) OwnerIdentity {
	if privilege == nil {
		return OwnerIdentity{}
	}
	switch privilege.ObjectType {
	case ir.PrivilegeObjectTypeFunction:
		return OwnerIdentity{Kind: OwnerIdentityRoutine, Name: privilege.ObjectName, RoutineKind: "f", CatalogRoutineIdentity: true}
	case ir.PrivilegeObjectTypeProcedure:
		return OwnerIdentity{Kind: OwnerIdentityRoutine, Name: privilege.ObjectName, RoutineKind: "p", CatalogRoutineIdentity: true}
	case ir.PrivilegeObjectTypeType:
		return typeOwnerIdentity("", privilege.ObjectName)
	default:
		return OwnerIdentity{}
	}
}
