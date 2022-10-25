package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/PlayEconomy37/Play.Common/database"
	"github.com/PlayEconomy37/Play.Common/events"
	"github.com/PlayEconomy37/Play.Common/logger"
	"github.com/PlayEconomy37/Play.Common/types"

	amqp "github.com/rabbitmq/amqp091-go"
)

// UserUpdatedConsumer is the consumer for user updated event
type UserUpdatedConsumer struct {
	conn            *amqp.Connection
	exchangeName    string
	routingKey      string
	consumerTag     string
	queueName       string
	usersRepository types.MongoRepository[int64, database.User]
	logger          *logger.Logger
}

// NewUserUpdatedConsumer returns a new UserUpdatedConsumer
func NewUserUpdatedConsumer(
	conn *amqp.Connection,
	usersRepository types.MongoRepository[int64, database.User],
	serviceName string,
	logger *logger.Logger,
) *UserUpdatedConsumer {
	return &UserUpdatedConsumer{
		conn:            conn,
		exchangeName:    "Play.Identity:user-updated",
		routingKey:      "",
		consumerTag:     "",
		queueName:       fmt.Sprintf("%s-user-updated", serviceName),
		usersRepository: usersRepository,
		logger:          logger,
	}
}

// CreateChannel declares an exchange and a queue using consumer fields and binds the two together
func (consumer *UserUpdatedConsumer) CreateChannel() (*amqp.Channel, error) {
	channel, err := consumer.conn.Channel()
	if err != nil {
		return nil, err
	}

	// Declare exchange
	err = channel.ExchangeDeclare(
		consumer.exchangeName,
		"fanout", // Exchange type
		true,     // durable?
		false,    // auto-delete?
		false,    // internal exchange
		false,    // no wait?
		nil,      // arguments
	)
	if err != nil {
		return nil, err
	}

	// Declare queue
	queue, err := channel.QueueDeclare(
		consumer.queueName,
		false, // durable?
		false, // delete when unused?
		true,  // exclusive channel?
		false, // no wait?
		nil,   // arguments
	)
	if err != nil {
		return nil, err
	}

	// Bind exchange to the queue
	err = channel.QueueBind(
		queue.Name,
		consumer.routingKey,
		consumer.exchangeName,
		false, // no wait?
		nil,
	)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

// StartConsumer starts up consumer and keeps it listening for messages
func (consumer *UserUpdatedConsumer) StartConsumer() error {
	// Declare exchange, create channel and queue, and bind the two
	channel, err := consumer.CreateChannel()
	if err != nil {
		return err
	}

	defer channel.Close()

	// Receive messages
	messages, err := channel.Consume(
		consumer.queueName,
		consumer.consumerTag,
		true,  // auto-ack?
		false, // exclusive?
		false, // no local?
		false, // no wait?
		nil,
	)
	if err != nil {
		return err
	}

	messagesChannel := make(chan bool)

	go func() {
		for msg := range messages {
			var event events.UserUpdatedEvent

			_ = json.Unmarshal(msg.Body, &event)

			go consumer.handleEvent(event)
		}
	}()

	// Waiting for messages to be sent to queue
	<-messagesChannel

	return nil
}

func (consumer *UserUpdatedConsumer) handleEvent(event events.UserUpdatedEvent) {
	// Check if user already exists in database
	user, err := consumer.usersRepository.GetByID(context.Background(), event.ID)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrRecordNotFound):
			break
		default:
			consumer.logger.Error(err, nil)
			return
		}
	}

	// Create user if it does not exist
	if user.ID == 0 {
		newUser := database.User{
			ID:          event.ID,
			Permissions: event.Permissions,
			Activated:   event.Activated,
			Version:     event.Version,
		}

		_, err := consumer.usersRepository.Create(context.Background(), newUser)
		if err != nil {
			consumer.logger.Error(err, nil)
			return
		}
	} else {
		// Every user should have default permissions so having none means that the permissions were not changed
		if len(event.Permissions) != 0 {
			user.Permissions = event.Permissions
		}

		if event.Activated {
			user.Activated = event.Activated
		}

		err = consumer.usersRepository.Update(context.Background(), user)
		if err != nil {
			consumer.logger.Error(err, nil)
			return
		}
	}
}
