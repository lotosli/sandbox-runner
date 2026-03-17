package main

import (
	"log"
	"net/http"

	"github.com/lotosli/sandbox-runner/examples/go-http-service/internal/handler"
	"github.com/lotosli/sandbox-runner/pkg/helper"
)

func main() {
	mux := http.NewServeMux()
	mux.Handle("/health", helper.WrapHTTPHandler(http.HandlerFunc(handler.Health)))
	log.Println("go-http-service listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
