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

		// If destination still hasn't been found, attempt single-city detection ("... a londres?", "... londres?")
		if destination == "" {
			for syn, canon := range synonyms {
				if strings.Contains(lower, syn) && canon != origin {
					destination = canon
					break
				}
			}
		}

		// If both origin and destination are empty, search without filters (all flights).
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
			eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 1 (list available flights only)"}
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
			eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 2 (calculate duration and cost for each flight)"}
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

		// Now use LLM3 to aggregate the responses
		eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 3 (aggregation)"}

		aggregationPrompt := fmt.Sprintf(`You are an intelligent aggregator. Combine these two responses about flights into one coherent, well-formatted answer:

LLM1 Response (flight list):
%s

LLM2 Response (duration and cost):
%s

Please create a unified response that:
1. Lists all available flights clearly
2. Includes duration and cost for each flight
3. Uses clean formatting without excessive markdown (avoid ** for emphasis)
4. Removes any redundancy between the two responses
5. Maintains all the important information from both responses
6. Uses simple formatting like "Flight FL101:" instead of "**Flight FL101:**"`, llm1Resp, llm2Resp)

		llm3Resp, err := o.llm3Client.ChatCompletion(ctx, aggregationPrompt)
		if err != nil {
			eventChan <- sse.Event{Type: "Status", Data: "LLM3 aggregation failed"}
			// Fallback to combined response
			combined := "LLM1 (flights list):\n" + llm1Resp + "\n\nLLM2 (duration and cost):\n" + llm2Resp
			eventChan <- sse.Event{Type: "Message", Data: combined}
		} else {
			eventChan <- sse.Event{Type: "Status", Data: "Got response from LLM 3"}
			eventChan <- sse.Event{Type: "Message", Data: llm3Resp}
		}
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

	// Use LLM3 to aggregate the two different style responses
	eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 3 (aggregation)"}

	aggregationPrompt := fmt.Sprintf(`You are an intelligent aggregator. Combine these two responses to the same question into one coherent, well-balanced answer:

LLM1 Response (formal and concise):
%s

LLM2 Response (friendly and verbose):
%s

At the top of your answer, briefly state that LLM1 is short/formal/concise and LLM2 is friendly/verbose/opinionated.

Please create a unified response that:
1. Combines the best of both styles
2. Is well-formatted and easy to read
3. Removes redundancy while keeping all important information
4. Maintains a balanced tone between formal and friendly`, llm1Resp, llm2Resp)

	llm3Resp, err := o.llm3Client.ChatCompletion(ctx, aggregationPrompt)
	if err != nil {
		eventChan <- sse.Event{Type: "Status", Data: "LLM3 aggregation failed"}
		// Fallback to combined response
		combined := "LLM1 (short, formal, concise):\n" + llm1Resp + "\n\nLLM2 (friendly, verbose, opinionated):\n" + llm2Resp
		eventChan <- sse.Event{Type: "Message", Data: combined}
	} else {
		eventChan <- sse.Event{Type: "Status", Data: "Got response from LLM 3"}
		eventChan <- sse.Event{Type: "Message", Data: llm3Resp}
	}
}

