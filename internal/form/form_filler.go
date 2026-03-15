package form

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/manish/job-auto-apply/internal/config"
	"github.com/playwright-community/playwright-go"
)

// FormFiller detects and fills form fields on the current Playwright page
// using heuristic label-to-profile-field mapping.
type FormFiller struct {
	page    playwright.Page
	profile *config.Profile
	logger  *slog.Logger
}

// New creates a FormFiller bound to a specific page and candidate profile.
func New(page playwright.Page, profile *config.Profile, logger *slog.Logger) *FormFiller {
	return &FormFiller{page: page, profile: profile, logger: logger}
}

// FillForm scans the page for fillable inputs and fills them where a matching
// profile field can be resolved. Unknown fields are logged and skipped.
func (f *FormFiller) FillForm() error {
	f.logger.Info("form filler started")

	selectors := []string{
		`input:not([type="hidden"]):not([type="file"]):not([type="submit"]):not([type="button"])`,
		`textarea`,
	}

	for _, sel := range selectors {
		elems, err := f.page.Locator(sel).All()
		if err != nil {
			f.logger.Warn("locator error", "selector", sel, "err", err)
			continue
		}

		for _, el := range elems {
			label := f.resolveLabel(el)
			value := resolveValue(label, f.profile)
			if value == "" {
				f.logger.Debug("no mapping for field", "label", label)
				continue
			}
			if err := el.Fill(value); err != nil {
				f.logger.Warn("fill failed", "label", label, "err", err)
				continue
			}
			f.logger.Info("filled field", "label", label, "value", value)
		}
	}

	if err := f.fillDropdowns(); err != nil {
		f.logger.Warn("dropdown fill error", "err", err)
	}

	return nil
}

// UploadResume finds the first file input on the page and sets the resume path.
func (f *FormFiller) UploadResume(resumePath string) error {
	fileInput := f.page.Locator(`input[type="file"]`).First()

	count, err := fileInput.Count()
	if err != nil || count == 0 {
		f.logger.Info("no file upload input found on this page")
		return nil
	}

	if err := fileInput.SetInputFiles(resumePath); err != nil {
		return fmt.Errorf("form: upload resume from %q: %w", resumePath, err)
	}

	f.logger.Info("resume uploaded", "path", resumePath)
	return nil
}

// resolveLabel extracts human-readable label text from a form element.
// Resolution order: aria-label → <label for="id"> → placeholder → name attribute.
func (f *FormFiller) resolveLabel(el playwright.Locator) string {
	if v, err := el.GetAttribute("aria-label"); err == nil && v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	if id, err := el.GetAttribute("id"); err == nil && id != "" {
		if label := f.page.Locator(fmt.Sprintf(`label[for="%s"]`, id)).First(); label != nil {
			if text, err := label.InnerText(); err == nil && text != "" {
				return strings.ToLower(strings.TrimSpace(text))
			}
		}
	}
	if v, err := el.GetAttribute("placeholder"); err == nil && v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	if v, err := el.GetAttribute("name"); err == nil && v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	return ""
}

// fillDropdowns attempts to select experience-related <select> options.
func (f *FormFiller) fillDropdowns() error {
	selects, err := f.page.Locator(`select`).All()
	if err != nil {
		return err
	}

	yearString := fmt.Sprintf("%d", f.profile.YearsOfExperience)

	for _, sel := range selects {
		label := f.resolveLabel(sel)
		if !strings.Contains(label, "year") && !strings.Contains(label, "experience") {
			continue
		}
		if _, err := sel.SelectOption(playwright.SelectOptionValues{
			Values: &[]string{yearString},
		}); err != nil {
			_, _ = sel.SelectOption(playwright.SelectOptionValues{
				Labels: &[]string{yearString},
			})
		}
		f.logger.Info("dropdown filled", "label", label, "value", yearString)
	}

	return nil
}
