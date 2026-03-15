package scraper

import (
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// JobListing is the minimal data extracted from a Naukri search result page.
type JobListing struct {
	Title   string
	Company string
	URL     string
}

// Scraper manages a single Playwright browser session for Naukri.
type Scraper struct {
	pw      *playwright.Playwright
	browser playwright.Browser
	page    playwright.Page
	logger  *slog.Logger
	headless bool
}

// New initialises Playwright, launches Chromium, and returns a ready Scraper.
// headless controls whether the browser window is visible. Set to false during
// development so you can watch interactions; true for unattended runs.
func New(logger *slog.Logger, headless bool) (*Scraper, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("scraper: start playwright: %w", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
		// Slow motion for human-like pace (milliseconds per action)
		SlowMo: playwright.Float(120),
		Args: []string{
			"--no-sandbox",
			"--disable-blink-features=AutomationControlled",
		},
	})
	if err != nil {
		pw.Stop()
		return nil, fmt.Errorf("scraper: launch chromium: %w", err)
	}

	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String(
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
				"AppleWebKit/537.36 (KHTML, like Gecko) " +
				"Chrome/123.0.0.0 Safari/537.36",
		),
		Viewport: &playwright.Size{Width: 1366, Height: 768},
	})
	if err != nil {
		browser.Close()
		pw.Stop()
		return nil, fmt.Errorf("scraper: new browser context: %w", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		browser.Close()
		pw.Stop()
		return nil, fmt.Errorf("scraper: new page: %w", err)
	}

	return &Scraper{
		pw:       pw,
		browser:  browser,
		page:     page,
		logger:   logger,
		headless: headless,
	}, nil
}

// Close tears down the browser and Playwright runtime. Always defer this.
func (s *Scraper) Close() {
	s.browser.Close()
	s.pw.Stop()
}

// Page exposes the underlying Playwright page so the apply worker can reuse
// the same browser session (and therefore the same login cookies).
func (s *Scraper) Page() playwright.Page {
	return s.page
}

// Login authenticates with Naukri using email and password.
// It waits for the dashboard to confirm success before returning.
func (s *Scraper) Login(email, password string) error {
	s.logger.Info("navigating to Naukri login")

	if _, err := s.page.Goto("https://www.naukri.com/nlogin/login", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return fmt.Errorf("scraper: navigate to login: %w", err)
	}

	humanDelay(500, 900)

	// Fill email
	if err := s.page.Locator(`input[id="usernameField"]`).Fill(email); err != nil {
		return fmt.Errorf("scraper: fill email: %w", err)
	}
	humanDelay(300, 600)

	// Fill password
	if err := s.page.Locator(`input[id="passwordField"]`).Fill(password); err != nil {
		return fmt.Errorf("scraper: fill password: %w", err)
	}
	humanDelay(400, 700)

	// Click login button
	if err := s.page.Locator(`button[type="submit"]`).Click(); err != nil {
		return fmt.Errorf("scraper: click login: %w", err)
	}

	// Wait until the user menu appears — sign the login succeeded
	if err := s.page.Locator(`[class*="nI-gNb-drawer"]`).WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(15000),
	}); err != nil {
		return fmt.Errorf("scraper: login timed out — check credentials: %w", err)
	}

	s.logger.Info("login successful", "email", email)
	return nil
}

// SearchJobs iterates Naukri search result pages for each role and location
// combination and returns a de-duplicated slice of JobListings.
func (s *Scraper) SearchJobs(roles, locations []string, maxPages int) ([]JobListing, error) {
	seen := make(map[string]bool)
	var results []JobListing

	for _, role := range roles {
		for page := 1; page <= maxPages; page++ {
			listings, err := s.scrapePage(role, locations, page)
			if err != nil {
				s.logger.Warn("error scraping page", "role", role, "page", page, "err", err)
				break // stop paginating this role on error
			}
			if len(listings) == 0 {
				s.logger.Info("no more results", "role", role, "page", page)
				break
			}

			for _, l := range listings {
				if !seen[l.URL] {
					seen[l.URL] = true
					results = append(results, l)
				}
			}
			s.logger.Info("scraped page", "role", role, "page", page, "jobs_found", len(listings))
			humanDelay(1500, 3000) // be polite between pages
		}
	}

	s.logger.Info("job discovery complete", "total_unique_listings", len(results))
	return results, nil
}

