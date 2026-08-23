package main

import (
	"context"
	"fmt"

	browser "github.com/BugraAkdemir/gosearch/browser"
)

func main() {
	e, err := browser.New(context.Background(), browser.AllowDownload(true))
	if err != nil {
		panic(err)
	}
	defer e.Close()

	rs, err := e.Search(context.Background(), "istanbul hava durumu")
	if err != nil {
		fmt.Println("engellendi:", err)
		return
	}
	for _, r := range rs {
		fmt.Println(r.Title, "->", r.URL)
	}
}
