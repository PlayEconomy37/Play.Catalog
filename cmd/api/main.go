package main

import (
	"context"
	"os"
	"time"

	"github.com/PlayEconomy37/Play.Catalog/internal/constants"
	"github.com/PlayEconomy37/Play.Catalog/internal/data"
	"github.com/PlayEconomy37/Play.Catalog/internal/rabbitmq"
	"github.com/PlayEconomy37/Play.Common/common"
	"github.com/PlayEconomy37/Play.Common/configuration"
	"github.com/PlayEconomy37/Play.Common/database"
	"github.com/PlayEconomy37/Play.Common/events"
	"github.com/PlayEconomy37/Play.Common/logger"
	"github.com/PlayEconomy37/Play.Common/opentelemetry"
	"github.com/PlayEconomy37/Play.Common/types"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/otel"
)

// Application is a struct that defines the Catalog's microservice application.
// It embeds the common packages common application struct.
type Application struct {
	common.App
	ItemsRepository types.MongoRepository[primitive.ObjectID, data.Item]
	UsersRepository types.MongoRepository[int64, database.User]
}

func main() {
	// Setup logger
	logger := logger.New(os.Stdout, logger.LevelInfo)

	// Read configuration
	config, err := configuration.LoadConfig("config/dev.json")
	if err != nil {
		logger.Fatal(err, nil)
	}

	// Start MongoDB
	mongoClient, err := database.NewMongoClient(config)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err = mongoClient.Disconnect(ctx); err != nil {
			logger.Fatal(err, nil)
		}
	}()

	// Create "items" collection
	err = data.CreateItemsCollection(mongoClient, constants.Database)
	if err != nil {
		logger.Fatal(err, nil)
	}

	// Create "users" collection
	err = database.CreateUsersCollection(mongoClient, constants.Database)
	if err != nil {
		logger.Fatal(err, nil)
	}

	// Initialize tracer
	tracerProvider := opentelemetry.SetupTracer(false)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := tracerProvider.Shutdown(ctx); err != nil {
			logger.Error(err, nil)
		}
	}()

	// Connect to RabbitMQ
	rabbitMQConnection, err := events.NewRabbitMQConnection(config)
	if err != nil {
		logger.Fatal(err, nil)
	}

	defer rabbitMQConnection.Close()

	// Create users repository
	usersRepository := database.NewMongoRepository[int64, database.User](mongoClient, constants.Database, database.UsersCollection)

	// Create consumer
	updatedUserConsumer := rabbitmq.NewUserUpdatedConsumer(rabbitMQConnection, usersRepository, config.ServiceName, logger)

	// Watch the queue and consume events
	go func() {
		err = updatedUserConsumer.StartConsumer()
		if err != nil {
			logger.Fatal(err, nil)
		}
	}()

	app := &Application{
		App: common.App{
			Config: config,
			Logger: logger,
			Tracer: otel.Tracer(config.ServiceName),
		},
		ItemsRepository: database.NewMongoRepository[primitive.ObjectID, data.Item](mongoClient, constants.Database, constants.ItemsCollection),
		UsersRepository: usersRepository,
	}

	err = app.Serve(app.routes())
	if err != nil {
		logger.Fatal(err, nil)
	}
}
