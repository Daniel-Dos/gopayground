package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/models"
	"github.com/Daniel-Dos/gopayground/internal/validator"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
)

type SyncProducer interface {
	SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
}

type Service interface {
	Publish(ctx context.Context, events []*models.PaymentEvent, rate int) []Result
}

type Result struct {
	Event     *models.PaymentEvent
	Partition int32
	Offset    int64
	Error     error
}

type service struct {
	producer  SyncProducer
	topic     string
	validator validator.Validator
}

func New(producer SyncProducer, topic string, v validator.Validator) Service {
	return &service{
		producer:  producer,
		topic:     topic,
		validator: v,
	}
}

func (s *service) Publish(ctx context.Context, events []*models.PaymentEvent, rate int) []Result {
	results := make([]Result, 0, len(events))

	var ticker *time.Ticker
	if rate > 0 {
		ticker = time.NewTicker(time.Second / time.Duration(rate))
		defer ticker.Stop()
	}

	for i, event := range events {
		select {
		case <-ctx.Done():
			results = append(results, Result{
				Event: event,
				Error: ctx.Err(),
			})
			return results
		default:
		}

		if ticker != nil && i > 0 {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				results = append(results, Result{
					Event: event,
					Error: ctx.Err(),
				})
				return results
			}
		}

		eventData, err := json.Marshal(event)
		if err != nil {
			results = append(results, Result{Event: event, Error: fmt.Errorf("marshal error: %w", err)})
			continue
		}

		validateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err = s.validator.Validate(validateCtx, eventData)
		cancel()
		if err != nil {
			results = append(results, Result{Event: event, Error: err})
			continue
		}

		msg := &sarama.ProducerMessage{
			Topic: s.topic,
			Key:   sarama.StringEncoder(event.PaymentID),
			Value: sarama.ByteEncoder(eventData),
			Headers: []sarama.RecordHeader{
				{Key: []byte("source"), Value: []byte("cli-producer")},
				{Key: []byte("timestamp"), Value: []byte(time.Now().UTC().Format(time.RFC3339))},
			},
		}

		partition, offset, err := s.producer.SendMessage(msg)
		if err != nil {
			results = append(results, Result{Event: event, Error: err})
			continue
		}

		results = append(results, Result{
			Event:     event,
			Partition: partition,
			Offset:    offset,
		})
	}

	return results
}

func GenerateEvent(paymentID, status string, amount float64, currency, description string) *models.PaymentEvent {
	if paymentID == "" {
		paymentID = uuid.New().String()
	}
	if status == "" {
		status = "confirmed"
	}
	if amount <= 0 {
		amount = 100.00
	}
	if currency == "" {
		currency = "BRL"
	}

	return &models.PaymentEvent{
		PaymentID:   paymentID,
		Status:      status,
		Amount:      amount,
		Currency:    currency,
		Description: description,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func GenerateBulkEvents(count int) []*models.PaymentEvent {
	events := make([]*models.PaymentEvent, count)
	for i := 0; i < count; i++ {
		events[i] = GenerateEvent(
			uuid.New().String(),
			"confirmed",
			float64(i+1)*10.0,
			"BRL",
			fmt.Sprintf("Bulk event %d of %d", i+1, count),
		)
	}
	return events
}
