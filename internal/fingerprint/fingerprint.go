package fingerprint

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/pgplex/pgschema/ir"
)

// SchemaFingerprint represents a fingerprint of a database schema state
type SchemaFingerprint struct {
	Hash string `json:"hash"` // SHA256 of normalized IR
}

// ComputeFingerprint generates a fingerprint for the given IR and schema
func ComputeFingerprint(schemaIR *ir.IR, schemaName string) (*SchemaFingerprint, error) {
	return ComputeFingerprintForSchemas(schemaIR, []string{schemaName})
}

// ComputeFingerprintForSchemas hashes the union of the listed PostgreSQL namespaces.
func ComputeFingerprintForSchemas(schemaIR *ir.IR, schemaNames []string) (*SchemaFingerprint, error) {
	if len(schemaNames) == 0 {
		schemaNames = []string{"public"}
	}
	if len(schemaNames) == 1 {
		return computeFingerprintSingle(schemaIR, schemaNames[0])
	}

	subset := make(map[string]*ir.Schema)
	for _, name := range schemaNames {
		if s := schemaIR.Schemas[name]; s != nil {
			subset[name] = s
		}
	}
	partial := &ir.IR{
		Metadata: schemaIR.Metadata,
		Schemas:  subset,
	}
	hash, err := hashObject(partial)
	if err != nil {
		return nil, fmt.Errorf("failed to compute schema hash: %w", err)
	}
	return &SchemaFingerprint{Hash: hash}, nil
}

func computeFingerprintSingle(schemaIR *ir.IR, schemaName string) (*SchemaFingerprint, error) {
	// Get the target schema, default to "public" if not found
	targetSchema := schemaIR.Schemas[schemaName]
	if targetSchema == nil && schemaName == "public" {
		// Handle case where public schema might not exist in IR
		for _, schema := range schemaIR.Schemas {
			if schema.Name == "public" {
				targetSchema = schema
				break
			}
		}
	}

	// Compute hash of the entire target schema
	hash, err := hashObject(targetSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to compute schema hash: %w", err)
	}

	return &SchemaFingerprint{
		Hash: hash,
	}, nil
}

// hashObject computes a SHA256 hash of any object
func hashObject(obj interface{}) (string, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// String returns a human-readable representation of the fingerprint
func (f *SchemaFingerprint) String() string {
	if len(f.Hash) >= 8 {
		return fmt.Sprintf("Schema fingerprint: %s", f.Hash[:8])
	}
	return fmt.Sprintf("Schema fingerprint: %s", f.Hash)
}
