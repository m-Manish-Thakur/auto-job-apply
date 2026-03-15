package ai

import (
	"strings"
	"testing"

	"github.com/manish/job-auto-apply/internal/config"
)

func TestBuildPrompt_ContainsCandidateSkills(t *testing.T) {
	profile := &config.Profile{
		Name:              "Test User",
		Email:             "test@example.com",
		Phone:             "1234567890",
		Location:          "Bangalore",
		YearsOfExperience: 3,
		Skills:            []string{"Go", "PostgreSQL", "Docker"},
		ExpectedRoles:     []string{"Backend Developer"},
		ResumePath:        "resume/resume.pdf",
		MatchThreshold:    60,
	}

	m := New("http://localhost:11434", "llama3", nil)
	prompt := m.buildPrompt(profile, "Golang Backend Engineer", "We need a backend developer with Go and Docker.")

	checks := []string{
		"Go, PostgreSQL, Docker",
		"3 years",
		"Bangalore",
		"Golang Backend Engineer",
		"match_score",
		"reason",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt missing expected content %q\n\nFull prompt:\n%s", check, prompt)
		}
	}
}

func TestStripMarkdownFences(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    "```json\n{\"match_score\":75,\"reason\":\"good fit\"}\n```",
			expected: `{"match_score":75,"reason":"good fit"}`,
		},
		{
			input:    "{\"match_score\":80,\"reason\":\"excellent\"}",
			expected: `{"match_score":80,"reason":"excellent"}`,
		},
		{
			input:    "```\n{\"match_score\":50,\"reason\":\"partial\"}\n```",
			expected: `{"match_score":50,"reason":"partial"}`,
		},
	}

	for _, tc := range cases {
		got := stripMarkdownFences(tc.input)
		if got != tc.expected {
			t.Errorf("stripMarkdownFences(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.expected)
		}
	}
}
