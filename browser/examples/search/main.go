// Command search runs a headless Google search through the browser engine
// and prints results. Use examples/headed-once first if the engine reports a
// challenge (landed on: .../sorry/...) — that means Google demands an
// interactive CAPTCHA for this network; solve it once headed with a shared
// profile, then this example reuses that session.
//
//	go run ./examples/search [query]
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	browser "github.com/BugraAkdemir/gosearch/browser"
)

func main() {
	query := "istanbul hava durumu"
	if len(os.Args) > 1 {
		query = os.Args[1]
	}

	ctx := context.Background()
	e, err := browser.New(ctx,
		browser.AllowDownload(true),
		browser.WithProfileDir(".gosearch-profile"),
	)
	if err != nil {
		log.Fatal(err)
	}

	results, err := e.Search(ctx, query)
	// Close explicitly on every path: a deferred Close would be skipped by
	// log.Fatalf's os.Exit below.
	_ = e.Close()
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	fmt.Printf("%d sonuç:\n\n", len(results))
	for _, r := range results {
		fmt.Printf("- %s\n  %s\n", r.Title, r.URL)
	}
}
