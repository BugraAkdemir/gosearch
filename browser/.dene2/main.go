package main

import (
   "context"
   "fmt"
   "time"

   browser "github.com/BugraAkdemir/gosearch/browser"
)

func main() {
   e, err := browser.New(context.Background(),
      browser.AllowDownload(true),
      browser.WithProfileDir("/home/bugra/.gosearch-profile"),
      browser.WithHeadless(false),
   )
   if err != nil { panic(err) }
   rs, err := e.Search(context.Background(), "istanbul hava durumu")
   fmt.Println("sonuç:", len(rs), "hata:", err)
   if err == nil { time.Sleep(10 * time.Second) } // oturumu otursun diye bekle
   e.Close()
}
