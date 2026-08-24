# Getting Started

Everything you need to go from zero to your first search result and first
extracted page. For the complete option-by-option reference see
[API.md](./API.md); for task-oriented copy-paste solutions see
[RECIPES.md](./RECIPES.md); for how the library works inside see
[ARCHITECTURE.md](./ARCHITECTURE.md).

## Requirements

- Go 1.25+ (`go.mod` directive).
- No API keys, no accounts, no config files. The only third-party dependency
  of the core module is `golang.org/x/net/html`.

## Install

```bash
go get github.com/BugraAkdemir/gosearch@latest
```

## Your first search (30 seconds)

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/BugraAkdemir/gosearch"
)

func main() {
	ctx := context.Background()

	results, err := gosearch.Search(ctx, "facebook", gosearch.DuckDuckGo)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Printf("%s\n  %s\n  %s\n\n", r.Title, r.URL, r.Snippet)
	}
}
```

That's it. Under the hood this sent a single HTTP GET with realistic browser
headers to DuckDuckGo's no-JavaScript HTML endpoint, parsed the returned HTML
into structured results, and decoded each redirect wrapper into the real
destination URL. No JavaScript ran anywhere; nothing was downloaded beyond
the result page itself.

## Your first page extraction

```go
page, err := gosearch.Fetch(ctx, "https://en.wikipedia.org/wiki/Facebook")
if err != nil {
	log.Fatal(err)
}
fmt.Println(page.Title)   // best-guess article title
fmt.Println(page.Content) // main text only — nav/ads/footer stripped
```

`Fetch` returns extracted readable content, never raw HTML. It does not run
JavaScript, so pages that render entirely client-side come back with an
empty `Content` — when you hit that, see [the browser engine](./RECIPES.md#when-plain-http-is-not-enough-the-browser-engine).

## Which engine should I use?

| Engine | Status | Use it when |
|---|---|---|
| `gosearch.DuckDuckGo` | Validated against a real captured page | You want the most reliable default |
| `gosearch.Bing` | Best-effort heuristic | You want Bing-specific results or a tolerant fallback engine |
| `gosearch.Google` | Best-effort heuristic | You need Google-specific results |
| `gosearch.Yandex` | Best-effort heuristic | You need Yandex-specific results |

Reliability depends heavily on **your network's IP reputation**, not just the
engine. From a typical residential connection all of them usually work; from
datacenter/cloud IPs every engine may challenge or block you regardless of
how polite the client is. See
[Reliability, honestly](../README.md#reliability-honestly) in the README.

## Make it robust in one line

The single most useful upgrade over the basic call is a fallback chain:

```go
results, err := gosearch.Search(ctx, "facebook", gosearch.Google,
	gosearch.WithFallback(gosearch.DuckDuckGo, gosearch.Yandex),
)
```

If Google challenges or blocks the request, the same query automatically
continues on DuckDuckGo, then Yandex. First success wins; if everything is
blocked you get one joined error describing what each engine did. Details
and error-handling patterns: [RECIPES.md](./RECIPES.md).

## Run the bundled example

```bash
git clone https://github.com/BugraAkdemir/gosearch
cd gosearch
go run ./examples/basic    # hits the live web
```

## Tests never touch the network

```bash
go test -race ./...
```

is fully offline and deterministic — engines are exercised through recorded
HTML fixtures, not live requests. The optional browser subpackage follows the
same rule plus a separately-tagged integration test:

```bash
cd browser && go test -race -tags integration ./...   # needs Chrome installed
```
