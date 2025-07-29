package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Cris245/go-llm-chat/internal/db"           // Database package
	"github.com/Cris245/go-llm-chat/internal/llmclient"    // LLM client package
	"github.com/Cris245/go-llm-chat/internal/orchestrator" // Orchestrator package
	"github.com/Cris245/go-llm-chat/internal/sse"          // SSE package
)

func main() {
	// Check if the OPENAI_API_KEY environment variable is set.
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("Error: OPENAI_API_KEY environment variable is not set. Please set it before running.")
	}

	// Get MongoDB URI from environment variable. Docker Compose will set this.
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("Error: MONGO_URI environment variable is not set. Please set it or ensure docker-compose.yml provides it.")
	}

	// Create a context for database connection with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // Ensure the context is cancelled when main exits.

	// Initialize MongoDB client and connect to the database.
	dbClient, err := db.NewClient(ctx, mongoURI)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer dbClient.Disconnect(context.Background()) // Ensure MongoDB connection is closed when main exits.

	// Populate the database with sample flights if empty
	if err := dbClient.SeedFlights(ctx); err != nil {
		log.Fatalf("Error seeding flights: %v", err)
	}

	log.Printf("Is OPENAI_API_KEY present?: %v", os.Getenv("OPENAI_API_KEY") != "")

	// Initialize LLM clients
	llm1Client := llmclient.NewOpenAIClient("gpt-4o-mini")
	llm2Client := llmclient.NewOpenAIClient("gpt-4o-mini")
	llm3Client := llmclient.NewOpenAIClient("gpt-4o-mini")

	// Initialize orchestrator with all three LLM clients
	orch := orchestrator.NewOrchestrator(llm1Client, llm2Client, llm3Client, dbClient)

	// Handle HTTP POST requests to the "/api" endpoint.
	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the user's message from the request body.
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
		userMessage := string(buf)
		if userMessage == "" {
			http.Error(w, "User message cannot be empty", http.StatusBadRequest)
			return
		}

		// Create a new SSE handler for this specific request.
		sseHandler := sse.NewHandler()
		// Create a channel for the orchestrator to send events to the SSE handler.
		eventChan := make(chan sse.Event)

		// Start a goroutine to process the message with the orchestrator.
		// This allows the HTTP handler to immediately set up the SSE connection
		// while the LLM processing happens concurrently.
		go func() {
			defer close(eventChan)                                   // Ensure the event channel is closed when processing is done.
			orch.ProcessMessage(r.Context(), userMessage, eventChan) // Pass the context for cancellation.
		}()

		// Serve the SSE events to the client using the sseHandler and the eventChan.
		sseHandler.ServeHTTP(w, r, eventChan)
	})

	// Start the HTTP server on port 8080.
	log.Println("Server listening on :8080. Send POST requests to /api with your message in the body.")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
