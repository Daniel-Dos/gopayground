package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/kafka"
	"github.com/Daniel-Dos/gopayground/internal/models"
	"github.com/Daniel-Dos/gopayground/internal/producer"
	"github.com/Daniel-Dos/gopayground/internal/validator"

	"github.com/google/uuid"
)

const maxFileSize = 10 * 1024 * 1024

// flags contém as flags de linha de comando para o subcomando "publish".
type flags struct {
	paymentID   string
	status      string
	amount      float64
	currency    string
	description string
	topic       string
	brokers     string
	payload     string
	file        string
	count       int
	rate        int
	dryRun      bool
	jsonOutput  bool
}

// newPublishFlagSet cria um FlagSet para o subcomando "publish" e
// vincula todas as flags específicas ao struct flags fornecido.
func newPublishFlagSet(f *flags) *flag.FlagSet {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)

	fs.StringVar(&f.paymentID, "payment-id", "", "UUID v4 do pagamento (auto-gerado se vazio)")
	fs.StringVar(&f.status, "status", "confirmed", "Status: pending|confirmed|failed|refunded")
	fs.Float64Var(&f.amount, "amount", 100.00, "Valor do pagamento > 0")
	fs.StringVar(&f.currency, "currency", "BRL", "ISO 4217")
	fs.StringVar(&f.description, "description", "", "Descricao opcional (max 255 chars)")

	fs.StringVar(&f.topic, "topic", "payment.events", "Topico Kafka")
	fs.StringVar(&f.brokers, "brokers", "localhost:9092", "Brokers Kafka separados por virgula")

	fs.StringVar(&f.payload, "payload", "", "JSON payload direto (sobrescreve flags individuais)")
	fs.StringVar(&f.file, "file", "", "Arquivo JSON com array de eventos")
	fs.IntVar(&f.count, "count", 1, "Numero de eventos em bulk mode")
	fs.IntVar(&f.rate, "rate", 0, "Eventos por segundo em bulk mode (0 = sem limite)")

	fs.BoolVar(&f.dryRun, "dry-run", false, "Apenas exibir JSON sem publicar")
	fs.BoolVar(&f.jsonOutput, "json-output", false, "Saida em JSON (para scripting)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Uso: producer publish [flags]\n\nFlags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nStdin:\n")
		fmt.Fprintf(os.Stderr, "  echo '{\"payment_id\":\"...\",\"status\":\"confirmed\"}' | producer publish\n")
	}

	return fs
}

// parsePublishFlags converte os args (sem subcomando) em um struct flags.
func parsePublishFlags(args []string) (flags, error) {
	var f flags
	fs := newPublishFlagSet(&f)
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	return f, nil
}

func isStdinPipe() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

func readStdin() ([]byte, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("stdin read error: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("stdin is empty")
	}
	return data, nil
}

func readEventsFromFile(path string) ([]*models.PaymentEvent, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file error: %w", err)
	}
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file error: %w", err)
	}

	var events []*models.PaymentEvent
	if err := json.Unmarshal(data, &events); err != nil {
		var single models.PaymentEvent
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("json unmarshal error: %w", err)
		}
		events = []*models.PaymentEvent{&single}
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("no events found in file")
	}

	return events, nil
}

func unmarshalEvents(data []byte) ([]*models.PaymentEvent, error) {
	var events []*models.PaymentEvent
	if err := json.Unmarshal(data, &events); err != nil {
		var single models.PaymentEvent
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("json unmarshal error: %w", err)
		}
		events = []*models.PaymentEvent{&single}
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events found in payload")
	}
	return events, nil
}

func getEvents(f flags) ([]*models.PaymentEvent, error) {
	if f.payload != "" {
		return unmarshalEvents([]byte(f.payload))
	}

	if isStdinPipe() {
		data, err := readStdin()
		if err != nil {
			return nil, err
		}
		return unmarshalEvents(data)
	}

	if f.file != "" {
		return readEventsFromFile(f.file)
	}

	if f.count > 1 {
		return producer.GenerateBulkEvents(f.count), nil
	}

	event := producer.GenerateEvent(f.paymentID, f.status, f.amount, f.currency, f.description)
	return []*models.PaymentEvent{event}, nil
}

func printEvent(event *models.PaymentEvent, indent string) string {
	data, _ := json.MarshalIndent(event, indent, "  ")
	return string(data)
}

func printEvents(events []*models.PaymentEvent, jsonOutput bool) {
	if jsonOutput {
		for _, event := range events {
			data, _ := json.Marshal(event)
			fmt.Println(string(data))
		}
		return
	}

	for i, event := range events {
		if i > 0 {
			fmt.Println("---")
		}
		fmt.Println(printEvent(event, ""))
	}
}

func printResults(results []producer.Result, jsonOutput bool) bool {
	hasError := false

	for _, r := range results {
		if jsonOutput {
			printResultJSON(r)
		} else {
			printResultText(r)
		}
		if r.Error != nil {
			hasError = true
		}
	}

	return hasError
}

