package httpclient

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/BugraAkdemir/gosearch/internal/serrors"
)

func readFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	// testdata lives at the module root; from internal/httpclient that is ../..
	full := filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read fixture %v: %v", parts, err)
	}
	return b
}

func TestDetectDuckDuckGoCaptchaFixture(t *testing.T) {
	body := readFixture(t, "duckduckgo", "blocked.html")
	// DuckDuckGo serves the captcha with status 200.
	resp := &Response{StatusCode: 200, FinalURL: "https://html.duckduckgo.com/html/", Body: body}
	err := Detect(resp)
	if !errors.Is(err, serrors.ErrChallenge) {
		t.Fatalf("Detect(ddg captcha) = %v, want ErrChallenge", err)
	}
}

func TestDetectGoogleEnableJSFixture(t *testing.T) {
	body := readFixture(t, "google", "blocked.html")
	resp := &Response{StatusCode: 200, FinalURL: "https://www.google.com/search?q=facebook", Body: body}
	err := Detect(resp)
	if !errors.Is(err, serrors.ErrChallenge) {
		t.Fatalf("Detect(google enablejs) = %v, want ErrChallenge", err)
	}
}

func TestDetectYandexCaptchaRedirect(t *testing.T) {
	// Yandex signals via a redirect to showcaptchafast and an x-yandex-captcha
	// header. No body is needed to detect it.
	resp := &Response{
		StatusCode: 200,
		FinalURL:   "https://yandex.com/showcaptchafast?d=abc&retpath=xyz",
		Header:     http.Header{"X-Yandex-Captcha": {"captcha"}},
	}
	if err := Detect(resp); !errors.Is(err, serrors.ErrChallenge) {
		t.Fatalf("Detect(yandex header) = %v, want ErrChallenge", err)
	}

	// Redirect URL alone (no header) must also be caught.
	resp2 := &Response{StatusCode: 200, FinalURL: "https://yandex.com/showcaptchafast?d=abc"}
	if err := Detect(resp2); !errors.Is(err, serrors.ErrChallenge) {
		t.Fatalf("Detect(yandex redirect) = %v, want ErrChallenge", err)
	}
}

func TestDetectStatusCodes(t *testing.T) {
	if err := Detect(&Response{StatusCode: 429}); !errors.Is(err, serrors.ErrBlocked) {
		t.Errorf("Detect(429) = %v, want ErrBlocked", err)
	}
	if err := Detect(&Response{StatusCode: 403}); !errors.Is(err, serrors.ErrBlocked) {
		t.Errorf("Detect(403) = %v, want ErrBlocked", err)
	}
}

func TestDetectCleanPageReturnsNil(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		FinalURL:   "https://html.duckduckgo.com/html/",
		Body:       []byte(`<html><body><div class="result__body">ok</div></body></html>`),
	}
	if err := Detect(resp); err != nil {
		t.Errorf("Detect(clean page) = %v, want nil", err)
	}
}

func TestDetectNilResponse(t *testing.T) {
	if err := Detect(nil); err != nil {
		t.Errorf("Detect(nil) = %v, want nil", err)
	}
}
