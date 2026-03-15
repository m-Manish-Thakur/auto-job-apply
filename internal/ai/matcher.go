package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/manish/job-auto-apply/internal/config"
)

// MatchResult is the structured response from the local LLM.
type MatchResult struct {
	MatchScore float64 `json:"match_score"`
	Reason     string  `json:"reason"`
}

// Matcher evaluates job-to-profile relevance using a locally running Ollama model.
type Matcher struct {
	baseURL    string
	model      string
	httpClient *http.Client
	logger     *slog.Logger
}

// New creates a Matcher pointed at the Ollama API.
// baseURL is typically http://localhost:11434.
// model is the Ollama model tag, e.g. "llama3".
func New(baseURL, model string, logger *slog.Logger) *Matcher {
	return &Matcher{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // LLMs can be slow; generous timeout
		},
		logger: logger,
	}
}

// EvaluateMatch sends the candidate profile and job description to the local LLM
// and returns a MatchResult. It retries up to 3 times on JSON parse failure.
func (m *Matcher) EvaluateMatch(profile *config.Profile, jobTitle, jobDescription string) (*MatchResult, error) {
	prompt := m.buildPrompt(profile, jobTitle, jobDescription)

	const maxRetries = 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := m.callOllama(prompt)
		if err == nil {
			m.logger.Info("AI match evaluated",
				"job", jobTitle,
				"score", result.MatchScore,
				"reason", result.Reason,
			)
			return result, nil
		}
		m.logger.Warn("AI call failed, retrying",
			"attempt", attempt,
			"max", maxRetries,
			"err", err,
		)
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}

	return nil, fmt.Errorf("ai: EvaluateMatch failed after %d attempts", maxRetries)
}

// buildPrompt constructs the system + user prompt sent to the LLM.
// Exported for unit testing.
func (m *Matcher) buildPrompt(profile *config.Profile, jobTitle, jobDescription string) string {
	var sb strings.Builder

	sb.WriteString("You are an expert technical recruiter with 15 years of experience.\n\n")
	sb.WriteString("Evaluate whether the following job matches the candidate profile.\n\n")

	sb.WriteString("=== CANDIDATE PROFILE ===\n")
	fmt.Fprintf(&sb, "Name:            %s\n", profile.Name)
	fmt.Fprintf(&sb, "Experience:      %d years\n", profile.YearsOfExperience)
	fmt.Fprintf(&sb, "Skills:          %s\n", profile.SkillsString())
	fmt.Fprintf(&sb, "Expected Roles:  %s\n", profile.ExpectedRolesString())
	fmt.Fprintf(&sb, "Location:        %s\n", profile.Location)

	sb.WriteString("\n=== JOB LISTING ===\n")
	fmt.Fprintf(&sb, "Title:           %s\n", jobTitle)
	fmt.Fprintf(&sb, "Description:\n%s\n", jobDescription)

	sb.WriteString("\n=== INSTRUCTIONS ===\n")
	sb.WriteString("Score the match from 0 to 100 based on:\n")
	sb.WriteString("- Skill overlap (most important)\n")
	sb.WriteString("- Experience level fit\n")
	sb.WriteString("- Role title alignment\n\n")
	sb.WriteString("Respond with ONLY valid JSON. No markdown, no explanation outside the JSON.\n")
	sb.WriteString(`{"match_score": <number 0-100>, "reason": "<one sentence explanation>"}`)

	return sb.String()
}

// ollamaRequest is the payload sent to POST /api/generate.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format"` // "json" instructs Ollama to enforce JSON output
}

// ollamaResponse is the envelope returned by Ollama (non-streaming).
type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// callOllama sends the prompt to Ollama and parses the MatchResult from the response.
func (m *Matcher) callOllama(prompt string) (*MatchResult, error) {
	reqBody, err := json.Marshal(ollamaRequest{
		Model:  m.model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal request: %w", err)
	}

	resp, err := m.httpClient.Post(
		m.baseURL+"/api/generate",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("ai: POST /api/generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai: ollama returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("ai: decode ollama envelope: %w", err)
	}

	// The model may wrap the JSON in markdown code fences — strip them.
	raw := stripMarkdownFences(ollamaResp.Response)

	var result MatchResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("ai: parse match JSON %q: %w", raw, err)
	}

	// Clamp score to [0, 100] in case the model hallucinates out-of-range values
	if result.MatchScore < 0 {
		result.MatchScore = 0
	}
	if result.MatchScore > 100 {
		result.MatchScore = 100
	}

	return &result, nil
}

// stripMarkdownFences removes ```json ... ``` or ``` ... ``` wrappers that
// some models add despite explicit instructions.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) == 2 {
			s = lines[1]
		}
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}
