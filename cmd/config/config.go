package config

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// ResolvedConfig is the final, flattened configuration consumed by plan/apply/dump commands.
// It is produced by merging base config with an optional named env override via LoadConfig.
type ResolvedConfig struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
	SSLMode  string

	Schema string
	File   string

	PlanHost     string
	PlanPort     int
	PlanDB       string
	PlanUser     string
	PlanPassword string
	PlanSSLMode  string

	LockTimeout     string
	AutoApprove     bool
	ApplicationName string

	OutputHuman string
	OutputJSON  string
	OutputSQL   string

	MultiFile  bool
	NoComments bool

	NoColor bool

	Schemas *SchemasConfig
}

type SchemasConfig struct {
	Query string `toml:"query"`
}

// envConfig is the TOML deserialization target. It mirrors ResolvedConfig but carries
// toml struct tags. Both the base level and each [env.*] block parse into this type.
// It is unexported — callers only see the merged ResolvedConfig.
type envConfig struct {
	Host            string         `toml:"host"`
	Port            int            `toml:"port"`
	DB              string         `toml:"db"`
	User            string         `toml:"user"`
	Password        string         `toml:"password"`
	SSLMode         string         `toml:"sslmode"`
	Schema          string         `toml:"schema"`
	File            string         `toml:"file"`
	PlanHost        string         `toml:"plan-host"`
	PlanPort        int            `toml:"plan-port"`
	PlanDB          string         `toml:"plan-db"`
	PlanUser        string         `toml:"plan-user"`
	PlanPassword    string         `toml:"plan-password"`
	PlanSSLMode     string         `toml:"plan-sslmode"`
	LockTimeout     string         `toml:"lock-timeout"`
	AutoApprove     bool           `toml:"auto-approve"`
	ApplicationName string         `toml:"application-name"`
	OutputHuman     string         `toml:"output-human"`
	OutputJSON      string         `toml:"output-json"`
	OutputSQL       string         `toml:"output-sql"`
	MultiFile       bool           `toml:"multi-file"`
	NoComments      bool           `toml:"no-comments"`
	NoColor         bool           `toml:"no-color"`
	Schemas         *SchemasConfig `toml:"schemas"`
}

// fileConfig is the top-level TOML structure: base-level fields (embedded envConfig)
// plus a map of named environment overrides ([env.dev], [env.prod], etc.).
type fileConfig struct {
	envConfig
	Env map[string]envConfig `toml:"env"`
}

func LoadConfig(path string, envName string) (*ResolvedConfig, error) {
	var fc fileConfig
	meta, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	resolved := toResolved(&fc.envConfig)

	if envName != "" {
		env, ok := fc.Env[envName]
		if !ok {
			return nil, fmt.Errorf("environment %q not found in %s", envName, path)
		}
		mergeEnvConfig(resolved, &env, meta, "env", envName)
	}

	return resolved, nil
}

func toResolved(ec *envConfig) *ResolvedConfig {
	return &ResolvedConfig{
		Host:            ec.Host,
		Port:            ec.Port,
		DB:              ec.DB,
		User:            ec.User,
		Password:        ec.Password,
		SSLMode:         ec.SSLMode,
		Schema:          ec.Schema,
		File:            ec.File,
		PlanHost:        ec.PlanHost,
		PlanPort:        ec.PlanPort,
		PlanDB:          ec.PlanDB,
		PlanUser:        ec.PlanUser,
		PlanPassword:    ec.PlanPassword,
		PlanSSLMode:     ec.PlanSSLMode,
		LockTimeout:     ec.LockTimeout,
		AutoApprove:     ec.AutoApprove,
		ApplicationName: ec.ApplicationName,
		OutputHuman:     ec.OutputHuman,
		OutputJSON:      ec.OutputJSON,
		OutputSQL:       ec.OutputSQL,
		MultiFile:       ec.MultiFile,
		NoComments:      ec.NoComments,
		NoColor:         ec.NoColor,
		Schemas:         ec.Schemas,
	}
}

var resolvedCfg *ResolvedConfig

