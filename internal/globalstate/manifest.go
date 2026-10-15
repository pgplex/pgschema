package globalstate

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

var settingNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

var supportedOwnershipKinds = map[string]bool{
	"database": true, "schema": true, "table": true, "view": true,
	"materialized_view": true, "sequence": true, "function": true,
	"procedure": true, "type": true,
}

var supportedDefaultPrivilegeTypes = map[string]bool{
	"tables": true, "sequences": true, "functions": true, "types": true, "schemas": true,
}

var allowedDefaultPrivileges = map[string]map[string]bool{
	"tables":    {"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true, "TRUNCATE": true, "REFERENCES": true, "TRIGGER": true, "MAINTAIN": true},
	"sequences": {"USAGE": true, "SELECT": true, "UPDATE": true},
	"functions": {"EXECUTE": true},
	"types":     {"USAGE": true},
	"schemas":   {"USAGE": true, "CREATE": true},
}

type State string

const (
	StatePresent  State = "present"
	StateAbsent   State = "absent"
	StateExternal State = "external"
)

type Manifest struct {
	Version           int                `toml:"version"`
	Roles             []Role             `toml:"roles"`
	Memberships       []Membership       `toml:"memberships"`
	Ownership         []Ownership        `toml:"ownership"`
	DefaultPrivileges []DefaultPrivilege `toml:"default_privileges"`
}

type Role struct {
	Name            string            `toml:"name"`
	State           State             `toml:"state"`
	Login           bool              `toml:"login"`
	Inherit         bool              `toml:"inherit"`
	CreateDB        bool              `toml:"createdb"`
	CreateRole      bool              `toml:"createrole"`
	Replication     bool              `toml:"replication"`
	BypassRLS       bool              `toml:"bypassrls"`
	ConnectionLimit int               `toml:"connection_limit"`
	ValidUntil      string            `toml:"valid_until"`
	Configuration   map[string]string `toml:"configuration"`
}

type Ownership struct {
	Kind  string `toml:"kind"`
	Name  string `toml:"name"`
	Owner string `toml:"owner"`
}

type DefaultPrivilege struct {
	Owner       string   `toml:"owner"`
	ObjectType  string   `toml:"object_type"`
	Grantee     string   `toml:"grantee"`
	Privileges  []string `toml:"privileges"`
	GrantOption bool     `toml:"grant_option"`
	State       State    `toml:"state"`
}

type Membership struct {
	Role    string `toml:"role"`
	Member  string `toml:"member"`
	State   State  `toml:"state"`
	Admin   bool   `toml:"admin"`
	Inherit bool   `toml:"inherit"`
	Set     bool   `toml:"set"`
}

type rawManifest struct {
	Version           int                `toml:"version"`
	Roles             []rawRole          `toml:"roles"`
	Memberships       []rawMembership    `toml:"memberships"`
	Ownership         []Ownership        `toml:"ownership"`
	DefaultPrivileges []DefaultPrivilege `toml:"default_privileges"`
}

type rawRole struct {
	Name            string            `toml:"name"`
	State           State             `toml:"state"`
	Login           *bool             `toml:"login"`
	Inherit         *bool             `toml:"inherit"`
	CreateDB        *bool             `toml:"createdb"`
	CreateRole      *bool             `toml:"createrole"`
	Replication     *bool             `toml:"replication"`
	BypassRLS       *bool             `toml:"bypassrls"`
	ConnectionLimit *int              `toml:"connection_limit"`
	ValidUntil      *string           `toml:"valid_until"`
	Configuration   map[string]string `toml:"configuration"`
}

type rawMembership struct {
	Role    string `toml:"role"`
	Member  string `toml:"member"`
	State   State  `toml:"state"`
	Admin   *bool  `toml:"admin"`
	Inherit *bool  `toml:"inherit"`
	Set     *bool  `toml:"set"`
}

func LoadManifest(path string) (Manifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read global state file: %w", err)
	}

	var raw rawManifest
	metadata, err := toml.Decode(string(contents), &raw)
	if err != nil {
		return Manifest{}, fmt.Errorf("decode global state file: %w", err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return Manifest{}, fmt.Errorf("unknown field in global state file: %s", undecoded[0])
	}
	if raw.Version != 1 {
		return Manifest{}, fmt.Errorf("unsupported global state version %d: expected 1", raw.Version)
	}

	manifest := Manifest{Version: raw.Version}
	for _, rawRole := range raw.Roles {
		state := rawRole.State
		if state == "" {
			state = StatePresent
		}
		role := Role{
			Name:            rawRole.Name,
			State:           state,
			Inherit:         valueOr(rawRole.Inherit, true),
			ConnectionLimit: valueOr(rawRole.ConnectionLimit, -1),
			Login:           valueOr(rawRole.Login, false),
			CreateDB:        valueOr(rawRole.CreateDB, false),
			CreateRole:      valueOr(rawRole.CreateRole, false),
			Replication:     valueOr(rawRole.Replication, false),
			BypassRLS:       valueOr(rawRole.BypassRLS, false),
			ValidUntil:      valueOr(rawRole.ValidUntil, "infinity"),
			Configuration:   rawRole.Configuration,
		}
		manifest.Roles = append(manifest.Roles, role)
	}
	manifest.Ownership = append(manifest.Ownership, raw.Ownership...)
	for _, privilege := range raw.DefaultPrivileges {
		if privilege.State == "" {
			privilege.State = StatePresent
		}
		for i := range privilege.Privileges {
			privilege.Privileges[i] = strings.ToUpper(privilege.Privileges[i])
		}
		sort.Strings(privilege.Privileges)
		manifest.DefaultPrivileges = append(manifest.DefaultPrivileges, privilege)
	}
	for _, rawMembership := range raw.Memberships {
		state := rawMembership.State
		if state == "" {
			state = StatePresent
		}
		manifest.Memberships = append(manifest.Memberships, Membership{
			Role:    rawMembership.Role,
			Member:  rawMembership.Member,
			State:   state,
			Admin:   valueOr(rawMembership.Admin, false),
			Inherit: valueOr(rawMembership.Inherit, true),
			Set:     valueOr(rawMembership.Set, true),
		})
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}

	sort.Slice(manifest.Roles, func(i, j int) bool { return manifest.Roles[i].Name < manifest.Roles[j].Name })
	sort.Slice(manifest.Memberships, func(i, j int) bool {
		if manifest.Memberships[i].Role == manifest.Memberships[j].Role {
			return manifest.Memberships[i].Member < manifest.Memberships[j].Member
		}
		return manifest.Memberships[i].Role < manifest.Memberships[j].Role
	})
	sort.Slice(manifest.Ownership, func(i, j int) bool {
		if manifest.Ownership[i].Kind == manifest.Ownership[j].Kind {
			return manifest.Ownership[i].Name < manifest.Ownership[j].Name
		}
		return manifest.Ownership[i].Kind < manifest.Ownership[j].Kind
	})
	sort.Slice(manifest.DefaultPrivileges, func(i, j int) bool {
		return defaultPrivilegeKey(manifest.DefaultPrivileges[i].Owner, manifest.DefaultPrivileges[i].ObjectType, manifest.DefaultPrivileges[i].Grantee).Path() <
			defaultPrivilegeKey(manifest.DefaultPrivileges[j].Owner, manifest.DefaultPrivileges[j].ObjectType, manifest.DefaultPrivileges[j].Grantee).Path()
	})
	return manifest, nil
}

func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

func validateManifest(manifest Manifest) error {
	roles := make(map[string]State, len(manifest.Roles))
	for _, role := range manifest.Roles {
		if role.Name == "" {
			return fmt.Errorf("global role name must not be empty")
		}
		if role.State != StatePresent && role.State != StateExternal && role.State != StateAbsent {
			return fmt.Errorf("role %q has unsupported state %q: expected present, external, or absent", role.Name, role.State)
		}
		if _, exists := roles[role.Name]; exists {
			return fmt.Errorf("global role %q is declared more than once", role.Name)
		}
		roles[role.Name] = role.State
		if role.ConnectionLimit < -1 {
			return fmt.Errorf("role %q has invalid connection_limit %d", role.Name, role.ConnectionLimit)
		}
		if role.ValidUntil != "" && role.ValidUntil != "infinity" {
			if _, err := time.Parse(time.RFC3339, role.ValidUntil); err != nil {
				return fmt.Errorf("role %q has invalid valid_until: use RFC3339 or infinity", role.Name)
			}
		}
		for setting := range role.Configuration {
			if !settingNamePattern.MatchString(setting) {
				return fmt.Errorf("role %q has invalid configuration key %q", role.Name, setting)
			}
		}
	}

	memberships := make(map[MembershipRef]struct{}, len(manifest.Memberships))
	for _, membership := range manifest.Memberships {
		if membership.Role == "" || membership.Member == "" {
			return fmt.Errorf("membership role and member must not be empty")
		}
		if membership.State != StatePresent && membership.State != StateAbsent {
			return fmt.Errorf("membership %q has unsupported state %q: expected present or absent", MembershipKey(membership.Role, membership.Member).Path(), membership.State)
		}
		key := MembershipKey(membership.Role, membership.Member)
		if _, exists := memberships[key]; exists {
			return fmt.Errorf("membership %q is declared more than once", key.Path())
		}
		memberships[key] = struct{}{}
		if roles[membership.Role] == StateAbsent {
			return fmt.Errorf("membership role %q is declared absent", membership.Role)
		}
		if roles[membership.Member] == StateAbsent {
			return fmt.Errorf("membership member role %q is declared absent", membership.Member)
		}
	}
	ownership := make(map[OwnershipRef]struct{}, len(manifest.Ownership))
	for _, object := range manifest.Ownership {
		if !supportedOwnershipKinds[object.Kind] {
			return fmt.Errorf("ownership %q has unsupported kind %q", object.Name, object.Kind)
		}
		if object.Name == "" || object.Owner == "" {
			return fmt.Errorf("ownership kind, name, and owner must not be empty")
		}
		key := ownershipKey(object.Kind, object.Name)
		if _, exists := ownership[key]; exists {
			return fmt.Errorf("ownership %q is declared more than once", key.Path())
		}
		ownership[key] = struct{}{}
		if roles[object.Owner] == StateAbsent {
			return fmt.Errorf("ownership role %q is declared absent", object.Owner)
		}
	}
	defaults := make(map[DefaultPrivilegeRef]struct{}, len(manifest.DefaultPrivileges))
	for _, privilege := range manifest.DefaultPrivileges {
		if !supportedDefaultPrivilegeTypes[privilege.ObjectType] {
			return fmt.Errorf("default privilege has unsupported object_type %q", privilege.ObjectType)
		}
		if privilege.Owner == "" || privilege.Grantee == "" {
			return fmt.Errorf("default privilege owner and grantee must not be empty")
		}
		if privilege.State != StatePresent && privilege.State != StateAbsent {
			return fmt.Errorf("default privilege %q has unsupported state %q", privilege.ObjectType, privilege.State)
		}
		if privilege.State == StatePresent && len(privilege.Privileges) == 0 {
			return fmt.Errorf("present default privilege %q must declare at least one privilege", privilege.ObjectType)
		}
		seenPrivileges := make(map[string]struct{}, len(privilege.Privileges))
		for _, name := range privilege.Privileges {
			if !allowedDefaultPrivileges[privilege.ObjectType][name] {
				return fmt.Errorf("default privilege %q is invalid for %s", name, privilege.ObjectType)
			}
			if _, exists := seenPrivileges[name]; exists {
				return fmt.Errorf("duplicate privilege %q for %s", name, privilege.ObjectType)
			}
			seenPrivileges[name] = struct{}{}
		}
		key := defaultPrivilegeKey(privilege.Owner, privilege.ObjectType, privilege.Grantee)
		if _, exists := defaults[key]; exists {
			return fmt.Errorf("default privilege %q is declared more than once", key.Path())
		}
		defaults[key] = struct{}{}
		if roles[privilege.Owner] == StateAbsent {
			return fmt.Errorf("default privilege owner role %q is declared absent", privilege.Owner)
		}
		if privilege.Grantee != "PUBLIC" && roles[privilege.Grantee] == StateAbsent {
			return fmt.Errorf("default privilege grantee role %q is declared absent", privilege.Grantee)
		}
	}
	return nil
}
