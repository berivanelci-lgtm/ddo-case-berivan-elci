package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// healthResponse, /health ucunun döndüğü JSON yapısıdır.
type healthResponse struct {
	Status string `json:"status"`
}

// healthHandler, servisin ayakta olduğunu bildirir.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)

	log.Println("API :8080 portunda dinliyor")
	log.Fatal(http.ListenAndServe(":8080", mux))
}