func printResultText(r producer.Result) {
	if r.Error != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed %s → %v\n", r.Event.PaymentID, r.Error)
		return
	}
	fmt.Printf("✓ Published %s → partition %d, offset %d\n", r.Event.PaymentID, r.Partition, r.Offset)
}

func printResultJSON(r producer.Result) {
	if r.Error != nil {
		out := struct {
			Status    string `json:"status"`
			PaymentID string `json:"payment_id"`
			Error     string `json:"error"`
		}{
			Status:    "error",
			PaymentID: r.Event.PaymentID,
			Error:     r.Error.Error(),
		}
		data, _ := json.Marshal(out)
		fmt.Fprintln(os.Stderr, string(data))
		return
	}

	out := struct {
		Status    string `json:"status"`
		PaymentID string `json:"payment_id"`
		Partition int32  `json:"partition"`
		Offset    int64  `json:"offset"`
	}{
		Status:    "success",
		PaymentID: r.Event.PaymentID,
		Partition: r.Partition,
		Offset:    r.Offset,
	}
	data, _ := json.Marshal(out)
	fmt.Println(string(data))
}

func main() {
	os.Exit(run())
}

// serveFlags contém as flags de linha de comando para o subcomando "serve".
type serveFlags struct {
	port    string
	brokers string
	topic   string
}

// newServeFlagSet cria um FlagSet para o subcomando "serve" e
// vincula todas as flags específicas ao struct serveFlags fornecido.
func newServeFlagSet(f *serveFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)

	fs.StringVar(&f.port, "port", "8082", "HTTP server port")
	fs.StringVar(&f.brokers, "brokers", "localhost:9092", "Brokers Kafka separados por virgula")
	fs.StringVar(&f.topic, "topic", "payment.events", "Topico Kafka")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Uso: producer serve [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	return fs
}

// parseServeFlagsArgs converte os args (sem subcomando) em um struct serveFlags.
func parseServeFlagsArgs(args []string) (serveFlags, error) {
	var f serveFlags
	fs := newServeFlagSet(&f)
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	return f, nil
}

func run() int {
	args := os.Args[1:]

	// Extrai subcomando se presente. Um subcomando válido é o primeiro argumento
	// que não começa com '-'. Isso garante que "producer publish --count 10",
	// "producer serve --port 8082" e "producer --flag value" funcionem.
	sub, rest := parseSubcommand(args)

	switch sub {
	case "serve":
		return runServe(rest)
	case "publish":
		return runPublish(rest)
	case "":
		// No subcommand given; treat remaining args as publish flags.
		return runPublish(rest)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\n", sub)
		fmt.Fprintf(os.Stderr, "Usage: producer [publish|serve] [flags]\n")
		return 1
	}
}

// parseSubcommand separa o subcomando (primeiro argumento não-flag) do resto.
// Retorna o subcomando e os argumentos restantes (sem o subcomando).
func parseSubcommand(args []string) (subcommand string, rest []string) {
	if len(args) == 0 {
		return "", nil
	}
	if args[0] == "" || args[0][0] == '-' {
		return "", args
	}
	return args[0], args[1:]
}

func runPublish(args []string) int {
	f, err := parsePublishFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\ninterrupted, shutting down...")
		cancel()
	}()

	events, err := getEvents(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	v := validator.New()

	if f.dryRun {
		printEvents(events, f.jsonOutput)
		return 0
	}

	saramaCfg := kafka.NewProducerSaramaConfig(kafka.DefaultProducerConfig())
	syncProducer, err := kafka.NewSyncProducerWithRetry(ctx, strings.Split(f.brokers, ","), saramaCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot connect to Kafka: %v\n", err)
		return 1
	}
	defer syncProducer.Close()

	svc := producer.New(syncProducer, f.topic, v)
	results := svc.Publish(ctx, events, f.rate)

	if printResults(results, f.jsonOutput) {
		return 1
	}

	return 0
}

// runServe inicia um servidor HTTP de longa duração que expõe endpoints de publicação Kafka.
// Este é o modo usado no docker-compose para o serviço "producer".
func runServe(args []string) int {
	f, err := parseServeFlagsArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	// Logger estruturado (mesmo padrão do consumidor e UI)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	logger.Info("iniciando servidor HTTP do produtor",
		"port", f.port,
		"brokers", f.brokers,
		"topic", f.topic,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Captura SIGINT/SIGTERM para desligamento gracioso
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("sinal de desligamento recebido", "signal", sig.String())
		cancel()
	}()

	// Conectar ao Kafka com retry
	saramaCfg := kafka.NewProducerSaramaConfig(kafka.DefaultProducerConfig())
	syncProducer, err := kafka.NewSyncProducerWithRetry(ctx, strings.Split(f.brokers, ","), saramaCfg)
	if err != nil {
		logger.Error("cannot connect to Kafka", "error", err)
		return 1
	}
	defer func() {
		if err := syncProducer.Close(); err != nil {
			logger.Error("kafka producer close error", "error", err)
		}
	}()

	v := validator.New()
	svc := producer.New(syncProducer, f.topic, v)

	// Publica 10 eventos aleatórios na inicialização (melhor esforço)
	go func() {
		startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer startupCancel()

		events := producer.GenerateBulkEvents(10)
		results := svc.Publish(startupCtx, events, 0)
		published := 0
		for _, r := range results {
			if r.Error == nil {
				published++
			}
		}
		logger.Info("publicação inicial concluída", "total", len(events), "published", published)
	}()

	// Configuração do servidor HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("POST /publish", handlePublish(svc, logger))
	mux.HandleFunc("POST /publish/bulk", handlePublishBulk(svc, logger))
	mux.HandleFunc("GET /healthz", handleHealthz(logger))

	addr := ":" + f.port
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Goroutine de desligamento gracioso
	go func() {
		<-ctx.Done()
		logger.Info("shutting down HTTP server")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP server shutdown error", "error", err)
		}
	}()

	logger.Info("producer server ready", "addr", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("HTTP server error", "error", err)
		return 1
	}

	logger.Info("producer server stopped")
	return 0
}

