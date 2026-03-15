package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Profile holds all candidate information loaded from profile.yaml.
// Every field maps directly to a YAML key of the same snake_case name.
type Profile struct {
	// Personal info
	Name     string `yaml:"name"`
	Email    string `yaml:"email"`
	Phone    string `yaml:"phone"`
	Location string `yaml:"location"`

	// Experience and skills
	YearsOfExperience int      `yaml:"years_of_experience"`
	Skills            []string `yaml:"skills"`

	// Job search preferences
	ExpectedRoles      []string `yaml:"expected_roles"`
	PreferredLocations []string `yaml:"preferred_locations"`

	// File path to the candidate's resume PDF (relative to project root)
	ResumePath string `yaml:"resume_path"`

	// AI matching threshold (0–100). Jobs below this score are skipped.
	MatchThreshold int `yaml:"match_threshold"`

	// How many search result pages to scrape per role
	MaxSearchPages int `yaml:"max_search_pages"`
}

// LoadProfile reads a YAML profile file and returns a validated Profile.
// path is the file system path to the YAML file.
func LoadProfile(path string) (*Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: cannot open profile at %q: %w", path, err)
	}
	defer f.Close()

	var p Profile
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true) // reject unknown keys to catch typos
	if err := decoder.Decode(&p); err != nil {
		return nil, fmt.Errorf("config: invalid YAML in %q: %w", path, err)
	}

	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("config: validation failed: %w", err)
	}

	return &p, nil
}

// validate ensures all required fields are present and values are in range.
func (p *Profile) validate() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.Email == "" {
		return fmt.Errorf("email is required")
	}
	if p.Phone == "" {
		return fmt.Errorf("phone is required")
	}
	if len(p.Skills) == 0 {
		return fmt.Errorf("at least one skill is required")
	}
	if len(p.ExpectedRoles) == 0 {
		return fmt.Errorf("at least one expected_role is required")
	}
	if p.ResumePath == "" {
		return fmt.Errorf("resume_path is required")
	}
	if p.MatchThreshold < 0 || p.MatchThreshold > 100 {
		return fmt.Errorf("match_threshold must be between 0 and 100, got %d", p.MatchThreshold)
	}
	if p.MaxSearchPages <= 0 {
		p.MaxSearchPages = 3 // sensible default
	}
	return nil
}

// SkillsString returns the candidate's skills as a comma-separated string,
// suitable for embedding directly in LLM prompts.
func (p *Profile) SkillsString() string {
	result := ""
	for i, s := range p.Skills {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

// ExpectedRolesString returns roles as a comma-separated string.
func (p *Profile) ExpectedRolesString() string {
	result := ""
	for i, r := range p.ExpectedRoles {
		if i > 0 {
			result += ", "
		}
		result += r
	}
	return result
}