// scrapePage loads one Naukri search results page and returns job listings.
func (s *Scraper) scrapePage(role string, locations []string, page int) ([]JobListing, error) {
	url := buildSearchURL(role, locations, page)
	s.logger.Debug("loading search page", "url", url)

	if _, err := s.page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(20000),
	}); err != nil {
		return nil, fmt.Errorf("scraper: goto search page: %w", err)
	}

	// Scroll down to trigger lazy loading
	s.page.Evaluate(`window.scrollBy(0, 600)`)
	humanDelay(800, 1200)

	// Job cards on new Naukri UI share class "cust-job-tuple"
	cards := s.page.Locator(`article.jobTuple, .cust-job-tuple, [class*="jobTuple"]`)
	count, err := cards.Count()
	if err != nil || count == 0 {
		return nil, nil // empty page — caller will stop pagination
	}

	var listings []JobListing
	for i := 0; i < count; i++ {
		card := cards.Nth(i)

		titleEl := card.Locator(`a.title, a[class*="title"]`).First()
		title, _ := titleEl.InnerText()
		href, _ := titleEl.GetAttribute("href")

		companyEl := card.Locator(`a[class*="comp-name"], .companyInfo a`).First()
		company, _ := companyEl.InnerText()

		if href == "" || title == "" {
			continue
		}

		listings = append(listings, JobListing{
			Title:   strings.TrimSpace(title),
			Company: strings.TrimSpace(company),
			URL:     normalizeURL(href),
		})
	}

	return listings, nil
}

// ExtractJobDescription opens a job URL and returns the full description text.
// The caller is responsible for rate limiting between calls.
func (s *Scraper) ExtractJobDescription(url string) (string, error) {
	if _, err := s.page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(20000),
	}); err != nil {
		return "", fmt.Errorf("scraper: goto job page %q: %w", url, err)
	}

	humanDelay(600, 1000)

	// Naukri wraps the JD in a section with class "job-desc" or "jd-desc"
	descEl := s.page.Locator(`.job-desc, .jd-desc, [class*="job-desc"]`).First()
	text, err := descEl.InnerText()
	if err != nil {
		// Fallback: grab everything inside the main article tag
		text, err = s.page.Locator(`main, article`).First().InnerText()
		if err != nil {
			return "", fmt.Errorf("scraper: extract description from %q: %w", url, err)
		}
	}

	description := strings.TrimSpace(text)
	if len(description) < 50 {
		return "", fmt.Errorf("scraper: description too short for %q — possible bot detection", url)
	}

	return description, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// buildSearchURL constructs a Naukri search URL for the given role and locations.
func buildSearchURL(role string, locations []string, page int) string {
	// Naukri uses hyphen-separated slug paths: "golang-developer-jobs-in-bangalore"
	slug := strings.ToLower(strings.ReplaceAll(role, " ", "-"))
	loc := ""
	if len(locations) > 0 {
		l := strings.ToLower(strings.ReplaceAll(locations[0], " ", "-"))
		loc = "-in-" + l
	}

	base := fmt.Sprintf("https://www.naukri.com/%s-jobs%s", slug, loc)
	if page > 1 {
		return fmt.Sprintf("%s-%d", base, page)
	}
	return base
}

// normalizeURL makes sure relative Naukri URLs become absolute.
func normalizeURL(href string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	return "https://www.naukri.com" + href
}

// humanDelay sleeps for a random duration between minMs and maxMs milliseconds
// to mimic human browsing speed and reduce bot detection risk.
func humanDelay(minMs, maxMs int) {
	delta := maxMs - minMs
	if delta <= 0 {
		delta = 1
	}
	ms := minMs + rand.Intn(delta)
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
