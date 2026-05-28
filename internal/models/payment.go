package models

import "time"

// PaymentEvent representa o evento recebido do Kafka.
type PaymentEvent struct {
	PaymentID   string  `json:"payment_id"   validate:"required,uuid4"`
	Status      string  `json:"status"       validate:"required,oneof=pending confirmed failed refunded"`
	Amount      float64 `json:"amount"       validate:"required,gt=0"`
	Currency    string  `json:"currency"     validate:"required,len=3,uppercase"`
	Description string  `json:"description"  validate:"omitempty,max=255,printascii"`
	Timestamp   string  `json:"timestamp"    validate:"required,rfc3339"`
}

// PaymentStatus representa o status atual no Redis.
type PaymentStatus struct {
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

// PaymentHistory representa o registro no DynamoDB.
type PaymentHistory struct {
	PaymentID   string    `dynamodbav:"payment_id"   json:"payment_id"`
	Status      string    `dynamodbav:"status"       json:"status"`
	Amount      float64   `dynamodbav:"amount"       json:"amount"`
	Currency    string    `dynamodbav:"currency"     json:"currency"`
	Description string    `dynamodbav:"description"  json:"description"`
	Timestamp   string    `dynamodbav:"timestamp"    json:"timestamp"`
	ProcessedAt time.Time `dynamodbav:"processed_at" json:"processed_at"`
	TraceID     string    `dynamodbav:"trace_id"     json:"trace_id"`
}

// NewPaymentHistoryFromEvent cria um PaymentHistory a partir de um PaymentEvent.
func NewPaymentHistoryFromEvent(event *PaymentEvent, traceID string) PaymentHistory {
	return PaymentHistory{
		PaymentID:   event.PaymentID,
		Status:      event.Status,
		Amount:      event.Amount,
		Currency:    event.Currency,
		Description: event.Description,
		Timestamp:   event.Timestamp,
		ProcessedAt: time.Now().UTC(),
		TraceID:     traceID,
	}
}
