package main

import (
	"fmt"
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

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, World!")
	})

	log.Printf("Listening on %s", address())
	log.Fatal(http.ListenAndServe(address(), mux))
}
