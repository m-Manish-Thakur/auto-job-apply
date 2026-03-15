package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/manish/job-auto-apply/internal/ai"
	apply "github.com/manish/job-auto-apply/internal/apply"
	"github.com/manish/job-auto-apply/internal/config"
	"github.com/manish/job-auto-apply/internal/db"
	"github.com/manish/job-auto-apply/internal/scraper"
)

func main() {
	// ── CLI flags ──────────────────────────────────────────────────────────────
	var (
		configPath = flag.String("config", "configs/profile.yaml", "Path to profile.yaml")
		dryRun     = flag.Bool("dry-run", false, "Fill forms but do NOT submit applications")
		limit      = flag.Int("limit", 0, "Maximum jobs to process per run (0 = unlimited)")
		threshold  = flag.Int("threshold", 0, "Override match threshold (0 = use profile value)")
		headless   = flag.Bool("headless", false, "Run browser in headless mode")
		verbose    = flag.Bool("verbose", false, "Enable debug-level logging")
	)
	flag.Parse()

	// ── Logger ────────────────────────────────────────────────────────────────
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// ── Load .env ─────────────────────────────────────────────────────────────
	if err := godotenv.Load(); err != nil {
		logger.Warn(".env file not found — falling back to system environment variables")
	}

	naukriEmail := requireEnv("NAUKRI_EMAIL")
	naukriPass := requireEnv("NAUKRI_PASSWORD")
	ollamaURL := envOr("OLLAMA_BASE_URL", "http://localhost:11434")
	ollamaModel := envOr("OLLAMA_MODEL", "llama3")
	dbPath := envOr("DB_PATH", "data/jobs.db")

	// ── Load profile ──────────────────────────────────────────────────────────
	profile, err := config.LoadProfile(*configPath)
	if err != nil {
		logger.Error("failed to load profile", "path", *configPath, "err", err)
		os.Exit(1)
	}
	logger.Info("profile loaded", "name", profile.Name, "roles", profile.ExpectedRolesString())

	// Apply CLI overrides
	matchThreshold := float64(profile.MatchThreshold)
	if *threshold > 0 {
		matchThreshold = float64(*threshold)
		logger.Info("match threshold overridden via flag", "threshold", matchThreshold)
	}

	// ── Ensure data directory exists ──────────────────────────────────────────
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		logger.Error("cannot create data directory", "err", err)
		os.Exit(1)
	}

	// ── Database ──────────────────────────────────────────────────────────────
	database, err := db.New(dbPath, logger)
	if err != nil {
		logger.Error("database init failed", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	// ── AI Matcher ────────────────────────────────────────────────────────────
	matcher := ai.New(ollamaURL, ollamaModel, logger)
	logger.Info("AI matcher ready", "model", ollamaModel, "url", ollamaURL)

	// ── Browser / Scraper ─────────────────────────────────────────────────────
	sc, err := scraper.New(logger, *headless)
	if err != nil {
		logger.Error("scraper init failed", "err", err)
		os.Exit(1)
	}
	defer sc.Close()

	if err := sc.Login(naukriEmail, naukriPass); err != nil {
		logger.Error("Naukri login failed", "err", err)
		os.Exit(1)
	}

	// ── Job Discovery ─────────────────────────────────────────────────────────
	listings, err := sc.SearchJobs(
		profile.ExpectedRoles,
		profile.PreferredLocations,
		profile.MaxSearchPages,
	)
	if err != nil {
		logger.Error("job search failed", "err", err)
		os.Exit(1)
	}
	logger.Info("discovery complete", "total_listings", len(listings))

	// ── Apply Worker ──────────────────────────────────────────────────────────
	worker := apply.New(sc.Page(), profile, database, logger)

	processed := 0
	applied := 0
	skipped := 0
	failed := 0

	for _, listing := range listings {
		if *limit > 0 && processed >= *limit {
			logger.Info("job limit reached", "limit", *limit)
			break
		}

		// Deduplication — skip jobs already in the database
		exists, err := database.ExistsURL(listing.URL)
		if err != nil {
			logger.Warn("dedup check error", "url", listing.URL, "err", err)
		}
		if exists {
			logger.Debug("job already processed, skipping", "url", listing.URL)
			continue
		}

		processed++
		logger.Info("processing job", "title", listing.Title, "company", listing.Company)

		// Save with status "pending" so we have a record even if later steps fail
		jobRecord := &db.Job{
			Title:   listing.Title,
			Company: listing.Company,
			URL:     listing.URL,
			Status:  db.StatusPending,
		}
		if err := database.SaveJob(jobRecord); err != nil {
			logger.Warn("save job error", "err", err)
		}

		// Extract full job description
		description, err := sc.ExtractJobDescription(listing.URL)
		if err != nil {
			logger.Warn("JD extraction failed", "url", listing.URL, "err", err)
			database.UpdateStatus(listing.URL, db.StatusFailed)
			failed++
			continue
		}
		jobRecord.Description = description

		// AI match evaluation
		result, err := matcher.EvaluateMatch(profile, listing.Title, description)
		if err != nil {
			logger.Warn("AI eval failed", "url", listing.URL, "err", err)
			database.UpdateStatus(listing.URL, db.StatusFailed)
			failed++
			continue
		}

		database.UpdateMatchScore(listing.URL, result.MatchScore)
		logger.Info("match scored",
			"title", listing.Title,
			"score", result.MatchScore,
			"threshold", matchThreshold,
			"reason", result.Reason,
		)

		if result.MatchScore < matchThreshold {
			logger.Info("score below threshold — skipping", "score", result.MatchScore, "threshold", matchThreshold)
			database.UpdateStatus(listing.URL, db.StatusSkipped)
			skipped++
			continue
		}

		// Apply to the job
		jobRecord.MatchScore = result.MatchScore
		if err := worker.Apply(jobRecord, *dryRun); err != nil {
			logger.Warn("application worker error", "url", listing.URL, "err", err)
			failed++
			continue
		}
		applied++
	}

	// ── Run Summary ───────────────────────────────────────────────────────────
	stats, _ := database.Stats()
	fmt.Println()
	logger.Info("=== RUN COMPLETE ===",
		"processed", processed,
		"applied", applied,
		"skipped", skipped,
		"failed", failed,
		"db_stats", stats,
	)
}

// requireEnv reads an environment variable and exits if it is empty.
func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "ERROR: required environment variable %q is not set\n", key)
		fmt.Fprintf(os.Stderr, "Copy .env.example to .env and fill in your values.\n")
		os.Exit(1)
	}
	return v
}

// envOr returns the env value or a fallback default.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
