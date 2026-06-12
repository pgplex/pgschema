package diff

import (
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestPolicyReferencesNewFunction_Unqualified(t *testing.T) {
	functions := []*ir.Function{
		{Schema: "public", Name: "tenant_filter"},
	}
	lookup := buildFunctionLookup(functions)

	policy := &ir.RLSPolicy{
		Schema: "public",
		Table:  "users",
		Name:   "tenant_policy",
		Using:  "tenant_filter(tenant_id)",
	}

	if !policyReferencesNewFunction(policy, lookup) {
		t.Fatalf("expected policy to reference newly added function")
	}
}

func TestPolicyReferencesNewFunction_Qualified(t *testing.T) {
	functions := []*ir.Function{
		{Schema: "auth", Name: "tenant_filter"},
	}
	lookup := buildFunctionLookup(functions)

	policy := &ir.RLSPolicy{
		Schema: "public",
		Table:  "users",
		Name:   "tenant_policy",
		Using:  "auth.tenant_filter(tenant_id)",
	}

	if !policyReferencesNewFunction(policy, lookup) {
		t.Fatalf("expected policy to match schema-qualified helper function")
	}
}

func TestPolicyReferencesNewFunction_BuiltInIgnored(t *testing.T) {
	functions := []*ir.Function{
		{Schema: "public", Name: "tenant_filter"},
	}
	lookup := buildFunctionLookup(functions)

	policy := &ir.RLSPolicy{
		Schema: "public",
		Table:  "audit",
		Name:   "audit_policy",
		Using:  "user_name = CURRENT_USER",
	}

	if policyReferencesNewFunction(policy, lookup) {
		t.Fatalf("expected policy referencing only built-in functions to remain inline")
	}
}

func TestPolicyReferencesOtherNewTable_IdentifierAware(t *testing.T) {
	lookup := map[string]struct{}{
		"public.orders": {},
		"public.users":  {},
		"public.user":   {},
	}

	if !policyReferencesOtherNewTable(&ir.RLSPolicy{
		Schema: "public",
		Table:  "orders",
		Using:  "EXISTS (SELECT 1 FROM users u WHERE u.id = orders.user_id)",
	}, lookup) {
		t.Fatalf("expected policy referencing another new table to be deferred")
	}

	if policyReferencesOtherNewTable(&ir.RLSPolicy{
		Schema: "public",
		Table:  "orders",
		Using:  "orders_archive.user_id = 1",
	}, lookup) {
		t.Fatalf("did not expect substring-only matches inside longer identifiers")
	}
}
