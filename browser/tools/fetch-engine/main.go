// Command fetch-engine downloads the stable chrome-headless-shell archive
// for the current platform into engine/chrome-headless-shell.zip so a build
// with -tags gosearch_embed_engine can embed it. It is a developer tool, not
// part of the library.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/BugraAkdemir/gosearch/browser"
)

func main() {
	out := flag.String("out", "", "output zip path (default: engine/chrome-headless-shell.zip next to this module)")
	flag.Parse()

	if *out == "" {
		*out = filepath.Join("engine", "chrome-headless-shell.zip")
	}
	if err := run(*out); err != nil {
		log.Fatalf("fetch-engine: %v", err)
	}
	fmt.Printf("fetch-engine: wrote %s\n", *out)
}

func run(out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	ctx := context.Background()
	version, url, err := browser.StableEngineAsset(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("fetch-engine: chrome-headless-shell %s (%s)\n", version, browser.CurrentPlatform())
	tmp := out + ".part"
	if err := browser.DownloadFile(ctx, url, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, out)
}
