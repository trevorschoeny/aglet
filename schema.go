package main

import "fmt"

// checkSchemaCompatibility validates that each calls edge connects compatible schemas.
// For every A → B edge: every field required by B's schema.in must be present
// in A's schema.out.properties, and their types must be compatible.
//
// This is a structural check — it catches mismatched field names and type conflicts
// before they cause silent data errors at runtime.
//
// Skips pairs where either schema is nil (caught by checkBlockFiles) or where
// neither side has required fields (stub schemas from `aglet new` fall into this
// category — they're placeholders, not errors).
func checkSchemaCompatibility(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Build name → block lookup for resolving calls edges
	blockByName := map[string]*DiscoveredBlock{}
	for _, b := range inv.Blocks {
		blockByName[b.Config.Name] = b
	}

	for _, upstream := range inv.Blocks {
		for _, callRef := range upstream.Config.Calls {
			// Resolve domain-qualified names: "domain/BlockName" → "BlockName"
			downstreamName := callRef
			for i := len(callRef) - 1; i >= 0; i-- {
				if callRef[i] == '/' {
					downstreamName = callRef[i+1:]
					break
				}
			}

			downstream, ok := blockByName[downstreamName]
			if !ok {
				// Already caught by checkCallsEdges — skip here
				continue
			}

			// Both schemas must be non-nil to proceed
			if upstream.Config.Schema.Out == nil || downstream.Config.Schema.In == nil {
				continue
			}

			// Extract the downstream's required fields — if none, nothing to check
			required := extractRequired(downstream.Config.Schema.In)
			if len(required) == 0 {
				continue
			}

			// If the upstream output is fully open (additionalProperties: true),
			// any downstream input is satisfied — skip
			if schemaIsOpen(upstream.Config.Schema.Out) {
				continue
			}

			// Get the upstream output's declared properties
			outProps := extractProperties(upstream.Config.Schema.Out)

			// Check each required downstream field against the upstream output
			for _, field := range required {
				outField, exists := outProps[field]
				if !exists {
					errors = append(errors, ValidationError{
						Unit: upstream.Config.Name,
						Message: fmt.Sprintf(
							"schema mismatch with '%s': output is missing field '%s' required by %s.schema.in",
							downstreamName, field, downstreamName,
						),
					})
					continue
				}

				// If both sides declare a type for this field, check compatibility
				upstreamType := extractType(outField)
				downstreamType := extractType(extractProperties(downstream.Config.Schema.In)[field])
				if upstreamType != "" && downstreamType != "" {
					if !typesCompatible(upstreamType, downstreamType) {
						errors = append(errors, ValidationError{
							Unit: upstream.Config.Name,
							Message: fmt.Sprintf(
								"schema type mismatch with '%s': field '%s' is %s in output but %s is required by %s.schema.in",
								downstreamName, field, upstreamType, downstreamType, downstreamName,
							),
						})
					}
				}
			}
		}
	}

	return errors
}

// --- Schema Navigation Helpers ---
// All helpers operate on interface{} because schemas are stored as parsed YAML
// (map[string]interface{}). Nil and unexpected types are handled gracefully.

// extractProperties returns the "properties" map from a JSON Schema object.
// Returns an empty map if the schema has no properties declared.
func extractProperties(schema interface{}) map[string]interface{} {
	m, ok := schema.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	props, ok := m["properties"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return props
}

// extractRequired returns the list of required field names from a JSON Schema object.
// Returns nil if no required fields are declared.
func extractRequired(schema interface{}) []string {
	m, ok := schema.(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := m["required"]
	if !ok {
		return nil
	}
	// YAML unmarshals arrays as []interface{}, not []string
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	var result []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// extractType returns the "type" string from a JSON Schema field definition.
// Returns empty string if no type is declared.
func extractType(fieldSchema interface{}) string {
	m, ok := fieldSchema.(map[string]interface{})
	if !ok {
		return ""
	}
	t, _ := m["type"].(string)
	return t
}

// schemaIsOpen returns true if the schema explicitly allows additional properties
// (additionalProperties: true), meaning any downstream input is satisfied.
func schemaIsOpen(schema interface{}) bool {
	m, ok := schema.(map[string]interface{})
	if !ok {
		return false
	}
	ap, ok := m["additionalProperties"]
	if !ok {
		return false
	}
	b, ok := ap.(bool)
	return ok && b
}

// typesCompatible reports whether an upstream field type can satisfy a downstream
// field type requirement. Uses JSON Schema type widening rules:
//   - Same type is always compatible
//   - integer satisfies number (integer is a numeric subtype)
//   - number does NOT satisfy integer (could be fractional)
//   - All other type mismatches are incompatible
func typesCompatible(upstream, downstream string) bool {
	if upstream == downstream {
		return true
	}
	// integer is a subtype of number — an integer output satisfies a number input
	if upstream == "integer" && downstream == "number" {
		return true
	}
	return false
}
