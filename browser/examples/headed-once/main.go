// Command headed-once opens a VISIBLE browser window with a persistent
// profile so YOU can manually clear an interactive challenge (CAPTCHA /
// consent) one time; the saved session then carries over to headless runs of
// examples/search using the same profile directory. The library never solves
// challenges itself — this is the human-in-the-loop escape hatch.
//
// Requires a windowed Chromium-family browser (system Chrome/Chromium/Edge).
// Note: the downloaded chrome-headless-shell is headless-only, so if your
// machine has no system browser, install one first (e.g. on Arch:
// sudo pacman -S chromium).
//
//	go run ./examples/headed-once
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	browser "github.com/BugraAkdemir/gosearch/browser"
)

func main() {
	ctx := context.Background()

	e, err := browser.New(ctx,
		browser.AllowDownload(true),
		browser.WithProfileDir(".gosearch-profile"),
		browser.WithHeadless(false),
	)
	if err != nil {
		log.Fatal(err)
	}

	results, err := e.Search(ctx, "istanbul hava durumu")
	if err != nil {
		fmt.Println("pencerede challenge'ı ELLE çöz; sonra bu örneği tekrar çalıştır.")
		fmt.Println("hata:", err)
		_ = e.Close()
		return
	}
	fmt.Printf("temiz geçti: %d sonuç. Oturum profilde saklandı.\n", len(results))
	time.Sleep(3 * time.Second)
	_ = e.Close()
}
