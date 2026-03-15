package form

import (
	"fmt"
	"strings"

	"github.com/manish/job-auto-apply/internal/config"
)

// fieldMapping maps lowercase label keywords to Profile field getter functions.
// Order matters — more specific phrases should come before generic ones.
var fieldMapping = []struct {
	keywords []string
	getValue func(p *config.Profile) string
}{
	{
		keywords: []string{"full name", "your name", "name"},
		getValue: func(p *config.Profile) string { return p.Name },
	},
	{
		keywords: []string{"email", "e-mail", "mail id"},
		getValue: func(p *config.Profile) string { return p.Email },
	},
	{
		keywords: []string{"mobile", "phone", "contact number", "cell"},
		getValue: func(p *config.Profile) string { return p.Phone },
	},
	{
		keywords: []string{"year", "experience", "work experience", "total exp"},
		getValue: func(p *config.Profile) string {
			return fmt.Sprintf("%d", p.YearsOfExperience)
		},
	},
	{
		keywords: []string{"location", "city", "current location", "preferred location"},
		getValue: func(p *config.Profile) string { return p.Location },
	},
	{
		keywords: []string{"skill", "key skill", "technology"},
		getValue: func(p *config.Profile) string { return p.SkillsString() },
	},
}

// resolveValue maps a label string to a profile field value.
// Returns empty string when no mapping is found.
func resolveValue(label string, profile *config.Profile) string {
	for _, entry := range fieldMapping {
		for _, kw := range entry.keywords {
			if strings.Contains(label, kw) {
				return entry.getValue(profile)
			}
		}
	}
	return ""
}

// ResolveFieldKey is exported for unit testing. It returns the first keyword
// from the matched mapping entry for the given label text.
func ResolveFieldKey(label string) string {
	lower := strings.ToLower(label)
	for _, entry := range fieldMapping {
		for _, kw := range entry.keywords {
			if strings.Contains(lower, kw) {
				return entry.keywords[0]
			}
		}
	}
	return ""
}
