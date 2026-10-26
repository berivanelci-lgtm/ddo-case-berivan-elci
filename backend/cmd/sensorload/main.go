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

	var (
		mu        sync.Mutex
		latencies []time.Duration
		failures  int
		events    int
		wg        sync.WaitGroup
	)
	started := time.Now()
	deadline := started.Add(*duration)

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
				body, _ := json.Marshal(batch{Readings: items})

				req, _ := http.NewRequest(http.MethodPost, *baseURL+"/sensor-readings", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)

				begin := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(begin)
				ok := err == nil && resp.StatusCode == http.StatusCreated
				if resp != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}

				mu.Lock()
				latencies = append(latencies, elapsed)
				if ok {
					events += len(items)
				} else {
					failures++
				}
				mu.Unlock()
			}
		}(station)
	}
	wg.Wait()
	total := time.Since(started)

	if len(latencies) == 0 {
		fmt.Fprintln(os.Stderr, "hiç istek gönderilmedi")
		os.Exit(1)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(p float64) time.Duration {
		index := int(math.Ceil(p*float64(len(latencies)))) - 1
		if index < 0 {
			index = 0
		}
		return latencies[index]
	}
	errorRate := 100 * float64(failures) / float64(len(latencies))

	fmt.Println("| Ölçüm | Değer |")
	fmt.Println("|---|---|")
	fmt.Printf("| İstasyon × olay/sn (hedef) | %d × %d = %d olay/sn |\n", *stations, *rate, *stations**rate)
	fmt.Printf("| Süre | %s |\n", total.Round(time.Second))
	fmt.Printf("| İstek (her biri %d okuma) | %d |\n", *rate, len(latencies))
	fmt.Printf("| Yazılan olay | %d |\n", events)
	fmt.Printf("| Gerçekleşen hız | %.0f olay/sn |\n", float64(events)/total.Seconds())
	fmt.Printf("| Hata oranı | %.2f %% (%d istek) |\n", errorRate, failures)
	fmt.Printf("| p50 | %s |\n", percentile(0.50).Round(time.Microsecond*100))
	fmt.Printf("| p95 | %s |\n", percentile(0.95).Round(time.Microsecond*100))
	fmt.Printf("| p99 | %s |\n", percentile(0.99).Round(time.Microsecond*100))
	fmt.Printf("| En yüksek | %s |\n", latencies[len(latencies)-1].Round(time.Microsecond*100))

	if failures > 0 {
		os.Exit(1)
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
