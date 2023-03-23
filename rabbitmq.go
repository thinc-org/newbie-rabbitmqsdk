package gosdk

import (
	"context"
	"encoding/json"
	amqp "github.com/rabbitmq/amqp091-go"
	gosdk "github.com/thinc-org/newbie-gosdk"
	"go.uber.org/zap"
	"time"
)

type RabbitMQ interface {
	Close()
	Listen() ([]byte, error)
	CreateQueue(name string, durable bool, autoDelete bool, exclusive bool, noWait bool, args map[string]interface{}) (amqp.Queue, error)
	CreateExchange(name string, kind string, durable bool, autoDelete bool, internal bool, noWait bool, args map[string]interface{}) error
	BindQueueWithExchange(queueName string, key string, exchangeName string, noWait bool, args map[string]interface{}) error
	CreateMessageChannel(queue string, consumer string, autoAck bool, exclusive bool, noLocal bool, noWait bool, args map[string]interface{}) error
	Publish(exchangeName string, topic string, message any) error
}

func NewRabbitMQ(conn *amqp.Connection) (RabbitMQ, error) {
	logger, _ := gosdk.NewLogger()

	ch, err := conn.Channel()
	if err != nil {
		logger.Error(
			"Error when creating channel",
			zap.Error(err),
		)
		return nil, err
	}

	return &rabbitmq{
		logger:  logger,
		conn:    conn,
		channel: ch,
	}, nil
}

type rabbitmq struct {
	logger      *zap.Logger
	conn        *amqp.Connection
	channel     *amqp.Channel
	errCh       <-chan *amqp.Error
	messageChan <-chan amqp.Delivery
}

func (r *rabbitmq) Close() {
	r.channel.Close()
}

func (r *rabbitmq) Publish(exchangeName string, key string, message any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, err := json.Marshal(message)
	if err != nil {
		r.logger.Error(
			"error while convert message to json",
			zap.Error(err),
			zap.String("exchange_name", exchangeName),
			zap.String("key", key),
		)
		return err
	}

	if err := r.channel.PublishWithContext(ctx,
		exchangeName,
		key,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	); err != nil {
		r.logger.Error(
			"error while publish message to rabbitmq",
			zap.Error(err),
			zap.String("exchange_name", exchangeName),
			zap.String("key", key),
		)
		return err
	}

	r.logger.Info(
		"successfully publish message to rabbitmq",
		zap.String("exchange_name", exchangeName),
		zap.String("key", key),
	)

	return nil
}

func (r *rabbitmq) CreateQueue(name string, durable bool, autoDelete bool, exclusive bool, noWait bool, args map[string]interface{}) (amqp.Queue, error) {
	queue, err := r.channel.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
	if err != nil {
		r.logger.Error(
			"error while creating queue",
			zap.Error(err),
			zap.String("name", name),
			zap.Bool("durable", durable),
			zap.Bool("autoDelete", autoDelete),
			zap.Bool("exclusive", exclusive),
			zap.Bool("noWait", noWait),
		)
		return amqp.Queue{}, err
	}

	return queue, nil
}

func (r *rabbitmq) CreateExchange(name string, kind string, durable bool, autoDelete bool, internal bool, noWait bool, args map[string]interface{}) error {
	if err := r.channel.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args); err != nil {
		r.logger.Error(
			"error while creating exchange",
			zap.Error(err),
			zap.String("name", name),
			zap.String("kind", kind),
			zap.Bool("durable", durable),
			zap.Bool("autoDelete", autoDelete),
			zap.Bool("internal", internal),
			zap.Bool("noWait", noWait),
		)

		return err
	}

	return nil
}

func (r *rabbitmq) BindQueueWithExchange(queueName string, key string, exchangeName string, noWait bool, args map[string]interface{}) error {
	if err := r.channel.QueueBind(queueName, key, exchangeName, noWait, args); err != nil {
		r.logger.Error(
			"error while queue binding",
			zap.Error(err),
		)

		return err
	}

	r.logger.Info(
		"binding queue",
		zap.String("queue_name", queueName),
		zap.String("exchange", exchangeName),
		zap.String("key", key),
		zap.Bool("noWait", noWait),
	)

	return nil
}

func (r *rabbitmq) CreateMessageChannel(queue string, consumer string, autoAck bool, exclusive bool, noLocal bool, noWait bool, args map[string]interface{}) error {
	var err error
	r.messageChan, err = r.channel.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
	if err != nil {

		r.logger.Error(
			"error when consuming message",
			zap.Error(err),
			zap.String("queue", queue),
			zap.String("consumer", consumer),
			zap.Bool("autoAck", autoAck),
			zap.Bool("exclusive", exclusive),
			zap.Bool("noLocal", noLocal),
			zap.Bool("noWait", noWait),
		)
		return err
	}

	return nil
}

func (r *rabbitmq) Listen() ([]byte, error) {
	select {
	case err := <-r.errCh:
		r.logger.Error(
			"error while receive message from rabbitmq",
			zap.Error(err),
		)
		return nil, err
	case msg := <-r.messageChan:
		return msg.Body, nil
	}
}
