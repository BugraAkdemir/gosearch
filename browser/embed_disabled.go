//go:build !gosearch_embed_engine

package browser

// embeddedEngineZip is nil in default builds: resolution then falls through
// to system discovery and opt-in download. See embed_engine.go for the
// embedded variant.
var embeddedEngineZip []byte
