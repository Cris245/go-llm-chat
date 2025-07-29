#!/bin/bash

# Load Testing Script for Go LLM Chat
# Tests concurrent request handling and validates all responses are received

set -e

# Default number of concurrent requests
NUM_REQUESTS=${1:-5}

# Server URL
SERVER_URL="http://localhost:8080/api"

# Test messages (mix of flight and general queries)
MESSAGES=(
    "What is the capital of France?"
    "Show me flights from Madrid to Paris under 600 euros"
    "Explain quantum physics in simple terms"
    "Hay vuelos a Londres?"
    "What's the best programming language for beginners?"
    "Show me flights from Barcelona to Seville"
    "How does Docker work?"
    "Vuelos de Madrid a Valencia"
    "Explain machine learning in simple terms"
    "Show me flights to Tokyo under 1000 euros"
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Starting Load Test with $NUM_REQUESTS concurrent requests${NC}"
echo -e "${YELLOW}Testing server at: $SERVER_URL${NC}"
echo ""

# Check if server is running
if ! curl -s -f -X POST -d "test" "$SERVER_URL" > /dev/null 2>&1; then
    echo -e "${RED}Error: Server is not running at $SERVER_URL${NC}"
    echo "Please start the server with: docker-compose up --build"
    exit 1
fi

echo -e "${GREEN}Server is running${NC}"
echo ""

# Function to send a single request and capture response
send_request() {
    local request_id=$1
    local message="${MESSAGES[$((request_id % ${#MESSAGES[@]}))]}"
    
    echo -e "${BLUE}Request $request_id: $message${NC}"
    
    # Send request and capture response
    response=$(curl -s -N -X POST -d "$message" "$SERVER_URL" 2>/dev/null)
    
    # Check if we got a response
    if [ -n "$response" ]; then
        # Extract the final message content (the last data line that's not a Status event)
        final_message=$(echo "$response" | grep "data: " | grep -v "Invoking LLM" | tail -n 1 | sed 's/data: //')
        
        # Get preview of the response (first 50 characters)
        preview=$(echo "$final_message" | cut -c1-50)
        if [ ${#preview} -eq 50 ]; then
            preview="$preview..."
        fi
        
        echo -e "${GREEN}Request $request_id completed successfully${NC}"
        echo -e "${YELLOW}Response preview: $preview${NC}"
        return 0
    else
        echo -e "${RED}Request $request_id failed or timed out${NC}"
        return 1
    fi
}

# Function to run concurrent requests
run_concurrent_requests() {
    local num_requests=$1
    local pids=()
    local failed_requests=0
    local successful_requests=0
    
    echo -e "${YELLOW}Starting $num_requests concurrent requests...${NC}"
    echo ""
    
    # Start all requests concurrently
    for ((i=1; i<=num_requests; i++)); do
        send_request $i &
        pids+=($!)
    done
    
    # Wait for all requests to complete
    for pid in "${pids[@]}"; do
        if wait $pid; then
            ((successful_requests++))
        else
            ((failed_requests++))
        fi
    done
    
    echo ""
    echo -e "${BLUE}Load Test Results:${NC}"
    echo -e "${GREEN}Successful requests: $successful_requests${NC}"
    echo -e "${RED}Failed requests: $failed_requests${NC}"
    echo -e "${YELLOW}Success rate: $((successful_requests * 100 / num_requests))%${NC}"
    
    # Return success if all requests succeeded
    if [ $failed_requests -eq 0 ]; then
        echo ""
        echo -e "${GREEN}All requests completed successfully!${NC}"
        return 0
    else
        echo ""
        echo -e "${RED}Some requests failed. Check server logs for details.${NC}"
        return 1
    fi
}

# Function to test SSE streaming
test_sse_streaming() {
    echo ""
    echo -e "${BLUE}Testing SSE streaming with a single request...${NC}"
    
    # Send a request and capture all events
    response=$(curl -s -N -X POST -d "What is the capital of France?" "$SERVER_URL" 2>/dev/null)
    
    # Check for SSE events
    if echo "$response" | grep -q "event:"; then
        echo -e "${GREEN}SSE streaming is working correctly${NC}"
        echo -e "${YELLOW}Events received:${NC}"
        echo "$response" | grep "event:" | head -5
    else
        echo -e "${RED}SSE streaming may not be working correctly${NC}"
    fi
}

# Main execution
main() {
    echo -e "${BLUE}Go LLM Chat Load Test${NC}"
    echo "=================================="
    
    # Test SSE streaming first
    test_sse_streaming
    
    # Run concurrent requests
    if run_concurrent_requests $NUM_REQUESTS; then
        echo ""
        echo -e "${GREEN}Load test PASSED! Server handles concurrent requests well.${NC}"
        exit 0
    else
        echo ""
        echo -e "${RED}Load test FAILED! Some requests were not handled properly.${NC}"
        exit 1
    fi
}

# Run the main function
main 