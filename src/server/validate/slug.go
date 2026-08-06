package validate

import (
	"fmt"
	"regexp"
	"strings"
)

// Organization slug rules per AI.md PART 35 (Org Slug Validation).
var orgSlugRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidateOrgSlug validates an organization slug per AI.md PART 35.
//
// Rules: 2-39 characters, lowercase alphanumeric and hyphens only, must
// start and end with an alphanumeric character, no consecutive hyphens, and
// not a reserved name. Users and orgs share a namespace, so the org reserved
// list is the shared UsernameBlocklist. Cross-collision against existing
// usernames/org slugs is enforced separately at the data layer.
func ValidateOrgSlug(slug string) error {
	slug = strings.ToLower(strings.TrimSpace(slug))

	if len(slug) < 2 || len(slug) > 39 {
		return fmt.Errorf("slug must be 2-39 characters")
	}
	if !orgSlugRegex.MatchString(slug) {
		return fmt.Errorf("slug must be lowercase letters, numbers, and hyphens, starting and ending with a letter or number")
	}
	if strings.Contains(slug, "--") {
		return fmt.Errorf("slug cannot contain consecutive hyphens")
	}
	for _, blocked := range UsernameBlocklist {
		if slug == strings.ToLower(blocked) {
			return fmt.Errorf("slug is a reserved name: %s", blocked)
		}
	}

	return nil
}
