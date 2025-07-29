package db

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson"          // BSON (Binary JSON) package for MongoDB documents
	"go.mongodb.org/mongo-driver/mongo"         // MongoDB Go Driver main package
	"go.mongodb.org/mongo-driver/mongo/options" // Options for MongoDB client and operations
)

// Client defines the interface for database operations.
// Using an interface allows easy swapping between a real MongoDB client and a mock client for testing.
type Client interface {
	Connect(ctx context.Context, uri string) error
	Disconnect(ctx context.Context) error
	InsertFlights(ctx context.Context, flights []Flight) error // New method for inserting flights
	SearchFlights(ctx context.Context, origin, destination string, maxPrice float64) ([]Flight, error)
}

// MongoDBClient implements the Client interface for MongoDB.
type MongoDBClient struct {
	client     *mongo.Client     // The underlying MongoDB client connection
	collection *mongo.Collection // The specific MongoDB collection to work with (e.g., "flights")
}

// NewClient creates a new MongoDBClient instance and establishes a connection to the database.
func NewClient(ctx context.Context, uri string) (*MongoDBClient, error) {
	// Set client options using the provided URI (connection string).
	clientOptions := options.Client().ApplyURI(uri)

	// Connect to MongoDB. This does not block for server discovery.
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the database to verify a successful connection.
	err = client.Ping(ctx, nil)
	if err != nil {
		// Disconnect if ping fails to clean up resources.
		if disconnectErr := client.Disconnect(ctx); disconnectErr != nil {
			log.Printf("Error disconnecting after failed ping: %v", disconnectErr)
		}
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}
	log.Println("Successfully connected to MongoDB!")

	// Select the database ("flightdb") and collection ("flights") to use.
	collection := client.Database("flightdb").Collection("flights")

	return &MongoDBClient{
		client:     client,
		collection: collection,
	}, nil
}

// Connect is part of the Client interface. For MongoDBClient, connection is established during NewClient.
func (m *MongoDBClient) Connect(ctx context.Context, uri string) error {
	// For MongoDBClient, connection is handled by NewClient.
	return nil
}

// Disconnect closes the MongoDB connection.
// It's good practice to defer this call after creating the client in main.
func (m *MongoDBClient) Disconnect(ctx context.Context) error {
	if m.client == nil {
		return nil // No client to disconnect.
	}
	log.Println("Disconnecting from MongoDB...")
	return m.client.Disconnect(ctx)
}

// InsertFlights inserts multiple flight documents into the collection.
func (m *MongoDBClient) InsertFlights(ctx context.Context, flights []Flight) error {
	if len(flights) == 0 {
		return nil // Nothing to insert.
	}

	// Convert []Flight to []interface{} as InsertMany expects a slice of interface{}.
	docs := make([]interface{}, len(flights))
	for i, flight := range flights {
		docs[i] = flight
	}

	_, err := m.collection.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to insert flights: %w", err)
	}
	log.Printf("Inserted %d flight documents.", len(flights))
	return nil
}

// SeedFlightData inserts some initial fictional flight data if the collection is empty.
// This function is called once on application startup to populate the database.
func SeedFlightData(ctx context.Context, client Client) error {
	// Check if the collection is empty to avoid re-inserting data on every restart.
	// Cast to *MongoDBClient to access the underlying collection for CountDocuments.
	mongoClient, ok := client.(*MongoDBClient)
	if !ok {
		// In a real app, this should be handled more gracefully.
		// For this example, the client is assumed to always be a *MongoDBClient.
		return fmt.Errorf("client is not a *MongoDBClient")
	}
	count, err := mongoClient.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to count documents: %w", err)
	}
	if count > 0 {
		log.Println("Flight data already exists. Skipping seeding.")
		return nil
	}

	// Define some fictional flight data.
	flights := []Flight{
		{
			FlightNumber:   "FL101",
			Origin:         "New York",
			Destination:    "London",
			DepartureTime:  "2025-08-10T09:00:00Z",
			ArrivalTime:    "2025-08-10T17:00:00Z",
			Price:          550.00,
			AvailableSeats: 120,
		},
		{
			FlightNumber:   "FL102",
			Origin:         "London",
			Destination:    "New York",
			DepartureTime:  "2025-08-11T10:00:00Z",
			ArrivalTime:    "2025-08-11T18:00:00Z",
			Price:          520.00,
			AvailableSeats: 100,
		},
		{
			FlightNumber:   "FL203",
			Origin:         "Paris",
			Destination:    "Rome",
			DepartureTime:  "2025-08-12T14:30:00Z",
			ArrivalTime:    "2025-08-12T16:00:00Z",
			Price:          120.00,
			AvailableSeats: 50,
		},
		{
			FlightNumber:   "FL204",
			Origin:         "Rome",
			Destination:    "Paris",
			DepartureTime:  "2025-08-13T11:00:00Z",
			ArrivalTime:    "2025-08-13T12:30:00Z",
			Price:          110.00,
			AvailableSeats: 60,
		},
		{
			FlightNumber:   "FL305",
			Origin:         "New York",
			Destination:    "Los Angeles",
			DepartureTime:  "2025-08-15T08:00:00Z",
			ArrivalTime:    "2025-08-15T11:00:00Z",
			Price:          300.00,
			AvailableSeats: 200,
		},
	}

	// Insert the defined flights into the database.
	return client.InsertFlights(ctx, flights)
}

