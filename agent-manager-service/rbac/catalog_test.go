// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// The catalog is a wall of `const ... Permission = "..."` declarations with no
// enumerable slice, deliberately: a slice would be one more copy to drift. So
// the invariants below read the declarations out of the source they guard, which
// cannot fall out of step with them.
package rbac

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// declaredPermissions returns every Permission constant declared in
// permissions.go, keyed by Go identifier. Parsing the source is what makes this
// the catalog rather than a second copy of it.
func declaredPermissions(t *testing.T) map[string]Permission {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "permissions.go", nil, 0)
	if err != nil {
		t.Fatalf("parse permissions.go: %v", err)
	}
	out := make(map[string]Permission)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "Permission" {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("%s is not declared with a string literal", name.Name)
				}
				out[name.Name] = Permission(lit.Value[1 : len(lit.Value)-1])
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no Permission constants found; the parser stopped matching the source")
	}
	return out
}

// TestEnvironmentTierScopesExist pins the two scopes the environment-tier axis
// is built on, and the shape of the strings Thunder composes them from.
func TestEnvironmentTierScopesExist(t *testing.T) {
	if got, want := AgentEnvNonProduction.Scope(), "amp:agent:env-non-production"; got != want {
		t.Errorf("AgentEnvNonProduction.Scope() = %q, want %q", got, want)
	}
	if got, want := AgentEnvProduction.Scope(), "amp:agent:env-production"; got != want {
		t.Errorf("AgentEnvProduction.Scope() = %q, want %q", got, want)
	}
}

// TestCatalogScopeStringsAreUnique catches the copy-paste that gives two
// constants the same scope. Thunder would accept it and one of the two would
// silently grant the other's permission.
func TestCatalogScopeStringsAreUnique(t *testing.T) {
	seen := make(map[Permission]string)
	for name, perm := range declaredPermissions(t) {
		if first, dup := seen[perm]; dup {
			t.Errorf("scope %q is declared twice: %s and %s", perm, first, name)
			continue
		}
		seen[perm] = name
	}
}

// TestAdminHoldsEntireCatalog is the invariant that makes every other role a
// subset question. A scope missing from Admin is a scope nobody can be granted
// through a predefined role.
func TestAdminHoldsEntireCatalog(t *testing.T) {
	held := make(map[Permission]bool, len(PredefinedRolePermissions[RoleAdmin]))
	for _, perm := range PredefinedRolePermissions[RoleAdmin] {
		held[perm] = true
	}
	for name, perm := range declaredPermissions(t) {
		if !held[perm] {
			t.Errorf("%s (%q) is in the catalog but not held by %s", name, perm, RoleAdmin)
		}
	}
}

// TestPredefinedRolesHoldOnlyCatalogScopes catches a role granting a scope that
// no longer exists — the state a removed constant leaves behind, and the one the
// Thunder resource-server tree silently drops on import.
func TestPredefinedRolesHoldOnlyCatalogScopes(t *testing.T) {
	catalog := make(map[Permission]bool)
	for _, perm := range declaredPermissions(t) {
		catalog[perm] = true
	}
	for role, perms := range PredefinedRolePermissions {
		seen := make(map[Permission]bool, len(perms))
		for _, perm := range perms {
			if !catalog[perm] {
				t.Errorf("role %q holds %q, which is not in the catalog", role, perm)
			}
			if seen[perm] {
				t.Errorf("role %q holds %q twice", role, perm)
			}
			seen[perm] = true
		}
	}
}
