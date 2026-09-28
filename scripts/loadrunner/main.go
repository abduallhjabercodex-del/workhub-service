package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: loadrunner <url>")
		os.Exit(1)
	}
	url := os.Args[1]
	concurrency := 50
	requestsPerWorker := 50
	totalRequests := concurrency * requestsPerWorker

	var wg sync.WaitGroup
	durations := make([]time.Duration, totalRequests)
	var errorsCount int
	var mu sync.Mutex

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		},
	}

	startAll := time.Now()
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				idx := workerID*requestsPerWorker + j
				reqStart := time.Now()
				resp, err := client.Get(url)
				reqDuration := time.Since(reqStart)

				if err != nil || resp.StatusCode >= 500 {
					mu.Lock()
					errorsCount++
					mu.Unlock()
				}
				if resp != nil {
					_ = resp.Body.Close()
				}
				durations[idx] = reqDuration
			}
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startAll)

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[int(float64(totalRequests)*0.50)]
	p95 := durations[int(float64(totalRequests)*0.95)]
	p99 := durations[int(float64(totalRequests)*0.99)]
	rps := float64(totalRequests) / totalDuration.Seconds()
	errorRate := (float64(errorsCount) / float64(totalRequests)) * 100.0

	fmt.Printf("Total Requests: %d\n", totalRequests)
	fmt.Printf("Total Duration: %v\n", totalDuration)
	fmt.Printf("Throughput (RPS): %.2f req/sec\n", rps)
	fmt.Printf("Latency p50: %v\n", p50)
	fmt.Printf("Latency p95: %v (Threshold < 500ms)\n", p95)
	fmt.Printf("Latency p99: %v\n", p99)
	fmt.Printf("Error Rate: %.2f%% (Threshold < 1.0%%)\n", errorRate)

	if p95 > 500*time.Millisecond {
		fmt.Println("❌ THRESHOLD VIOLATED: p95 latency exceeded 500ms")
		os.Exit(1)
	}
	if errorRate > 1.0 {
		fmt.Println("❌ THRESHOLD VIOLATED: error rate exceeded 1%")
		os.Exit(1)
	}
	fmt.Println("✓ THRESHOLDS VERIFIED: p95 < 500ms and error rate < 1.0%")
}
