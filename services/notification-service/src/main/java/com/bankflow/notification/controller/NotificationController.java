package com.bankflow.notification.controller;

import com.bankflow.notification.model.Notification;
import io.opentelemetry.api.common.AttributeKey;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.StatusCode;
import io.opentelemetry.api.trace.Tracer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

@RestController
@RequestMapping
public class NotificationController {

    private static final Logger log = LoggerFactory.getLogger(NotificationController.class);

    /** Valid notification types. */
    private static final Set<String> VALID_TYPES = Set.of("TRANSFER", "ALERT", "FRAUD_ALERT");

    /** In-memory store: accountId -> list of notifications */
    private final ConcurrentHashMap<String, List<Notification>> store = new ConcurrentHashMap<>();

    private final Tracer tracer;

    public NotificationController(Tracer tracer) {
        this.tracer = tracer;
    }

    // ---------------------------------------------------------------------------
    // POST /notify
    // ---------------------------------------------------------------------------

    @PostMapping("/notify")
    public ResponseEntity<Map<String, Object>> sendNotification(
            @RequestBody NotifyRequest req) {

        Span span = tracer.spanBuilder("notification.send")
                .setAttribute(AttributeKey.stringKey("notification.account_id"), req.accountId())
                .setAttribute(AttributeKey.stringKey("notification.type"), req.type())
                .startSpan();

        try (var scope = span.makeCurrent()) {

            if (req.accountId() == null || req.accountId().isBlank()) {
                span.setStatus(StatusCode.ERROR, "accountId is required");
                return ResponseEntity.badRequest()
                        .body(Map.of("error", "accountId is required"));
            }

            String notificationType = req.type() != null && VALID_TYPES.contains(req.type().toUpperCase())
                    ? req.type().toUpperCase()
                    : "ALERT";

            String notificationId = "NOTIF-" + UUID.randomUUID().toString().replace("-", "").substring(0, 10).toUpperCase();

            Notification notification = new Notification(
                    notificationId,
                    req.accountId(),
                    notificationType,
                    req.message() != null ? req.message() : "",
                    LocalDateTime.now(),
                    true);

            // Thread-safe: initialise the list if this accountId hasn't been seen before
            store.computeIfAbsent(req.accountId(), k -> Collections.synchronizedList(new ArrayList<>()))
                    .add(notification);

            // Simulate dispatch (log-only in demo)
            log.info("Sending notification to {}: [{}] {} (transactionId={})",
                    req.accountId(), notificationType, notification.getMessage(), req.transactionId());

            span.setAttribute(AttributeKey.stringKey("notification.id"), notificationId);

            return ResponseEntity.status(HttpStatus.CREATED)
                    .body(Map.of(
                            "notificationId", notificationId,
                            "status", "SENT"));

        } finally {
            span.end();
        }
    }

    // ---------------------------------------------------------------------------
    // GET /notifications/{accountId}
    // ---------------------------------------------------------------------------

    @GetMapping("/notifications/{accountId}")
    public ResponseEntity<List<Notification>> getNotifications(@PathVariable String accountId) {

        Span span = tracer.spanBuilder("notification.list")
                .setAttribute(AttributeKey.stringKey("notification.account_id"), accountId)
                .startSpan();

        try (var scope = span.makeCurrent()) {
            log.info("Listing notifications for accountId={}", accountId);

            List<Notification> notifications = store.getOrDefault(accountId, List.of());

            span.setAttribute(AttributeKey.longKey("notification.count"), notifications.size());
            return ResponseEntity.ok(new ArrayList<>(notifications));

        } finally {
            span.end();
        }
    }

    // ---------------------------------------------------------------------------
    // GET /health
    // ---------------------------------------------------------------------------

    @GetMapping("/health")
    public ResponseEntity<Map<String, String>> health() {
        return ResponseEntity.ok(Map.of("status", "UP"));
    }

    // ---------------------------------------------------------------------------
    // Request DTO
    // ---------------------------------------------------------------------------

    public record NotifyRequest(
            String accountId,
            String type,
            String message,
            String transactionId) {}
}