// Tipos para os handlers HTTP do produtor

type publishRequest struct {
	PaymentID   string  `json:"payment_id"`
	Status      string  `json:"status"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Description string  `json:"description"`
}

type publishResponse struct {
	Status    string `json:"status"`
	PaymentID string `json:"payment_id"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
}

type bulkPublishRequest struct {
	Count int `json:"count"`
}

type bulkPublishItem struct {
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
	Partition int32  `json:"partition,omitempty"`
	Offset    int64  `json:"offset,omitempty"`
	Error     string `json:"error,omitempty"`
}

// validStatuses contém os valores de status de pagamento permitidos.
var validStatuses = map[string]bool{
	"pending":   true,
	"confirmed": true,
	"failed":    true,
	"refunded":  true,
}

// handlePublish retorna um handler HTTP que publica um único evento de pagamento no Kafka.
func handlePublish(svc producer.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate Content-Type
		ct := r.Header.Get("Content-Type")
		if ct != "" && ct != "application/json" && ct != "application/json; charset=utf-8" {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		var req publishRequest
		r.Body = http.MaxBytesReader(w, r.Body, 65536) // 64KB limit
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}

		// Validation
		if req.Status != "" && !validStatuses[req.Status] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status: must be one of pending, confirmed, failed, refunded"})
			return
		}
		if req.Amount <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "amount must be greater than zero"})
			return
		}
		req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
		if len(req.Currency) != 3 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "currency must be a 3-letter code"})
			return
		}
		if len(req.Description) > 255 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description too long (max 255 characters)"})
			return
		}

		// Montar o evento de pagamento
		if req.PaymentID == "" {
			req.PaymentID = uuid.New().String()
		}
		if req.Status == "" {
			req.Status = "pending"
		}

		event := &models.PaymentEvent{
			PaymentID:   req.PaymentID,
			Status:      req.Status,
			Amount:      req.Amount,
			Currency:    req.Currency,
			Description: req.Description,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}

		results := svc.Publish(ctx, []*models.PaymentEvent{event}, 0)
		if len(results) == 0 {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no result returned"})
			return
		}

		if results[0].Error != nil {
			logger.Error("kafka publish error", "payment_id", event.PaymentID, "error", results[0].Error)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "kafka publish failed"})
			return
		}

		logger.Info("payment published",
			"payment_id", event.PaymentID,
			"partition", results[0].Partition,
			"offset", results[0].Offset,
		)

		writeJSON(w, http.StatusOK, publishResponse{
			Status:    "published",
			PaymentID: event.PaymentID,
			Partition: results[0].Partition,
			Offset:    results[0].Offset,
		})
	}
}

// handlePublishBulk retorna um handler HTTP que gera e publica N eventos de pagamento.
func handlePublishBulk(svc producer.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate Content-Type
		ct := r.Header.Get("Content-Type")
		if ct != "" && ct != "application/json" && ct != "application/json; charset=utf-8" {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
			return
		}

		var req bulkPublishRequest
		r.Body = http.MaxBytesReader(w, r.Body, 4096) // 4KB limit
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}

		if req.Count < 1 || req.Count > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "count must be between 1 and 100"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		events := producer.GenerateBulkEvents(req.Count)
		results := svc.Publish(ctx, events, 0)

		items := make([]bulkPublishItem, 0, len(results))
		for _, r := range results {
			item := bulkPublishItem{
				PaymentID: r.Event.PaymentID,
				Status:    r.Event.Status,
			}
			if r.Error != nil {
				item.Error = r.Error.Error()
			} else {
				item.Partition = r.Partition
				item.Offset = r.Offset
			}
			items = append(items, item)
		}

		logger.Info("bulk publish completed",
			"requested", req.Count,
			"published", len(items),
		)

		writeJSON(w, http.StatusOK, items)
	}
}

// handleHealthz retorna uma resposta simples de health check.
func handleHealthz(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// writeJSON é um helper para escrever uma resposta JSON com código de status.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
