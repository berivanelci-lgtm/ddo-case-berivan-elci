// Command simulator, bir üretim hattını taklit edip API'ye sürekli veri gönderir.
// Her adımda: SPC ölçümü (her 200 adımlık döngünün ilk 60 adımı kararlı, sonrası bilerek
// yukarı kayan trend), her istasyondan bir sensör okuması ve bir hat sayacı. Ayrıca ara sıra
// arıza, 30 dakika gecikmeyle bildirilen duruş ve bilerek tekrar gönderilen olaylar üretir.
// Aynı SIM_SEED aynı değer dizisini üretir; zaman damgaları çalıştırma anına bağlıdır.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	operatorEmail    = "operator@hatkontrol.test"
	qualityEmail     = "kalite@hatkontrol.test"
	plannerEmail     = "planlama@hatkontrol.test"
	maintenanceEmail = "bakim@hatkontrol.test"
	tokenLifetime    = 10 * time.Minute
	heartbeatFile    = "/tmp/heartbeat"
)

type config struct {
	apiURL   string
	password string
	seed     int64
	interval time.Duration
	stations int
}

func loadConfig() config {
	cfg := config{
		apiURL:   getEnv("API_URL", "http://localhost:8080"),
		password: getEnv("DEMO_PASSWORD", "Demo!2026Parola"),
		seed:     42,
		interval: 5 * time.Second,
		stations: 50,
	}
	if n, err := strconv.ParseInt(os.Getenv("SIM_SEED"), 10, 64); err == nil {
		cfg.seed = n
	}
	if d, err := time.ParseDuration(os.Getenv("SIM_INTERVAL")); err == nil && d > 0 {
		cfg.interval = d
	}
	if n, err := strconv.Atoi(os.Getenv("SIM_STATIONS")); err == nil && n >= 1 && n <= 1000 {
		cfg.stations = n
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type cachedToken struct {
	value  string
	issued time.Time
}

// apiClient, demo hesaplarla giriş yapıp API'ye istek gönderir; token'ları 10 dakika saklar.
type apiClient struct {
	http     *http.Client
	base     string
	password string
	tokens   map[string]cachedToken
}

func (c *apiClient) token(email string) (string, error) {
	if t, ok := c.tokens[email]; ok && time.Since(t.issued) < tokenLifetime {
		return t.value, nil
	}
	body, _ := json.Marshal(map[string]string{"email": email, "password": c.password})
	resp, err := c.http.Post(c.base+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s girişi başarısız: %d", email, resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	c.tokens[email] = cachedToken{value: out.AccessToken, issued: time.Now()}
	return out.AccessToken, nil
}

// call, isteği verilen kullanıcı adına gönderir; durum kodunu ve yanıt gövdesini döner.
func (c *apiClient) call(method, path, email string, payload any) (int, []byte, error) {
	tok, err := c.token(email)
	if err != nil {
		return 0, nil, err
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, err
}

// ensureChart, simülatörün SPC grafiğini bulur ya da oluşturur ve kimliğini döner.
func ensureChart(c *apiClient, name string) (int64, error) {
	find := func() (int64, error) {
		status, data, err := c.call(http.MethodGet, "/charts", operatorEmail, nil)
		if err != nil {
			return 0, err
		}
		if status != http.StatusOK {
			return 0, fmt.Errorf("grafikler okunamadı: %d", status)
		}
		var charts []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &charts); err != nil {
			return 0, err
		}
		for _, chart := range charts {
			if chart.Name == name {
				return chart.ID, nil
			}
		}
		return 0, nil
	}

	if id, err := find(); err != nil || id != 0 {
		return id, err
	}
	status, data, err := c.call(http.MethodPost, "/charts", qualityEmail, map[string]any{"name": name, "type": "individual"})
	if err != nil {
		return 0, err
	}
	switch status {
	case http.StatusCreated:
		var chart struct {
			ID int64 `json:"id"`
		}
		err := json.Unmarshal(data, &chart)
		return chart.ID, err
	case http.StatusConflict:
		return find()
	default:
		return 0, fmt.Errorf("grafik oluşturulamadı: %d %s", status, data)
	}
}

// ensureLine, simülatörün hattını yoksa oluşturur.
func ensureLine(c *apiClient, code string) error {
	status, data, err := c.call(http.MethodPost, "/lines", plannerEmail,
		map[string]any{"code": code, "name": "Simulasyon hatti", "ideal_cycle_ms": 6000})
	if err != nil {
		return err
	}
	if status != http.StatusCreated && status != http.StatusConflict {
		return fmt.Errorf("hat oluşturulamadı: %d %s", status, data)
	}
	return nil
}

// simulation, tohumla deterministik değer üreten simülasyon durumudur.
type simulation struct {
	rng       *rand.Rand
	stations  int
	tick      int
	lastCount map[string]any
	sent      int
	failed    int
}

func newSimulation(seed int64, stations int) *simulation {
	return &simulation{rng: rand.New(rand.NewSource(seed)), stations: stations}
}

// measurement, SPC ölçümünü üretir: her 200 adımlık döngünün ilk 60 adımı kararlıdır,
// sonrası bilerek yukarı kayan bir trenddir (kontrolden çıkan süreç).
func (s *simulation) measurement() float64 {
	value := 10 + s.rng.NormFloat64()*0.5
	if phase := s.tick % 200; phase > 60 {
		value += 0.05 * float64(phase-60)
	}
	return value
}

func (s *simulation) step(c *apiClient, chartID int64, lineCode string, now time.Time) {
	s.tick++

	s.send(c, fmt.Sprintf("/charts/%d/measurements", chartID), operatorEmail,
		map[string]any{"values": []float64{s.measurement()}})

	readings := make([]map[string]any, 0, s.stations)
	for station := 1; station <= s.stations; station++ {
		readings = append(readings, map[string]any{
			"station_id":  station,
			"recorded_at": now,
			"value":       50 + 0.1*float64(station) + s.rng.NormFloat64(),
		})
	}
	s.send(c, "/sensor-readings", operatorEmail, map[string]any{"readings": readings})

	count := map[string]any{"occurred_at": now, "good": 1, "scrap": 0}
	if s.rng.Float64() < 0.05 {
		count["good"], count["scrap"] = 0, 1
	}
	s.send(c, "/lines/"+lineCode+"/counts", operatorEmail, count)
	if s.tick%15 == 0 && s.lastCount != nil {
		// Tekrar eden olay: önceki sayaç bilerek yeniden gönderilir, OEE'de iki kez sayılmamalı.
		s.send(c, "/lines/"+lineCode+"/counts", operatorEmail, s.lastCount)
	}
	s.lastCount = count

	if s.tick%40 == 0 {
		breakdown := map[string]any{"reason_code": "BREAKDOWN", "started_at": now.Add(-2 * time.Minute), "ended_at": now}
		s.send(c, "/lines/"+lineCode+"/downtime", maintenanceEmail, breakdown)
		s.send(c, "/lines/"+lineCode+"/downtime", maintenanceEmail, breakdown) // tekrar eden olay
	}
	if s.tick%60 == 0 {
		// Gecikmeli olay: 30 dakika önce biten bir duruş şimdi bildirilir.
		late := map[string]any{"reason_code": "MATERIAL_WAIT", "started_at": now.Add(-35 * time.Minute), "ended_at": now.Add(-30 * time.Minute)}
		s.send(c, "/lines/"+lineCode+"/downtime", maintenanceEmail, late)
	}

	if s.tick%12 == 0 {
		slog.Info("simülasyon adımı", "tick", s.tick, "sent", s.sent, "failed", s.failed)
	}
}

func (s *simulation) send(c *apiClient, path, email string, payload any) {
	status, data, err := c.call(http.MethodPost, path, email, payload)
	switch {
	case err != nil:
		s.failed++
		slog.Warn("istek başarısız", "path", path, "error", err)
	case status >= 300:
		s.failed++
		slog.Warn("istek reddedildi", "path", path, "status", status, "body", string(data))
	default:
		s.sent++
	}
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := loadConfig()
	client := &apiClient{
		http:     &http.Client{Timeout: 10 * time.Second},
		base:     cfg.apiURL,
		password: cfg.password,
		tokens:   make(map[string]cachedToken),
	}

	chartName := fmt.Sprintf("SIM-SPC-%d", cfg.seed)
	lineCode := fmt.Sprintf("SIM-HAT-%d", cfg.seed)

	var chartID int64
	for {
		id, err := ensureChart(client, chartName)
		if err == nil {
			err = ensureLine(client, lineCode)
		}
		if err == nil {
			chartID = id
			break
		}
		slog.Warn("hazırlık başarısız, tekrar denenecek", "error", err)
		time.Sleep(3 * time.Second)
	}
	slog.Info("simülatör başladı", "seed", cfg.seed, "chart", chartName, "line", lineCode,
		"stations", cfg.stations, "interval", cfg.interval.String())

	sim := newSimulation(cfg.seed, cfg.stations)
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		sim.step(client, chartID, lineCode, time.Now().UTC())
		_ = os.WriteFile(heartbeatFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
		<-ticker.C
	}
}
