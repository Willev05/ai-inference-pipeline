package main

import (
	"ai-ingerence-pipeline/internal/cache"
	"ai-ingerence-pipeline/pkg/protocol"
	"encoding/json"
	"log"
	"net/http"
	"os"
)

var redisCache *cache.Cache

type Config struct {
	Port          string
	ModelEndpoint string
	RedisHost     string
}

// Gets the environment value from the key, if not present, returns the fallback.
func getEnv(key string, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// Returns a config struct filled from the environment variables.
func loadConfig() Config {
	return Config{
		Port:          getEnv("PORT", "8080"),
		ModelEndpoint: getEnv("MODEL_ENDPOINT", "http://localhost:11434/api/generate"),
		RedisHost:     getEnv("REDIS_HOST", "localhost:6379"),
	}
}

// Replies to an http request with the specified error code and message
func replyWithError(w http.ResponseWriter, code int, message string) {
	errorStruct := struct {
		Error string `json:"error"`
	}{
		Error: message,
	}

	json.NewEncoder(w).Encode(errorStruct)
	w.WriteHeader(code)
}

//Endpoints

// Endpoint health check
func endpointHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"alive"}`))
}

// Redis health check
func redisHealthCheck(w http.ResponseWriter, r *http.Request) {
	//We try to ping the redis cache and see if we get an error or not.
	if err := redisCache.Ping(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"unavailable"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"available"}`))
}

func getResponseFromModel(w http.ResponseWriter, r *http.Request) {
	//Lets create the prompt request to send.
	var promptReq protocol.PromptRequest

	if err := json.NewDecoder(r.Body).Decode(&promptReq); err != nil {
		replyWithError(w, http.StatusBadRequest, "Invalid JSON string")
	}
}

func main() {
	cfg := loadConfig()
	redisCache = cache.NewCache(cfg.RedisHost)

	mux := http.NewServeMux()

	//This endpoint health check
	mux.HandleFunc("GET /health/endpoint", endpointHealthCheck)

	//Redis health check
	mux.HandleFunc("GET /health/redis", redisHealthCheck)

	//

	log.Printf("Starting endpoint on port %s...", cfg.Port)
	http.ListenAndServe(":"+cfg.Port, mux)
}