// ProcessMessageStream orchestrates the calls to the LLMs and streams the final response.
// This version uses streaming for the final LLM3 response to provide real-time updates.
func (o *Orchestrator) ProcessMessageStream(ctx context.Context, userMessage string, eventChan chan<- sse.Event) {
	// Detect if the question is about flights
	lower := strings.ToLower(userMessage)
	isFlightQuery := strings.Contains(lower, "vuelo") || strings.Contains(lower, "flight") ||
		strings.Contains(lower, "fly") || strings.Contains(lower, "airplane") ||
		strings.Contains(lower, "madrid") || strings.Contains(lower, "paris") ||
		strings.Contains(lower, "london") || strings.Contains(lower, "londres") ||
		strings.Contains(lower, "barcelona") || strings.Contains(lower, "valencia") ||
		strings.Contains(lower, "seville") || strings.Contains(lower, "sevilla") ||
		strings.Contains(lower, "tokyo") || strings.Contains(lower, "new york") ||
		strings.Contains(lower, "los angeles") || strings.Contains(lower, "berlin") ||
		strings.Contains(lower, "rome") || strings.Contains(lower, "roma")

	if isFlightQuery {
		// Map of synonyms (lowercase) to their canonical DB names
		synonyms := map[string]string{
			"madrid": "Madrid", "paris": "Paris", "london": "London", "londres": "London",
			"barcelona": "Barcelona", "valencia": "Valencia", "seville": "Seville", "sevilla": "Seville",
			"tokyo": "Tokyo", "new york": "New York", "nyc": "New York", "jfk": "New York",
			"los angeles": "Los Angeles", "la": "Los Angeles", "lax": "Los Angeles",
			"berlin": "Berlin", "rome": "Rome", "roma": "Rome",
		}

		// Extract origin and destination from the query
		origin := ""
		destination := ""

		// Look for origin-destination patterns
		for syn, canon := range synonyms {
			if strings.Contains(lower, "from "+syn) || strings.Contains(lower, "desde "+syn) {
				origin = canon
			}
			if strings.Contains(lower, "to "+syn) || strings.Contains(lower, " a "+syn) || strings.Contains(lower, "hacia "+syn) {
				destination = canon
			}
		}

		// If destination still hasn't been found, attempt single-city detection ("... a londres?", "... londres?")
		if destination == "" {
			for syn, canon := range synonyms {
				if strings.Contains(lower, syn) && canon != origin {
					destination = canon
					break
				}
			}
		}

		// If both origin and destination are empty, search without filters (all flights).
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

		// Now use LLM3 to aggregate the responses with streaming
		eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 3 (aggregation)"}

		aggregationPrompt := fmt.Sprintf(`You are an intelligent aggregator. Combine these two responses about flights into one coherent, well-formatted answer:

LLM1 Response (flight list):
%s

LLM2 Response (duration and cost):
%s

Please create a unified response that:
1. Lists all available flights clearly
2. Includes duration and cost for each flight
3. Is well-formatted and easy to read
4. Removes any redundancy between the two responses
5. Maintains all the important information from both responses`, llm1Resp, llm2Resp)

		// Use streaming for the final response
		streamChan, err := o.llm3Client.StreamChatCompletion(ctx, aggregationPrompt)
		if err != nil {
			eventChan <- sse.Event{Type: "Status", Data: "LLM3 aggregation failed"}
			// Fallback to combined response
			combined := "LLM1 (flights list):\n" + llm1Resp + "\n\nLLM2 (duration and cost):\n" + llm2Resp
			eventChan <- sse.Event{Type: "Message", Data: combined}
		} else {
			eventChan <- sse.Event{Type: "Status", Data: "Got response from LLM 3"}
			// Stream the final response
			for chunk := range streamChan {
				eventChan <- sse.Event{Type: "Message", Data: chunk}
			}
		}
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

	// Use LLM3 to aggregate the two different style responses with streaming
	eventChan <- sse.Event{Type: "Status", Data: "Invoking LLM 3 (aggregation)"}

	aggregationPrompt := fmt.Sprintf(`You are an intelligent aggregator. Combine these two responses to the same question into one coherent, well-balanced answer:

LLM1 Response (formal and concise):
%s

LLM2 Response (friendly and verbose):
%s

At the top of your answer, briefly state that LLM1 is short/formal/concise and LLM2 is friendly/verbose/opinionated.

Please create a unified response that:
1. Combines the best of both styles
2. Is well-formatted and easy to read
3. Removes redundancy while keeping all important information
4. Maintains a balanced tone between formal and friendly`, llm1Resp, llm2Resp)

	// Use streaming for the final response
	streamChan, err := o.llm3Client.StreamChatCompletion(ctx, aggregationPrompt)
	if err != nil {
		eventChan <- sse.Event{Type: "Status", Data: "LLM3 aggregation failed"}
		// Fallback to combined response
		combined := "LLM1 (formal):\n" + llm1Resp + "\n\nLLM2 (friendly):\n" + llm2Resp
		eventChan <- sse.Event{Type: "Message", Data: combined}
	} else {
		eventChan <- sse.Event{Type: "Status", Data: "Got response from LLM 3"}
		// Stream the final response
		for chunk := range streamChan {
			eventChan <- sse.Event{Type: "Message", Data: chunk}
		}
	}
}
