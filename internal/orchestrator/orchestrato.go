package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Cris245/go-llm-chat/internal/db"
	"github.com/Cris245/go-llm-chat/internal/llmclient"
	"github.com/Cris245/go-llm-chat/internal/sse"
)

// Orchestrator coordinates interactions with the LLMs and the database.
type Orchestrator struct {
	llm1Client llmclient.LLMClient // Client for the first LLM
	llm2Client llmclient.LLMClient // Client for the second LLM
	llm3Client llmclient.LLMClient // Client for the third LLM
	dbClient   db.Client           // Client for database operations (new field)
}

// NewOrchestrator creates a new instance of Orchestrator.
// It takes three LLMClient implementations and a db.Client implementation.
func NewOrchestrator(llm1, llm2, llm3 llmclient.LLMClient, dbClient db.Client) *Orchestrator {
	return &Orchestrator{
		llm1Client: llm1,
		llm2Client: llm2,
		llm3Client: llm3,
		dbClient:   dbClient, // Assign the database client
	}
}

// ProcessMessage orchestrates the calls to the LLMs and sends SSE events.
// It takes the user's message and a channel to send SSE events back to the client.
func (o *Orchestrator) ProcessMessage(ctx context.Context, userMessage string, eventChan chan<- sse.Event) {
	// Detect if the question is about flights
	lowerMsg := strings.ToLower(userMessage)
	if strings.Contains(lowerMsg, "vuelo") || strings.Contains(lowerMsg, "vuelos") || strings.Contains(lowerMsg, "flight") || strings.Contains(lowerMsg, "flights") {
		// Map of synonyms (lowercase) to their canonical DB names
		synonyms := map[string]string{
			"madrid":      "Madrid",
			"paris":       "Paris",
			"parís":       "Paris",
			"barcelona":   "Barcelona",
			"london":      "London",
			"londres":     "London",
			"new york":    "New York",
			"roma":        "Rome",
			"rome":        "Rome",
			"los angeles": "Los Angeles",
			"berlin":      "Berlin",
			"tokyo":       "Tokyo",
			"seville":     "Seville",
			"sevilla":     "Seville",
			"valencia":    "Valencia",
		}

		var origin, destination string

		lower := strings.ToLower(userMessage)
		for syn, canon := range synonyms {
			if origin == "" && (strings.Contains(lower, "from "+syn) || strings.Contains(lower, "desde "+syn)) {
				origin = canon
			}
			if destination == "" && (strings.Contains(lower, "to "+syn) || strings.Contains(lower, " a "+syn) || strings.Contains(lower, "hacia "+syn)) {
				destination = canon
			}
		}

		// If we still haven't found destination, attempt single-city detection ("... a londres?", "... londres?")
		if destination == "" {
			for syn, canon := range synonyms {
				if strings.Contains(lower, syn) && canon != origin {
					destination = canon
					break
				}
			}
		}

		// If both origin and destination are empty, we'll search without filters (all flights).
		flights, err := o.dbClient.SearchFlights(ctx, origin, destination)
		if err != nil || len(flights) == 0 {
			eventChan <- sse.Event{Type: "Message", Data: "No flights found for your query."}
			return
		}
		flightsInfo := ""
		for _, f := range flights {
			flightsInfo += fmt.Sprintf("Flight %s: %s -> %s, departure %s, arrival %s, price $%.2f\n",
				f.FlightNumber, f.Origin, f.Destination, f.DepartureTime, f.ArrivalTime, f.Price)
		}
		// LLM1: List the available flights
		promptLLM1 := "List the available flights from the following data. Only list the flights, do not provide extra information.\n" + flightsInfo
		// LLM2: For each flight, say how long it takes and how much it costs
		promptLLM2 := "For each flight in the following data, say how long the flight takes and how much it costs.\n" + flightsInfo

		// Channels to collect responses
		llm1RespChan := make(chan string, 1)
		llm2RespChan := make(chan string, 1)
		var wg sync.WaitGroup
		wg.Add(2)

		// LLM1 goroutine
		go func() {
			defer wg.Done()
			eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 1"}
			resp, err := o.llm1Client.ChatCompletion(ctx, promptLLM1)
			if err != nil {
				llm1RespChan <- "[LLM1 Error] " + err.Error()
			} else {
				llm1RespChan <- resp
			}
			eventChan <- sse.Event{Type: "Status", Data: "Got response from LLM 1"}
		}()

		// LLM2 goroutine
		go func() {
			defer wg.Done()
			eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 2"}
			resp, err := o.llm2Client.ChatCompletion(ctx, promptLLM2)
			if err != nil {
				llm2RespChan <- "[LLM2 Error] " + err.Error()
			} else {
				llm2RespChan <- resp
			}
			eventChan <- sse.Event{Type: "Status", Data: "Got response from LLM 2"}
		}()

		// Wait for both LLMs
		wg.Wait()
		close(llm1RespChan)
		close(llm2RespChan)

		// Collect responses
		llm1Resp := <-llm1RespChan
		llm2Resp := <-llm2RespChan

		// Send both responses as a single message event (with attribution)
		combined := "LLM1 (flights list):\n" + llm1Resp + "\n\nLLM2 (duration and cost):\n" + llm2Resp
		eventChan <- sse.Event{Type: "Message", Data: combined}
		return
	}
	// Prepare prompts for LLM1 and LLM2
	promptLLM1 := "Please answer the following question in a short, formal, and concise manner: " + userMessage
	promptLLM2 := "Please answer the following question in a friendly, verbose, and opinionated way, providing more information and your thoughts: " + userMessage

	// Channels to collect responses
	llm1RespChan := make(chan string, 1)
	llm2RespChan := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Add(2)

	// LLM1 goroutine
	go func() {
		defer wg.Done()
		eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 1"}
		resp, err := o.llm1Client.ChatCompletion(ctx, promptLLM1)
		if err != nil {
			llm1RespChan <- "[LLM1 Error] " + err.Error()
		} else {
			llm1RespChan <- resp
		}
		eventChan <- sse.Event{Type: "Status", Data: "Got response from LLM 1"}
	}()

	// LLM2 goroutine
	go func() {
		defer wg.Done()
		eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 2"}
		resp, err := o.llm2Client.ChatCompletion(ctx, promptLLM2)
		if err != nil {
			llm2RespChan <- "[LLM2 Error] " + err.Error()
		} else {
			llm2RespChan <- resp
		}
		eventChan <- sse.Event{Type: "Status", Data: "Got response from LLM 2"}
	}()

	// Wait for both LLMs
	wg.Wait()
	close(llm1RespChan)
	close(llm2RespChan)

	// Collect responses
	llm1Resp := <-llm1RespChan
	llm2Resp := <-llm2RespChan

	// Send both responses as a single message event (with attribution)
	combined := "LLM1 (short/formal):\n" + llm1Resp + "\n\nLLM2 (friendly/verbose):\n" + llm2Resp
	eventChan <- sse.Event{Type: "Message", Data: combined}
}
