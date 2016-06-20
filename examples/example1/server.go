package main

import (
	"net/http"

	_ "github.com/dustywilson/coedit"
)

func main() {
	http.Handle("/", http.FileServer(http.Dir("www")))
	http.ListenAndServe(":7654", nil)
}
