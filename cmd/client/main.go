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
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(resp.Status)
	fmt.Println(string(body))
}
