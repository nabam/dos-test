package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	username       = os.Getenv("AUTH_USERNAME")
	password       = os.Getenv("AUTH_PASSWORD")
	url            = os.Getenv("DEST_URL")
	numCalls, _    = strconv.Atoi(os.Getenv("NUM_CALLS"))
	concurrency, _ = strconv.Atoi(os.Getenv("CONCURRENCY"))

	client = http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

func worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan struct{}, results chan<- int) {
	// log.Println("DEBUG: worker started")

	for {
		select {
		case <-jobs:
			req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
			if err != nil {
				log.Println(err)
				results <- 0
				continue
			}

			req.SetBasicAuth(username, password)
			res, err := client.Do(req)
			if err != nil {
				log.Println(err)
				results <- 0
				continue
			}

			switch res.StatusCode {
			case 403, 307:
				headers := ""
				for name, values := range res.Header {
					// Loop over all values for the name.
					for _, value := range values {
						headers = headers + fmt.Sprintf("%s: %s\n", name, value)
					}
				}

				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					fmt.Println(err)
				}
				log.Printf("error response:\n%s\n%s\n", headers, body)
				panic("gotcha")
			}
			res.Body.Close()
			results <- res.StatusCode
		case <-ctx.Done():
			// log.Println("DEBUG: worker done")
			wg.Done()
			return
		}
	}
}

func main() {
	jobs := make(chan struct{})
	results := make(chan int)
	wg := &sync.WaitGroup{}

	ctx, stop := context.WithCancel(context.Background())

	requestCount := 0

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker(ctx, wg, jobs, results)
	}

	codes := map[int]int{}
	go func() {
		// log.Println("DEBUG: watcher started")
		for a := 1; a <= numCalls; a++ {
			code := <-results
			// log.Printf("DEBUG: got result %d\n", a)
			codes[code] = codes[code] + 1
		}
		// log.Println("DEBUG: watcher done")
		stop()
	}()

	go func() {
		// log.Println("DEBUG: reporter started")
		start := time.Now()
		for {
			time.Sleep(1 * time.Second)
			now := time.Now()
			responseCount := 0
			for k, v := range codes {
				log.Printf("%d -> %d\n", k, v)
				responseCount = responseCount + v
			}
			rate := float64(requestCount) / now.Sub(start).Seconds()
			log.Printf("rate: %.2f rps; in flight: %d requests\n", rate, requestCount-responseCount)
			log.Printf("---\n")
		}
	}()

	for i := 0; i < numCalls; i++ {
		jobs <- struct{}{}
		requestCount = requestCount + 1
	}
	// log.Println("DEBUG: finishing")

	wg.Wait()
	close(jobs)

	for k, v := range codes {
		log.Printf("%d -> %d\n", k, v)
	}
}
