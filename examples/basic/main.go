// Command basic demonstrates gosearch's public API: a DuckDuckGo search and a
// Fetch of one of the results. It exists purely as a runnable, hand-verifiable
// check of the API — it hits the live web and is not part of the test suite.
//
// Run it with:
//
//	go run ./examples/basic
//
// Note: search engines run anti-bot systems that are more likely to block
// requests from datacenter/cloud IPs than from an ordinary residential
// network — see AGENTS.md's Known Pitfalls if this returns ErrBlocked or
// ErrChallenge.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/BugraAkdemir/gosearch"
)

func main() {
	ctx := context.Background()

	results, err := gosearch.Search(ctx, "facebook", gosearch.DuckDuckGo)
	if err != nil {
		if errors.Is(err, gosearch.ErrBlocked) || errors.Is(err, gosearch.ErrChallenge) {
			log.Fatalf("search blocked by anti-bot system: %v", err)
		}
		log.Fatalf("search failed: %v", err)
	}

	fmt.Printf("found %d results:\n\n", len(results))
	for _, r := range results {
		fmt.Println(r.Title)
		fmt.Println(r.URL)
		fmt.Println(r.Snippet)
		fmt.Println()
	}

	if len(results) == 0 {
		return
	}

	page, err := gosearch.Fetch(ctx, results[0].URL)
	if err != nil {
		log.Fatalf("fetch failed: %v", err)
	}

	fmt.Println("--- fetched page ---")
	fmt.Println(page.Title)
	fmt.Println(page.Content)
}