func (m *MongoDBClient) SeedFlights(ctx context.Context) error {
	log.Println("Ensuring sample flights are present (upsert)...")
	flights := []Flight{
		{
			FlightNumber:   "FL101",
			Origin:         "Madrid",
			Destination:    "Paris",
			DepartureTime:  "2025-08-10T09:00:00Z",
			ArrivalTime:    "2025-08-10T11:00:00Z",
			Price:          120.0,
			AvailableSeats: 50,
		},
		{
			FlightNumber:   "FL102",
			Origin:         "Madrid",
			Destination:    "Paris",
			DepartureTime:  "2025-08-10T15:00:00Z",
			ArrivalTime:    "2025-08-10T17:00:00Z",
			Price:          150.0,
			AvailableSeats: 30,
		},
		{
			FlightNumber:   "FL103",
			Origin:         "Madrid",
			Destination:    "Paris",
			DepartureTime:  "2025-08-11T10:00:00Z",
			ArrivalTime:    "2025-08-11T12:00:00Z",
			Price:          110.0,
			AvailableSeats: 20,
		},
		{
			FlightNumber:   "FL104",
			Origin:         "Madrid",
			Destination:    "Paris",
			DepartureTime:  "2025-08-11T18:00:00Z",
			ArrivalTime:    "2025-08-11T20:00:00Z",
			Price:          130.0,
			AvailableSeats: 40,
		},
		// Additional sample flights for more diverse queries
		{
			FlightNumber:   "FL105",
			Origin:         "Madrid",
			Destination:    "Barcelona",
			DepartureTime:  "2025-08-12T07:00:00Z",
			ArrivalTime:    "2025-08-12T08:30:00Z",
			Price:          90.0,
			AvailableSeats: 60,
		},
		{
			FlightNumber:   "FL106",
			Origin:         "Barcelona",
			Destination:    "Madrid",
			DepartureTime:  "2025-08-12T19:00:00Z",
			ArrivalTime:    "2025-08-12T20:30:00Z",
			Price:          95.0,
			AvailableSeats: 55,
		},
		{
			FlightNumber:   "FL107",
			Origin:         "London",
			Destination:    "New York",
			DepartureTime:  "2025-08-13T09:00:00Z",
			ArrivalTime:    "2025-08-13T17:00:00Z",
			Price:          550.0,
			AvailableSeats: 120,
		},
		{
			FlightNumber:   "FL108",
			Origin:         "New York",
			Destination:    "London",
			DepartureTime:  "2025-08-14T10:00:00Z",
			ArrivalTime:    "2025-08-14T18:00:00Z",
			Price:          540.0,
			AvailableSeats: 110,
		},
		{
			FlightNumber:   "FL109",
			Origin:         "Rome",
			Destination:    "Paris",
			DepartureTime:  "2025-08-15T11:00:00Z",
			ArrivalTime:    "2025-08-15T12:30:00Z",
			Price:          115.0,
			AvailableSeats: 65,
		},
		{
			FlightNumber:   "FL110",
			Origin:         "London",
			Destination:    "Paris",
			DepartureTime:  "2025-08-16T09:00:00Z",
			ArrivalTime:    "2025-08-16T11:30:00Z",
			Price:          200.0,
			AvailableSeats: 100,
		},
		{
			FlightNumber:   "FL111",
			Origin:         "Paris",
			Destination:    "London",
			DepartureTime:  "2025-08-16T14:00:00Z",
			ArrivalTime:    "2025-08-16T16:30:00Z",
			Price:          195.0,
			AvailableSeats: 100,
		},
		{
			FlightNumber:   "FL112",
			Origin:         "London",
			Destination:    "Berlin",
			DepartureTime:  "2025-08-17T08:00:00Z",
			ArrivalTime:    "2025-08-17T10:00:00Z",
			Price:          160.0,
			AvailableSeats: 80,
		},
		{
			FlightNumber:   "FL113",
			Origin:         "Berlin",
			Destination:    "London",
			DepartureTime:  "2025-08-17T18:00:00Z",
			ArrivalTime:    "2025-08-17T20:00:00Z",
			Price:          155.0,
			AvailableSeats: 85,
		},
		{
			FlightNumber:   "FL114",
			Origin:         "Barcelona",
			Destination:    "Seville",
			DepartureTime:  "2025-08-18T07:30:00Z",
			ArrivalTime:    "2025-08-18T08:45:00Z",
			Price:          80.0,
			AvailableSeats: 70,
		},
		{
			FlightNumber:   "FL115",
			Origin:         "Seville",
			Destination:    "Barcelona",
			DepartureTime:  "2025-08-18T19:30:00Z",
			ArrivalTime:    "2025-08-18T20:45:00Z",
			Price:          82.0,
			AvailableSeats: 70,
		},
		{
			FlightNumber:   "FL116",
			Origin:         "Madrid",
			Destination:    "Valencia",
			DepartureTime:  "2025-08-19T06:00:00Z",
			ArrivalTime:    "2025-08-19T07:00:00Z",
			Price:          70.0,
			AvailableSeats: 90,
		},
		{
			FlightNumber:   "FL117",
			Origin:         "Valencia",
			Destination:    "Madrid",
			DepartureTime:  "2025-08-19T18:00:00Z",
			ArrivalTime:    "2025-08-19T19:00:00Z",
			Price:          72.0,
			AvailableSeats: 88,
		},
		{
			FlightNumber:   "FL118",
			Origin:         "Tokyo",
			Destination:    "Los Angeles",
			DepartureTime:  "2025-08-20T02:00:00Z",
			ArrivalTime:    "2025-08-20T12:00:00Z",
			Price:          900.0,
			AvailableSeats: 250,
		},
		{
			FlightNumber:   "FL119",
			Origin:         "Los Angeles",
			Destination:    "Tokyo",
			DepartureTime:  "2025-08-21T03:00:00Z",
			ArrivalTime:    "2025-08-21T13:00:00Z",
			Price:          880.0,
			AvailableSeats: 245,
		},
		{
			FlightNumber:   "FL120",
			Origin:         "New York",
			Destination:    "Tokyo",
			DepartureTime:  "2025-08-22T04:00:00Z",
			ArrivalTime:    "2025-08-22T18:00:00Z",
			Price:          950.0,
			AvailableSeats: 200,
		},
	}
	for _, f := range flights {
		filter := bson.M{"flight_number": f.FlightNumber}
		update := bson.M{"$set": f}
		opts := options.Update().SetUpsert(true)
		if _, err := m.collection.UpdateOne(ctx, filter, update, opts); err != nil {
			log.Printf("Error upserting flight %s: %v", f.FlightNumber, err)
			return err
		}
	}
	log.Println("Sample flights ensured (upsert complete).")
	return nil
}

func (m *MongoDBClient) SearchFlights(ctx context.Context, origin, destination string, maxPrice float64) ([]Flight, error) {
	// Build MongoDB filter dynamically based on provided parameters.
	filter := bson.M{}
	if origin != "" {
		filter["origin"] = bson.M{"$regex": origin, "$options": "i"} // Case-insensitive match
	}
	if destination != "" {
		if origin == "" {
			// If only destination provided, search where either origin or destination matches
			filter["$or"] = []bson.M{
				{"destination": bson.M{"$regex": destination, "$options": "i"}},
				{"origin": bson.M{"$regex": destination, "$options": "i"}},
			}
		} else {
			filter["destination"] = bson.M{"$regex": destination, "$options": "i"}
		}
	}
	// Add price filter if maxPrice is specified (> 0)
	if maxPrice > 0 {
		filter["price"] = bson.M{"$lte": maxPrice}
	}
	cur, err := m.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var flights []Flight
	for cur.Next(ctx) {
		var f Flight
		if err := cur.Decode(&f); err == nil {
			flights = append(flights, f)
		}
	}
	return flights, nil
}
