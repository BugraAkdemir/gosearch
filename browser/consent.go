package browser

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// pageState is what pageStateJS returns.
type pageState struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// dismissConsentIfNeeded clears Google's cookie-consent interstitial when the
// tab landed on one (consent.google.com). Clicking "Accept" is exactly what
// an ordinary visitor does — this is normal UI interaction, not defeating a
// security control; CAPTCHAs are never touched. Best-effort by design: any
// failure leaves the page as-is and Search's own error path reports it.
func dismissConsentIfNeeded(ctx context.Context) error {
	dctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	var raw string
	if err := chromedp.Run(dctx, chromedp.Evaluate(pageStateJS, &raw)); err != nil {
		return nil //nolint:nilerr // diagnostics-only step
	}
	var st pageState
	if json.Unmarshal([]byte(raw), &st) != nil || !consentNeedsHandling(st.URL) {
		return nil
	}

	var clicked bool
	if err := chromedp.Run(dctx, chromedp.Evaluate(consentAcceptClickJS, &clicked)); err != nil || !clicked {
		return nil //nolint:nilerr // nothing clickable matched; let Search report the wall
	}
	// The click submits a form that redirects back to the results page.
	return chromedp.Run(dctx, chromedp.WaitVisible("h3", chromedp.ByQuery))
}

// consentNeedsHandling reports whether a landed-on state is Google's
// cookie-consent interstitial rather than an actual results page.
func consentNeedsHandling(landedURL string) bool {
	return strings.Contains(strings.ToLower(landedURL), "consent.google.")
}
