//go:build gosearch_embed_engine

package browser

import _ "embed"

// embeddedEngineZip holds the chrome-headless-shell archive compiled into
// the binary when this file's build tag is active. The archive is NOT stored
// in git; produce it before building with:
//
//	go run ./tools/fetch-engine -out engine/chrome-headless-shell.zip
//
// Without the file present, builds tagged gosearch_embed_engine fail at
// compile time with go:embed's missing-file error — deliberate, so a
// silently engine-less binary can never ship.
//
//go:embed engine/chrome-headless-shell.zip
var embeddedEngineZip []byte
