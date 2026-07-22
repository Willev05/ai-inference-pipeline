package main

import (
	"ai-ingerence-pipeline/internal/cache"
	"ai-ingerence-pipeline/pkg/protocol"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var redisCache *cache.Cache
var cfg Config

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
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(errorStruct)
}

func requestModel(ctx context.Context, promptReq *protocol.PromptRequest) (string, error) {
	var ollamaRes protocol.OllamaResponse
	//We prepare the ollamaReq struct we will send the model.
	ollamaReq := protocol.OllamaRequest{
		Model:  *promptReq.Model,
		Prompt: *promptReq.Prompt,
		Stream: false,
	}

	//Then create json bytes slice from the array.
	jsonReqData, err := json.Marshal(ollamaReq)

	if err != nil {
		return "", fmt.Errorf("JSON Marshall error: %s", err.Error())
	}

	//We prepare a context for the request with timeout of 60s.
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)

	defer cancel()

	//We then create the request object with the context and a new buffer containing the marshalled struct
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.ModelEndpoint, bytes.NewBuffer(jsonReqData))
	if err != nil {
		return "", fmt.Errorf("Http create request error: %s", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")

	//Lastly, we create a client, and fire the request.
	client := &http.Client{}
	res, err := client.Do(req)
	//If an error occured, we check and return error.
	if err != nil {
		return "", fmt.Errorf("Ollama unreachable or timed out: %s", err.Error())
	}
	defer res.Body.Close()

	//We get the response from ollama
	err = json.NewDecoder(res.Body).Decode(&ollamaRes)
	if err != nil {
		return "", fmt.Errorf("Error parsing ollama json: %s", err.Error())
	}

	//And check if it returned an error or response (either error or response will be nil)
	if ollamaRes.Error != nil {
		return "", fmt.Errorf("Ollama error code %d: %s", res.StatusCode, *ollamaRes.Error)
	}

	return *ollamaRes.Response, nil
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

// Get response from model (or cache if available)
func getResponseFromModel(w http.ResponseWriter, r *http.Request) {
	//Lets create the prompt request to send.
	var promptReq protocol.PromptRequest

	//Try to decode the request json
	if err := json.NewDecoder(r.Body).Decode(&promptReq); err != nil {
		replyWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	//If the prompt was not included, then return error.
	if promptReq.Prompt == nil {
		replyWithError(w, http.StatusBadRequest, "Invalid JSON structure")
		return
	}

	//This is the default/fallback model
	if promptReq.Model == nil {
		fallback := "llama3.1:8b"
		promptReq.Model = &fallback
	}

	//We hash the response and send it to redis
	hashedReq := redisCache.HashRequest(&promptReq)
	redisRes, err := redisCache.Get(r.Context(), hashedReq)

	var gatewayRes protocol.GatewayResponse

	//We start by checking if redis simply did not find the prompt/model combo
	if err == redis.Nil {
		modelRes, err := requestModel(r.Context(), &promptReq)

		if err != nil {
			log.Printf("%s", err.Error())
			replyWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		gatewayRes.Response = modelRes
		gatewayRes.Cached = false

		//We also cache it in redis
		//7 days since LLM inference is expensive
		if err := redisCache.Set(r.Context(), hashedReq, gatewayRes.Response, time.Hour*24*7); err != nil {
			log.Printf("Redis error: %s", err.Error())
		}
	} else if err != nil {
		replyWithError(w, http.StatusInternalServerError, err.Error())
		return
	} else {
		gatewayRes.Response = redisRes
		gatewayRes.Cached = true
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(gatewayRes)

	if err != nil {
		log.Printf("Json encoding error when encoding gateway response: %s", err.Error())
	}
}

func main() {
	cfg = loadConfig()
	redisCache = cache.NewCache(cfg.RedisHost)

	mux := http.NewServeMux()

	//This endpoint health check
	mux.HandleFunc("GET /health/endpoint", endpointHealthCheck)

	//Redis health check
	mux.HandleFunc("GET /health/redis", redisHealthCheck)

	//The main endpoint, for getting model response to user prompt.
	mux.HandleFunc("POST /api/generate", getResponseFromModel)

	log.Printf("Starting endpoint on port %s...", cfg.Port)
	http.ListenAndServe(":"+cfg.Port, mux)
}
