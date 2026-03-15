package apply

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/manish/job-auto-apply/internal/config"
	"github.com/manish/job-auto-apply/internal/db"
	"github.com/manish/job-auto-apply/internal/form"
	"github.com/playwright-community/playwright-go"
)

// Worker orchestrates the end-to-end application workflow for a single job.
// It reuses the browser page already authenticated by the Scraper.
type Worker struct {
	page    playwright.Page
	profile *config.Profile
	db      *db.DB
	logger  *slog.Logger
}

// New creates an apply Worker. The page must already be authenticated (logged in).
func New(page playwright.Page, profile *config.Profile, database *db.DB, logger *slog.Logger) *Worker {
	return &Worker{
		page:    page,
		profile: profile,
		db:      database,
		logger:  logger,
	}
}

// Apply attempts to apply to the job at the given URL.
// When dryRun is true the form is filled but the submit button is NOT clicked.
// Database status is updated on both success and failure.
func (w *Worker) Apply(job *db.Job, dryRun bool) error {
	w.logger.Info("starting application",
		"title", job.Title,
		"company", job.Company,
		"url", job.URL,
		"dry_run", dryRun,
	)

	if err := w.navigateToJob(job.URL); err != nil {
		return w.fail(job.URL, fmt.Errorf("apply: navigate: %w", err))
	}

	// Detect and dismiss any interstitial modals (cookie banners, login prompts)
	w.dismissModals()

	// Find and click the primary apply button
	clicked, err := w.clickApplyButton()
	if err != nil {
		return w.fail(job.URL, fmt.Errorf("apply: click apply button: %w", err))
	}
	if !clicked {
		w.logger.Info("no apply button found — job may be external or already applied", "url", job.URL)
		return w.db.UpdateStatus(job.URL, db.StatusSkipped)
	}

	// Wait for form or redirect to settle
	w.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(10000),
	})

	// Check for captcha — if detected, bail gracefully
	if w.isCaptchaPresent() {
		w.logger.Warn("captcha detected — skipping job", "url", job.URL)
		return w.db.UpdateStatus(job.URL, db.StatusFailed)
	}

	// Run the form filler
	filler := form.New(w.page, w.profile, w.logger)
	if err := filler.FillForm(); err != nil {
		w.logger.Warn("form fill error (continuing)", "err", err)
	}

	// Upload resume
	if err := filler.UploadResume(w.profile.ResumePath); err != nil {
		w.logger.Warn("resume upload error (continuing)", "err", err)
	}

	humanPause(800, 1200)

	if dryRun {
		w.logger.Info("DRY RUN — skipping submit", "title", job.Title)
		return w.db.UpdateStatus(job.URL, db.StatusSkipped)
	}

	// Submit the form
	if err := w.submitForm(); err != nil {
		return w.fail(job.URL, fmt.Errorf("apply: submit form: %w", err))
	}

	w.logger.Info("application submitted successfully",
		"title", job.Title,
		"company", job.Company,
	)
	return w.db.UpdateStatus(job.URL, db.StatusApplied)
}

// navigateToJob opens the job URL and waits for the page to settle.
func (w *Worker) navigateToJob(url string) error {
	if _, err := w.page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(20000),
	}); err != nil {
		return err
	}
	humanPause(600, 1000)
	return nil
}

// clickApplyButton locates and clicks the correct "Apply" button.
// Naukri has several variants: "Apply", "Easy Apply", "Apply on company site".
// Returns (true, nil) when a button was clicked, (false, nil) when none found.
func (w *Worker) clickApplyButton() (bool, error) {
	applySelectors := []string{
		`[id*="apply-button"]`,
		`button:has-text("Apply")`,
		`a:has-text("Apply")`,
		`[class*="apply-btn"]`,
		`button:has-text("Easy Apply")`,
	}

	for _, sel := range applySelectors {
		btn := w.page.Locator(sel).First()
		count, _ := btn.Count()
		if count == 0 {
			continue
		}

		if err := btn.Click(); err != nil {
			w.logger.Warn("click failed for selector", "selector", sel, "err", err)
			continue
		}

		w.logger.Info("apply button clicked", "selector", sel)
		humanPause(500, 900)
		return true, nil
	}

	return false, nil
}

// submitForm clicks the first visible submit button on the page.
func (w *Worker) submitForm() error {
	submitSelectors := []string{
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button:has-text("Submit")`,
		`button:has-text("Apply")`,
	}

	for _, sel := range submitSelectors {
		btn := w.page.Locator(sel).First()
		count, _ := btn.Count()
		if count == 0 {
			continue
		}
		if err := btn.Click(); err != nil {
			continue
		}
		w.logger.Info("form submitted", "selector", sel)
		humanPause(1000, 2000) // give the server time to process
		return nil
	}

	return fmt.Errorf("apply: no submit button found on page")
}

// dismissModals tries to close common overlay dialogs that block interactions.
func (w *Worker) dismissModals() {
	dismissSelectors := []string{
		`[class*="modal"] button[aria-label="Close"]`,
		`button:has-text("Not now")`,
		`button:has-text("Skip")`,
		`[class*="close-btn"]`,
	}

	for _, sel := range dismissSelectors {
		btn := w.page.Locator(sel).First()
		count, _ := btn.Count()
		if count > 0 {
			btn.Click()
			w.logger.Debug("dismissed modal", "selector", sel)
			humanPause(300, 500)
		}
	}
}

// isCaptchaPresent checks for common captcha indicators.
// If detected, the job is marked failed so the bot doesn't get stuck.
func (w *Worker) isCaptchaPresent() bool {
	captchaSelectors := []string{
		`iframe[src*="recaptcha"]`,
		`iframe[src*="captcha"]`,
		`[class*="g-recaptcha"]`,
		`[id*="captcha"]`,
	}

	for _, sel := range captchaSelectors {
		count, _ := w.page.Locator(sel).Count()
		if count > 0 {
			return true
		}
	}
	return false
}

// fail updates the job status to "failed" and returns the original error so the
// pipeline can continue with the next job rather than halting.
func (w *Worker) fail(url string, err error) error {
	w.logger.Error("application failed", "url", url, "err", err)
	_ = w.db.UpdateStatus(url, db.StatusFailed)
	return err
}

// humanPause introduces a random delay in the [minMs, maxMs] range.
func humanPause(minMs, maxMs int) {
	delta := maxMs - minMs
	if delta <= 0 {
		delta = 1
	}
	ms := minMs + int(time.Now().UnixNano()%int64(delta))
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
