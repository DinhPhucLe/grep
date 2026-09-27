package db

import (
	"fmt"
	"sort"
	"strings"
)

// RequiredCollectionsForVersion lists application collections that must exist
// after a clean migration to the given version (schema_migrations itself excluded).
func RequiredCollectionsForVersion(version uint) []string {
	var required []string
	if version >= 1 {
		required = append(required, "users")
	}
	if version >= 2 {
		required = append(required, "projects")
	}
	if version >= 3 {
		required = append(required, "sessions")
	}
	if version >= 4 {
		required = append(required, "evaluations")
	}
	if version >= 6 {
		required = append(required, "organizations", "organization_members", "practice_events")
	}
	if version >= 7 {
		required = append(required, "knowledge_records")
	}
	if version >= 8 {
		required = append(required, "knowledge_documents")
	}
	return required
}

// MissingCollectionsForVersion returns required collection names not present in have.
func MissingCollectionsForVersion(version uint, have []string) []string {
	present := make(map[string]bool, len(have))
	for _, name := range have {
		present[name] = true
	}
	var missing []string
	for _, name := range RequiredCollectionsForVersion(version) {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

// SchemaIntegrityError explains a stamped migration version that lacks collections.
func SchemaIntegrityError(version uint, missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	prev := int(version) - 1
	if prev < 0 {
		prev = 0
	}
	return fmt.Errorf(
		"migration version %d is recorded but missing collections [%s]; force schema_migrations.version to %d with dirty=false, then re-run migrate",
		version, strings.Join(missing, ", "), prev,
	)
}