func SetResolved(cfg *ResolvedConfig) {
	resolvedCfg = cfg
}

func Get() *ResolvedConfig {
	return resolvedCfg
}

func DiscoverSchemas(host string, port int, db, user, password, sslmode, query string) ([]string, error) {
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s sslmode=%s", host, port, db, user, sslmode)
	if password != "" {
		dsn += fmt.Sprintf(" password=%s", password)
	}
	dsn += " connect_timeout=30"

	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect for schema discovery: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run in a read-only transaction so the discovery query cannot modify data,
	// even if the config file contains a non-SELECT statement.
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin read-only transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("schema discovery query failed: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get query columns: %w", err)
	}
	if len(cols) != 1 {
		return nil, fmt.Errorf("schema discovery query must return exactly 1 column, got %d", len(cols))
	}

	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan schema name: %w", err)
		}
		schemas = append(schemas, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading schema names: %w", err)
	}

	return schemas, nil
}

// isDefined checks if a TOML key is explicitly present.
// prefix is a dot-separated path like "env.dev", key is the field name.
func isDefined(meta toml.MetaData, prefix string, key string) bool {
	var keys []string
	if prefix != "" {
		keys = strings.Split(prefix, ".")
	}
	keys = append(keys, key)
	return meta.IsDefined(keys...)
}

func mergeEnvConfig(resolved *ResolvedConfig, env *envConfig, meta toml.MetaData, prefixParts ...string) {
	prefix := strings.Join(prefixParts, ".")

	if isDefined(meta, prefix, "host") {
		resolved.Host = env.Host
	}
	if isDefined(meta, prefix, "port") {
		resolved.Port = env.Port
	}
	if isDefined(meta, prefix, "db") {
		resolved.DB = env.DB
	}
	if isDefined(meta, prefix, "user") {
		resolved.User = env.User
	}
	if isDefined(meta, prefix, "password") {
		resolved.Password = env.Password
	}
	if isDefined(meta, prefix, "sslmode") {
		resolved.SSLMode = env.SSLMode
	}
	if isDefined(meta, prefix, "schema") {
		resolved.Schema = env.Schema
	}
	if isDefined(meta, prefix, "file") {
		resolved.File = env.File
	}
	if isDefined(meta, prefix, "plan-host") {
		resolved.PlanHost = env.PlanHost
	}
	if isDefined(meta, prefix, "plan-port") {
		resolved.PlanPort = env.PlanPort
	}
	if isDefined(meta, prefix, "plan-db") {
		resolved.PlanDB = env.PlanDB
	}
	if isDefined(meta, prefix, "plan-user") {
		resolved.PlanUser = env.PlanUser
	}
	if isDefined(meta, prefix, "plan-password") {
		resolved.PlanPassword = env.PlanPassword
	}
	if isDefined(meta, prefix, "plan-sslmode") {
		resolved.PlanSSLMode = env.PlanSSLMode
	}
	if isDefined(meta, prefix, "lock-timeout") {
		resolved.LockTimeout = env.LockTimeout
	}
	if isDefined(meta, prefix, "auto-approve") {
		resolved.AutoApprove = env.AutoApprove
	}
	if isDefined(meta, prefix, "application-name") {
		resolved.ApplicationName = env.ApplicationName
	}
	if isDefined(meta, prefix, "output-human") {
		resolved.OutputHuman = env.OutputHuman
	}
	if isDefined(meta, prefix, "output-json") {
		resolved.OutputJSON = env.OutputJSON
	}
	if isDefined(meta, prefix, "output-sql") {
		resolved.OutputSQL = env.OutputSQL
	}
	if isDefined(meta, prefix, "multi-file") {
		resolved.MultiFile = env.MultiFile
	}
	if isDefined(meta, prefix, "no-comments") {
		resolved.NoComments = env.NoComments
	}
	if isDefined(meta, prefix, "no-color") {
		resolved.NoColor = env.NoColor
	}
	if isDefined(meta, prefix, "schemas") {
		resolved.Schemas = env.Schemas
	}
}
