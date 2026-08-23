// Package e2e exercises the public gosearch API against the REAL provider
// packages — the only place where the full dispatch → provider → Detect
// chain can be observed without hitting the live engines. Provider endpoint
// vars are pointed at local httptest servers serving captured block pages and
// synthetic success fixtures, so the fallback chain's documented behavior is
// confirmed end-to-end while staying offline and deterministic.
package e2e
