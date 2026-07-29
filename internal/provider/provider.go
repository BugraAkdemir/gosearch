// Package provider holds the shared type that search-engine providers return.
//
// It lives in an internal package (rather than the public gosearch package) so
// providers can be written without importing gosearch, avoiding an import cycle
// (gosearch imports the providers). The public gosearch package converts these
// into its own documented Result type.
package provider

// Result is one parsed search result, produced by a provider's Search function.
// It mirrors the public gosearch.Result field-for-field; gosearch copies these
// across so the public type's documentation and identity stay in the public
// package.
type Result struct {
	Title   string
	URL     string
	Snippet string
}
