// Command sensorload, çalışan API'ye istasyon başına saniyede belirli sayıda sensör okuması
// gönderir ve yazma isteklerinin p50/p95/p99 gecikmesini ve hata oranını ölçer.
// Okuma değerleri tohumla deterministik üretilir.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

type reading struct {
	StationID  int       `json:"station_id"`
	RecordedAt time.Time `json:"recorded_at"`
	Value      float64   `json:"value"`
}

type batch struct {
	Readings []reading `json:"readings"`
}

type collector struct {
	mu        sync.Mutex
	latencies []time.Duration
	failures  int
	events    int
	reasons   map[string]int
}

func (c *collector) record(elapsed time.Duration, written int, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latencies = append(c.latencies, elapsed)
	if reason == "" {
		c.events += written
		return
	}
	c.failures++
	c.reasons[reason]++
}

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "API adresi")
	email := flag.String("email", "operator@hatkontrol.test", "giriş e-postası")
	password := flag.String("password", "Demo!2026Parola", "giriş parolası")
	stations := flag.Int("stations", 50, "istasyon sayısı")
	rate := flag.Int("rate", 10, "istasyon başına saniyedeki olay sayısı")
	duration := flag.Duration("duration", 60*time.Second, "test süresi")
	seed := flag.Int64("seed", 42, "değer üretimi için tohum")
	flag.Parse()

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{MaxIdleConns: 200, MaxIdleConnsPerHost: 200},
	}
	token, err := login(client, *baseURL, *email, *password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "giriş başarısız:", err)
		os.Exit(1)
	}

	results := &collector{reasons: make(map[string]int)}
	started := time.Now()
	deadline := started.Add(*duration)

	var wg sync.WaitGroup
	for station := 1; station <= *stations; station++ {
		wg.Add(1)
		go func(station int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(*seed + int64(station)))
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for tick := 0; ; tick++ {
				now := <-ticker.C
				if now.After(deadline) {
					return
				}
				items := make([]reading, *rate)
				for i := range items {
					items[i] = reading{
						StationID:  station,
						RecordedAt: now.Add(-time.Second + time.Duration(i)*time.Second/time.Duration(*rate)).UTC(),
						Value:      50 + 0.01*float64(tick) + rng.NormFloat64(),
					}
				}
				elapsed, reason := send(client, *baseURL, token, batch{Readings: items})
				results.record(elapsed, len(items), reason)
			}
		}(station)
	}
	wg.Wait()
	total := time.Since(started)

	report(results, total, *stations, *rate)
	if results.failures > 0 {
		os.Exit(1)
	}
}

// send, bir toplu isteği gönderir; başarısızsa nedenini döner.
func send(client *http.Client, baseURL, token string, payload batch) (time.Duration, string) {
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/sensor-readings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	begin := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(begin)
	if err != nil {
		return elapsed, "transport: " + err.Error()
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return elapsed, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return elapsed, ""
}

func report(c *collector, total time.Duration, stations, rate int) {
	if len(c.latencies) == 0 {
		fmt.Fprintln(os.Stderr, "hiç istek gönderilmedi")
		os.Exit(1)
	}
	sort.Slice(c.latencies, func(i, j int) bool { return c.latencies[i] < c.latencies[j] })
	percentile := func(p float64) time.Duration {
		index := int(math.Ceil(p*float64(len(c.latencies)))) - 1
		if index < 0 {
			index = 0
		}
		return c.latencies[index].Round(100 * time.Microsecond)
	}

	fmt.Println("| Olcum | Deger |")
	fmt.Println("|---|---|")
	fmt.Printf("| Hedef | %d istasyon x %d olay/sn = %d olay/sn |\n", stations, rate, stations*rate)
	fmt.Printf("| Sure | %s |\n", total.Round(time.Second))
	fmt.Printf("| Istek (her biri %d okuma) | %d |\n", rate, len(c.latencies))
	fmt.Printf("| Yazilan olay | %d |\n", c.events)
	fmt.Printf("| Gerceklesen hiz | %.0f olay/sn |\n", float64(c.events)/total.Seconds())
	fmt.Printf("| Hata orani | %.2f %% (%d istek) |\n", 100*float64(c.failures)/float64(len(c.latencies)), c.failures)
	fmt.Printf("| p50 | %s |\n", percentile(0.50))
	fmt.Printf("| p95 | %s |\n", percentile(0.95))
	fmt.Printf("| p99 | %s |\n", percentile(0.99))
	fmt.Printf("| En yuksek | %s |\n", c.latencies[len(c.latencies)-1].Round(100*time.Microsecond))
	for reason, count := range c.reasons {
		fmt.Printf("| Hata nedeni: %s | %d |\n", reason, count)
	}
}

func login(client *http.Client, baseURL, email, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	resp, err := client.Post(baseURL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("durum %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.AccessToken, nil
}
