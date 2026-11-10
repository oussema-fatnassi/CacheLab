package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	resp, err := client.Get("http://localhost:" + port + "/")
	if err != nil {
		log.Fatal(err)
	}

	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()

	if err != nil {
		log.Fatal(err)
	}
	if closeErr != nil {
		log.Printf("closing response body: %v", closeErr)
	}

	fmt.Println(resp.Status)
	fmt.Println(string(body))
}
