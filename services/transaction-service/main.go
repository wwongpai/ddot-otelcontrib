package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// ── Data model ────────────────────────────────────────────────────────────────

type Transaction struct {
	ID          string    `json:"id"`
	FromAccount string    `json:"fromAccount"`
	ToAccount   string    `json:"toAccount"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"` // "PENDING", "COMPLETED", "FAILED"
	CreatedAt   time.Time `json:"createdAt"`
	Error       string    `json:"error,omitempty"`
}

type TransferRequest struct {
	FromAccount string  `json:"fromAccount"`
	ToAccount   string  `json:"toAccount"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
}

type FraudScoreRequest struct {
	TransactionID string  `json:"transactionId"`
	FromAccount   string  `json:"fromAccount"`
	ToAccount     string  `json:"toAccount"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
}

type FraudScoreResponse struct {
	TransactionID string  `json:"transactionId"`
	RiskLevel     string  `json:"riskLevel"`
	Score         float64 `json:"score"`
	Reason        string  `json:"reason"`
	Timestamp     string  `json:"timestamp"`
}

type NotifyRequest struct {
	TransactionID string  `json:"transactionId"`
	FromAccount   string  `json:"fromAccount"`
	ToAccount     string  `json:"toAccount"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	Status        string  `json:"status"`
}

// ── In-memory store ───────────────────────────────────────────────────────────

var store sync.Map // key: transaction ID → *Transaction

// ── Service URL helpers ───────────────────────────────────────────────────────

func fraudServiceURL() string {
	if v := os.Getenv("FRAUD_SERVICE_URL"); v != "" {
		return v
	}
	return "http://fraud-detection-service:3000"
}

func notificationServiceURL() string {
	if v := os.Getenv("NOTIFICATION_SERVICE_URL"); v != "" {
		return v
	}
	return "http://notification-service:8080"
}

// ── OTel metric instruments ───────────────────────────────────────────────────

var (
	transferCounter metric.Int64Counter
	latencyHist     metric.Float64Histogram

	// lastAmount is observed by the gauge callback.
	lastAmount float64
	amountMu   sync.Mutex
)

func initMetrics() {
	meter := otel.GetMeterProvider().Meter("transaction-service")

	var err error

	transferCounter, err = meter.Int64Counter(
		"transfer.total",
		metric.WithDescription("Total number of transfer attempts"),
	)
	if err != nil {
		panic("failed to create transfer.total counter: " + err.Error())
	}

	latencyHist, err = meter.Float64Histogram(
		"transfer.latency.ms",
		metric.WithDescription("Transfer processing latency in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		panic("failed to create transfer.latency.ms histogram: " + err.Error())
	}

	_, err = meter.Float64ObservableGauge(
		"transfer.amount.thb",
		metric.WithDescription("Most recent transfer amount (THB)"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			amountMu.Lock()
			v := lastAmount
			amountMu.Unlock()
			o.Observe(v)
			return nil
		}),
	)
	if err != nil {
		panic("failed to create transfer.amount.thb gauge: " + err.Error())
	}
}

// ── Logger helper ─────────────────────────────────────────────────────────────

func appLogger() otellog.Logger {
	return global.GetLoggerProvider().Logger(
		"transaction-service",
		otellog.WithInstrumentationVersion("1.0.0"),
	)
}

func logInfo(ctx context.Context, msg string, attrs ...otellog.KeyValue) {
	emitLog(ctx, otellog.SeverityInfo, msg, attrs...)
}

func logWarn(ctx context.Context, msg string, attrs ...otellog.KeyValue) {
	emitLog(ctx, otellog.SeverityWarn, msg, attrs...)
}

func logError(ctx context.Context, msg string, attrs ...otellog.KeyValue) {
	emitLog(ctx, otellog.SeverityError, msg, attrs...)
}

func emitLog(ctx context.Context, sev otellog.Severity, msg string, attrs ...otellog.KeyValue) {
	var r otellog.Record
	r.SetTimestamp(time.Now())
	r.SetSeverity(sev)
	r.SetBody(otellog.StringValue(msg))
	r.AddAttributes(attrs...)
	appLogger().Emit(ctx, r)
}

// ── HTTP client with OTel transport ──────────────────────────────────────────

var httpClient = &http.Client{
	Transport: otelhttp.NewTransport(http.DefaultTransport),
	Timeout:   10 * time.Second,
}

// ── Transfer handler ──────────────────────────────────────────────────────────

func handleTransfer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	start := time.Now()

	var req TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		decodeErr := fmt.Errorf("TransferDecodeError: invalid request body: %w", err)
		span.RecordError(decodeErr)
		span.SetStatus(codes.Error, decodeErr.Error())
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.FromAccount == "" || req.ToAccount == "" || req.Amount <= 0 {
		validationErr := fmt.Errorf("TransferValidationError: fromAccount=%q toAccount=%q amount=%.2f — all fields required and amount must be > 0", req.FromAccount, req.ToAccount, req.Amount)
		span.RecordError(validationErr)
		span.SetStatus(codes.Error, validationErr.Error())
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "fromAccount, toAccount, and amount > 0 are required",
		})
		return
	}
	if req.Currency == "" {
		req.Currency = "THB"
	}

	txn := &Transaction{
		ID:          uuid.New().String(),
		FromAccount: req.FromAccount,
		ToAccount:   req.ToAccount,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Status:      "PENDING",
		CreatedAt:   time.Now().UTC(),
	}
	store.Store(txn.ID, txn)

	span.SetAttributes(
		attribute.String("transaction.id", txn.ID),
		attribute.String("transaction.from_account", txn.FromAccount),
		attribute.String("transaction.to_account", txn.ToAccount),
		attribute.Float64("transaction.amount", txn.Amount),
		attribute.String("transaction.currency", txn.Currency),
	)
	span.AddEvent("transfer.initiated", trace.WithAttributes(
		attribute.String("transaction.id", txn.ID),
		attribute.String("from_account", txn.FromAccount),
		attribute.String("to_account", txn.ToAccount),
		attribute.Float64("amount", txn.Amount),
		attribute.String("currency", txn.Currency),
	))
	logInfo(ctx, "Transfer initiated",
		otellog.String("transaction_id", txn.ID),
		otellog.String("from_account", txn.FromAccount),
		otellog.String("to_account", txn.ToAccount),
		otellog.Float64("amount", txn.Amount),
		otellog.String("currency", txn.Currency),
	)

	// 15 % chance of insufficient-funds failure.
	// #nosec G404 — intentional use of weak random for simulation
	if rand.Float64() < 0.15 { //nolint:gosec
		txn.Status = "FAILED"
		txn.Error = "Insufficient funds"
		store.Store(txn.ID, txn)

		insufficientErr := fmt.Errorf("InsufficientFundsError: account %s has insufficient funds for transfer of %.2f %s to %s",
			txn.FromAccount, txn.Amount, txn.Currency, txn.ToAccount)
		span.RecordError(insufficientErr)
		span.SetStatus(codes.Error, insufficientErr.Error())
		span.AddEvent("transfer.failed", trace.WithAttributes(
			attribute.String("transaction.id", txn.ID),
			attribute.String("error.message", txn.Error),
		))
		logWarn(ctx, "Transfer failed: Insufficient funds",
			otellog.String("transaction_id", txn.ID),
			otellog.String("reason", txn.Error),
		)

		recordMetrics(ctx, txn, start)
		sleepRandom(300, 900)
		writeJSON(w, http.StatusPaymentRequired, txn)
		return
	}

	txn.Status = "COMPLETED"
	store.Store(txn.ID, txn)

	span.AddEvent("transfer.completed", trace.WithAttributes(
		attribute.String("transaction.id", txn.ID),
	))
	logInfo(ctx, "Transfer completed",
		otellog.String("transaction_id", txn.ID),
	)

	// Call downstream services (best-effort; errors are logged but don't fail
	// the transfer response).
	callFraudDetection(ctx, txn)
	callNotification(ctx, txn)

	recordMetrics(ctx, txn, start)
	sleepRandom(300, 900)

	writeJSON(w, http.StatusCreated, txn)
}

func callFraudDetection(ctx context.Context, txn *Transaction) {
	span := trace.SpanFromContext(ctx)

	fraudReq := FraudScoreRequest{
		TransactionID: txn.ID,
		FromAccount:   txn.FromAccount,
		ToAccount:     txn.ToAccount,
		Amount:        txn.Amount,
		Currency:      txn.Currency,
	}
	body, _ := json.Marshal(fraudReq)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fraudServiceURL()+"/score", bytes.NewReader(body))
	if err != nil {
		span.AddEvent("fraud_detection.request_build_error",
			trace.WithAttributes(attribute.String("error", err.Error())))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		span.AddEvent("fraud_detection.call_failed",
			trace.WithAttributes(attribute.String("error", err.Error())))
		logError(ctx, "Fraud detection call failed",
			otellog.String("transaction_id", txn.ID),
			otellog.String("error", err.Error()),
		)
		return
	}
	defer resp.Body.Close()

	var fraudResp FraudScoreResponse
	if decErr := json.NewDecoder(resp.Body).Decode(&fraudResp); decErr != nil {
		span.AddEvent("fraud_detection.response_decode_error",
			trace.WithAttributes(attribute.String("error", decErr.Error())))
		return
	}

	span.AddEvent("fraud_detection.score_received", trace.WithAttributes(
		attribute.String("transaction.id", fraudResp.TransactionID),
		attribute.String("risk_level", fraudResp.RiskLevel),
		attribute.Float64("score", fraudResp.Score),
		attribute.String("reason", fraudResp.Reason),
	))
	logInfo(ctx, "Fraud score received",
		otellog.String("transaction_id", txn.ID),
		otellog.String("risk_level", fraudResp.RiskLevel),
		otellog.Float64("score", fraudResp.Score),
		otellog.String("reason", fraudResp.Reason),
	)
}

func callNotification(ctx context.Context, txn *Transaction) {
	span := trace.SpanFromContext(ctx)

	notifyReq := NotifyRequest{
		TransactionID: txn.ID,
		FromAccount:   txn.FromAccount,
		ToAccount:     txn.ToAccount,
		Amount:        txn.Amount,
		Currency:      txn.Currency,
		Status:        txn.Status,
	}
	body, _ := json.Marshal(notifyReq)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		notificationServiceURL()+"/notify", bytes.NewReader(body))
	if err != nil {
		span.AddEvent("notification.request_build_error",
			trace.WithAttributes(attribute.String("error", err.Error())))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		span.AddEvent("notification.call_failed",
			trace.WithAttributes(attribute.String("error", err.Error())))
		logWarn(ctx, "Notification call failed",
			otellog.String("transaction_id", txn.ID),
			otellog.String("error", err.Error()),
		)
		return
	}
	defer resp.Body.Close()

	span.AddEvent("notification.sent", trace.WithAttributes(
		attribute.String("transaction.id", txn.ID),
		attribute.Int("http.status_code", resp.StatusCode),
	))
}

// ── GET /transactions/{id} ────────────────────────────────────────────────────

func handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/transactions/")
	v, ok := store.Load(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transaction not found"})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ── GET /transactions/history/{accountId} ────────────────────────────────────

func handleHistory(w http.ResponseWriter, r *http.Request) {
	accountID := strings.TrimPrefix(r.URL.Path, "/transactions/history/")

	var results []*Transaction
	store.Range(func(_, v any) bool {
		txn := v.(*Transaction)
		if txn.FromAccount == accountID || txn.ToAccount == accountID {
			results = append(results, txn)
		}
		return true
	})
	if results == nil {
		results = []*Transaction{}
	}
	writeJSON(w, http.StatusOK, results)
}

// ── GET /health ───────────────────────────────────────────────────────────────

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
}

// ── Routing ───────────────────────────────────────────────────────────────────

func newMux() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /transfer",
		otelhttp.NewHandler(http.HandlerFunc(handleTransfer), "POST /transfer"))

	// /transactions/history/{id} must be registered before /transactions/{id}
	// so the more-specific prefix wins.
	mux.Handle("GET /transactions/history/",
		otelhttp.NewHandler(http.HandlerFunc(handleHistory), "GET /transactions/history"))

	mux.Handle("GET /transactions/",
		otelhttp.NewHandler(http.HandlerFunc(handleGetTransaction), "GET /transactions/:id"))

	mux.Handle("GET /health",
		otelhttp.NewHandler(http.HandlerFunc(handleHealth), "GET /health"))

	return mux
}

// ── Utility helpers ───────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// sleepRandom sleeps a random duration between minMs and maxMs milliseconds.
func sleepRandom(minMs, maxMs int) {
	// #nosec G404 — intentional weak random for demo latency simulation
	d := time.Duration(minMs+rand.Intn(maxMs-minMs)) * time.Millisecond //nolint:gosec
	time.Sleep(d)
}

func recordMetrics(ctx context.Context, txn *Transaction, start time.Time) {
	elapsed := float64(time.Since(start).Milliseconds())

	transferCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", txn.Status),
			attribute.String("currency", txn.Currency),
		),
	)
	latencyHist.Record(ctx, elapsed,
		metric.WithAttributes(
			attribute.String("status", txn.Status),
			attribute.String("currency", txn.Currency),
		),
	)

	amountMu.Lock()
	lastAmount = txn.Amount
	amountMu.Unlock()
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown := initOtel(ctx)
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := shutdown(shutCtx); err != nil {
			fmt.Fprintf(os.Stderr, "OTel shutdown error: %v\n", err)
		}
	}()

	initMetrics()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      newMux(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	fmt.Printf("transaction-service listening on :%s\n", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
