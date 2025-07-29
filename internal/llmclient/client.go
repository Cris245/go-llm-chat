package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// LLMClient defines the interface for interacting with a Large Language Model.
type LLMClient interface {
	StreamChatCompletion(ctx context.Context, prompt string) (<-chan string, error)
	ChatCompletion(ctx context.Context, prompt string) (string, error)
}

// OpenAIClient implements the LLMClient interface for the OpenAI API.
type OpenAIClient struct {
	apiKey string
	model  string
	client *http.Client
}

// OpenAI API request/response structures
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
}

// NewOpenAIClient creates a new instance of OpenAIClient.
func NewOpenAIClient(model string) *OpenAIClient {
	return &OpenAIClient{
		apiKey: os.Getenv("OPENAI_API_KEY"),
		model:  model,
		client: &http.Client{},
	}
}

// StreamChatCompletion sends a prompt to the LLM and returns a channel for streaming the response.
func (c *OpenAIClient) StreamChatCompletion(ctx context.Context, prompt string) (<-chan string, error) {
	// For now, use the non-streaming version and return it as a stream
	// We can implement actual streaming later
	result, err := c.ChatCompletion(ctx, prompt)
	if err != nil {
		return nil, err
	}

	outputChan := make(chan string, 1)
	outputChan <- result
	close(outputChan)

	return outputChan, nil
}

// ChatCompletion sends a prompt to the LLM and waits for the complete response.
func (c *OpenAIClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("OpenAI API key not set")
	}

	// Create the request payload
	requestBody := ChatCompletionRequest{
		Model: c.model,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Make the request
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var chatResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return chatResp.Choices[0].Message.Content, nil
}
