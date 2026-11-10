package main

import (
	"log"
	"net/http"
	"os"
)

func address() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}

func main() {
	log.Printf("Listening on %s", address())
	log.Fatal(http.ListenAndServe(address(), nil))
}
