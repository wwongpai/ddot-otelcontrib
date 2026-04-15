"""
fraud-detection-service — Python/Flask
Instrumented with OpenTelemetry SDK (traces + metrics + logs via OTLP gRPC).
No Datadog SDK anywhere.
"""

import logging
import os
import random
import time
import uuid
from collections import deque
from datetime import datetime, timezone

from flask import Flask, jsonify, request

# ── OTel SDK imports ──────────────────────────────────────────────────────────
from opentelemetry import metrics, trace
from opentelemetry.trace import StatusCode
from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.flask import FlaskInstrumentor
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

# ── Configuration from environment ───────────────────────────────────────────
OTLP_ENDPOINT = os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
SERVICE_NAME = os.environ.get("OTEL_SERVICE_NAME", "fraud-detection-service")
SERVICE_VERSION = os.environ.get("SERVICE_VERSION", "1.0.0")

# Build Resource from env
resource_attrs = {
    "service.name": SERVICE_NAME,
    "service.version": SERVICE_VERSION,
}
for pair in os.environ.get("OTEL_RESOURCE_ATTRIBUTES", "").split(","):
    if "=" in pair:
        k, v = pair.split("=", 1)
        resource_attrs[k.strip()] = v.strip()

resource = Resource.create(resource_attrs)

# ── Traces ────────────────────────────────────────────────────────────────────
tracer_provider = TracerProvider(resource=resource)
tracer_provider.add_span_processor(
    BatchSpanProcessor(OTLPSpanExporter(endpoint=OTLP_ENDPOINT, insecure=True))
)
trace.set_tracer_provider(tracer_provider)
tracer = trace.get_tracer(__name__)

# ── Metrics ───────────────────────────────────────────────────────────────────
meter_provider = MeterProvider(
    resource=resource,
    metric_readers=[
        PeriodicExportingMetricReader(
            OTLPMetricExporter(endpoint=OTLP_ENDPOINT, insecure=True),
            export_interval_millis=10_000,
        )
    ],
)
metrics.set_meter_provider(meter_provider)
meter = metrics.get_meter(__name__)

fraud_score_counter = meter.create_counter(
    "fraud.score.total",
    description="Total fraud scores by risk level",
)
high_risk_counter = meter.create_counter(
    "fraud.high_risk_alerts.total",
    description="Total high-risk fraud alerts triggered",
)
scoring_histogram = meter.create_histogram(
    "fraud.scoring.latency.ms",
    description="Fraud scoring latency",
    unit="ms",
)

# ── Logs (stdout — collected by Datadog agent / OTel Collector filelog) ───────
logging.basicConfig(
    level=logging.INFO,
    format="[%(asctime)s] %(levelname)-5s %(name)s - %(message)s",
)
logger = logging.getLogger("fraud_detection")

# ── Flask app ─────────────────────────────────────────────────────────────────
app = Flask(__name__)
FlaskInstrumentor().instrument_app(app)

# In-memory alert ring buffer (max 100)
alerts: deque = deque(maxlen=100)


# ── Risk scoring logic ────────────────────────────────────────────────────────
def compute_risk(amount: float) -> tuple[str, float, str]:
    """Deterministic-ish risk scoring for demo purposes."""
    # 5% unconditional high-risk regardless of amount
    if random.random() < 0.05:
        return "high", round(random.uniform(0.85, 0.99), 2), "Anomalous transaction pattern detected"

    if amount > 100_000:
        r = random.random()
        if r < 0.30:
            return "high", round(random.uniform(0.75, 0.95), 2), "Large transfer exceeds risk threshold"
        if r < 0.70:
            return "medium", round(random.uniform(0.45, 0.74), 2), "Large transfer under review"
        return "low", round(random.uniform(0.10, 0.44), 2), "Large transfer cleared"

    if random.random() < 0.10:
        return "medium", round(random.uniform(0.35, 0.65), 2), "Unusual transaction velocity"

    return "low", round(random.uniform(0.01, 0.34), 2), "Transaction within normal parameters"


# ── Endpoints ─────────────────────────────────────────────────────────────────
@app.route("/score", methods=["POST"])
def score():
    start = time.time()
    data = request.get_json(force=True, silent=True) or {}

    transaction_id = data.get("transactionId", str(uuid.uuid4()))
    from_account = data.get("fromAccount", "")
    to_account = data.get("toAccount", "")
    currency = data.get("currency", "THB")

    # Parse amount — record exception on bad input so Error Tracking picks it up
    try:
        amount = float(data.get("amount", 0))
        if amount < 0:
            raise ValueError(f"Negative transfer amount: {amount}")
    except (TypeError, ValueError) as exc:
        with tracer.start_as_current_span("fraud.score") as span:
            span.record_exception(exc)
            span.set_status(StatusCode.ERROR, str(exc))
            span.set_attribute("transaction.id", transaction_id)
            logger.error("Invalid amount in scoring request: %s", exc)
            return jsonify({"error": str(exc)}), 400

    with tracer.start_as_current_span("fraud.score") as span:
        span.set_attribute("transaction.id", transaction_id)
        span.set_attribute("transaction.from_account", from_account)
        span.set_attribute("transaction.to_account", to_account)
        span.set_attribute("transaction.amount", amount)
        span.set_attribute("transaction.currency", currency)

        # Artificial 10–50 ms latency — makes traces visually interesting
        time.sleep(random.uniform(0.010, 0.050))

        risk_level, score_val, reason = compute_risk(amount)

        span.set_attribute("fraud.risk_level", risk_level)
        span.set_attribute("fraud.score", score_val)

        fraud_score_counter.add(1, {"risk_level": risk_level, "currency": currency})

        if risk_level == "high":
            # Record as exception so Datadog Error Tracking groups it
            fraud_exc = Exception(
                f"HighRiskFraudAlert: transaction {transaction_id} from {from_account} "
                f"scored {score_val:.2f} — {reason}"
            )
            span.record_exception(fraud_exc)
            span.set_status(StatusCode.ERROR, reason)
            span.add_event(
                "high_risk_detected",
                {"transaction_id": transaction_id, "score": score_val, "reason": reason},
            )
            high_risk_counter.add(1, {"currency": currency})
            alert = {
                "id": str(uuid.uuid4()),
                "transactionId": transaction_id,
                "fromAccount": from_account,
                "riskLevel": risk_level,
                "reason": reason,
                "timestamp": datetime.now(timezone.utc).isoformat(),
            }
            alerts.append(alert)
            logger.warning(
                "High-risk transaction: id=%s score=%.2f reason=%s",
                transaction_id, score_val, reason,
            )
        else:
            logger.info(
                "Transaction scored: id=%s risk=%s score=%.2f",
                transaction_id, risk_level, score_val,
            )

        latency_ms = (time.time() - start) * 1000
        scoring_histogram.record(latency_ms, {"risk_level": risk_level})

        return jsonify({
            "transactionId": transaction_id,
            "riskLevel": risk_level,
            "score": score_val,
            "reason": reason,
            "timestamp": datetime.now(timezone.utc).isoformat(),
        })


@app.route("/alerts", methods=["GET"])
def get_alerts():
    return jsonify(list(alerts)[-20:])


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "UP"})


# ── Entry point ───────────────────────────────────────────────────────────────
if __name__ == "__main__":
    logger.info("fraud-detection-service starting on port 3000 (service=%s)", SERVICE_NAME)
    app.run(host="0.0.0.0", port=3000, debug=False)
