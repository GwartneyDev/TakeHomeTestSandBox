package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

// Semaphore to limit the number of concurrent goroutines
type Semaphore struct {
	ch chan struct{}
}

func NewSemaphore(limit int) *Semaphore {
	return &Semaphore{
		ch: make(chan struct{}, limit),
	}
}

func (s *Semaphore) Acquire() {
	s.ch <- struct{}{}
}

func (s *Semaphore) Release() {
	<-s.ch
}

// Location struct to hold parsed data
type Location struct {
	URL string `json:"location"`
}

// PayLoad struct to hold response data
type PayLoad struct {
	Data string `json:"data"`
	Buf  *bytes.Buffer
}

// ValidateURL ensures the URL is valid and contains a scheme
func ValidateURL(input string) (string, error) {
	u, err := url.Parse(input)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		u.Scheme = "https" // Default to HTTPS if no scheme
	}
	return u.String(), nil
}

// Create a base HTTP request
func createBaseRequest(ctx context.Context, payload PayLoad) (*http.Request, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error marshalling JSON: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://bar.com", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// Goroutine function to process a location
func processLocation(semaphore *Semaphore, wg *sync.WaitGroup, loc Location, baseRequest *http.Request, client *http.Client) {
	defer wg.Done()           // Ensure the counter is decremented when the goroutine completes
	semaphore.Acquire()       // Acquire a semaphore slot to control concurrency
	defer semaphore.Release() // Release the semaphore slot when done

	// Validate the URL
	validatedURL, err := ValidateURL(loc.URL)
	if err != nil {
		log.Printf("Invalid URL: %s", loc.URL)
		return
	}

	if validatedURL == "https://bar.com" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Set a 5-second timeout
		defer cancel()                                                          // Ensure the context is canceled

		// Clone the base request and update the URL
		req := baseRequest.Clone(ctx)
		req.URL, _ = url.Parse(validatedURL) // Update the URL in the cloned request

		res, err := client.Do(req) // Send the POST request
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				log.Printf("Request to %s timed out", validatedURL)
			} else {
				log.Printf("Error sending request: %v", err)
			}
			return
		}

		defer res.Body.Close() // Ensure the response body is closed

		// Read the response
		payload := &PayLoad{Buf: new(bytes.Buffer)}
		if _, err := io.Copy(payload.Buf, res.Body); err != nil {
			log.Printf("Error copying response body: %v", err)
			return
		}

		fmt.Printf("Received data: %s\n", payload.Buf.String())
	}
}

func main() {
	// Read locations from a file
	data, err := os.ReadFile("./input.txt")
	if err != nil {
		log.Fatal("Error reading file:", err)
	}

	var locations []Location
	if err = json.Unmarshal(data, &locations); err != nil {
		log.Fatal("Error parsing JSON:", err)
	}

	var wg sync.WaitGroup         // Use a WaitGroup to synchronize goroutines
	semaphore := NewSemaphore(10) // Limit to 10 concurrent goroutines

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	// Create a base HTTP request
	baseRequest, err := createBaseRequest(context.Background(), PayLoad{Data: "example data"})
	if err != nil {
		log.Fatal("Error creating base request:", err)
	}

	for _, loc := range locations {
		wg.Add(1)
		go processLocation(semaphore, &wg, loc, baseRequest, client) // Start a new goroutine for each location
	}

	wg.Wait() // Wait for all goroutines to complete

}
