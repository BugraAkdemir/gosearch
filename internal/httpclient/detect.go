package httpclient

import (
	"fmt"
	"strings"

	"github.com/BugraAkdemir/gosearch/internal/serrors"
)

// Detect inspects a fetched Response and reports whether the engine blocked the
// request. It returns:
//
//   - an error wrapping serrors.ErrChallenge if the response is an interactive
//     CAPTCHA / JavaScript-challenge page,
//   - an error wrapping serrors.ErrBlocked for other anti-bot rejections
//     (HTTP 429/403, generic "unusual traffic" walls),
//   - nil if the response looks like a normal, parseable page.
//
// The returned error is wrapped with a short reason so logs are useful; callers
// match it with errors.Is against the sentinels, never by string.
//
// Detection markers are documented per engine below. They are the regression
// surface guarded by the real captured pages under testdata/*/blocked.html —
// changing them requires re-checking those fixtures.
func Detect(resp *Response) error {
	if resp == nil {
		return nil
	}

	// Status-code signals apply to every engine.
	switch resp.StatusCode {
	case 429:
		return fmt.Errorf("%w: HTTP 429 Too Many Requests", serrors.ErrBlocked)
	case 403:
		return fmt.Errorf("%w: HTTP 403 Forbidden", serrors.ErrBlocked)
	}

	finalURL := strings.ToLower(resp.FinalURL)
	body := bytesToLowerString(resp.Body)

	// --- Yandex ---
	// Yandex redirects blocked requests to /showcaptcha (or showcaptchafast) and
	// sets an x-yandex-captcha header. Observed 2026-07-29.
	if resp.Header != nil && resp.Header.Get("X-Yandex-Captcha") != "" {
		return fmt.Errorf("%w: yandex x-yandex-captcha header present", serrors.ErrChallenge)
	}
	if strings.Contains(finalURL, "showcaptcha") || strings.Contains(finalURL, "/checkcaptcha") {
		return fmt.Errorf("%w: yandex captcha redirect (%s)", serrors.ErrChallenge, resp.FinalURL)
	}
	if strings.Contains(body, "smartcaptcha") {
		return fmt.Errorf("%w: yandex smartcaptcha page", serrors.ErrChallenge)
	}

	// --- DuckDuckGo ---
	// The HTML endpoint serves a captcha with status 200 and "anomaly-modal"
	// markup / "bots use DuckDuckGo too" text. Observed 2026-07-29.
	if strings.Contains(body, "anomaly-modal") ||
		strings.Contains(body, "bots use duckduckgo too") {
		return fmt.Errorf("%w: duckduckgo anomaly/captcha page", serrors.ErrChallenge)
	}

	// --- Google ---
	// /sorry/ interstitial ("unusual traffic") is a hard block; the
	// "enablejs" retry page means Google will only serve results to a JS
	// client, which for a non-JS fetch is effectively a challenge. Observed
	// 2026-07-29.
	if strings.Contains(finalURL, "/sorry/") || strings.Contains(body, "/sorry/index") {
		return fmt.Errorf("%w: google /sorry interstitial", serrors.ErrChallenge)
	}
	if strings.Contains(body, "our systems have detected unusual traffic") {
		return fmt.Errorf("%w: google unusual-traffic wall", serrors.ErrBlocked)
	}
	if strings.Contains(body, "/httpservice/retry/enablejs") {
		return fmt.Errorf("%w: google javascript-required page", serrors.ErrChallenge)
	}

	return nil
}

// bytesToLowerString lowercases b for case-insensitive marker matching. Body
// bytes are already bounded by maxBodyBytes, so this allocation is bounded too.
func bytesToLowerString(b []byte) string {
	return strings.ToLower(string(b))
}
