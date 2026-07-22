package protocol

// The request sent by the client to the API gateway.
type PromptRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// The response the API gateway will return to the client.
type GatewayResponse struct {
	Response string `json:"response"`
	Cached   bool   `json:"cached"`
}

// The JSON structure returned by Ollama's endpoint.
type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}
