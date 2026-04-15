package com.bankflow.account.controller;

import com.bankflow.account.model.Account;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.api.common.AttributeKey;
import io.opentelemetry.api.common.Attributes;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.math.BigDecimal;
import java.time.LocalDateTime;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

@RestController
@RequestMapping
public class AccountController {

    private static final Logger log = LoggerFactory.getLogger(AccountController.class);

    private final ConcurrentHashMap<String, Account> store = new ConcurrentHashMap<>();
    private final Tracer tracer;

    public AccountController(Tracer tracer) {
        this.tracer = tracer;
        seedAccounts();
    }

    // ---------------------------------------------------------------------------
    // Seed data
    // ---------------------------------------------------------------------------

    private void seedAccounts() {
        store.put("ACC001", new Account(
                "ACC001", "OWN001", "Somchai Jaidee",
                new BigDecimal("10000.00"), "THB", "ACTIVE",
                LocalDateTime.of(2024, 1, 15, 9, 0)));

        store.put("ACC002", new Account(
                "ACC002", "OWN002", "Siriporn Kaewkla",
                new BigDecimal("75000.00"), "THB", "ACTIVE",
                LocalDateTime.of(2024, 2, 3, 14, 30)));

        store.put("ACC003", new Account(
                "ACC003", "OWN003", "Wichai Thongsuk",
                new BigDecimal("250000.00"), "THB", "ACTIVE",
                LocalDateTime.of(2024, 3, 20, 11, 15)));

        store.put("ACC004", new Account(
                "ACC004", "OWN004", "Malee Pongpan",
                new BigDecimal("500000.00"), "THB", "ACTIVE",
                LocalDateTime.of(2024, 4, 5, 8, 45)));

        store.put("ACC005", new Account(
                "ACC005", "OWN005", "Narong Yodrak",
                new BigDecimal("32500.00"), "THB", "FROZEN",
                LocalDateTime.of(2024, 5, 12, 16, 0)));

        log.info("Seeded {} accounts into in-memory store", store.size());
    }

    // ---------------------------------------------------------------------------
    // Endpoints
    // ---------------------------------------------------------------------------

    @GetMapping("/accounts/{id}")
    public ResponseEntity<Account> getAccount(@PathVariable String id) {
        Span span = tracer.spanBuilder("account.get")
                .setAttribute(AttributeKey.stringKey("account.id"), id)
                .startSpan();
        try (var scope = span.makeCurrent()) {
            log.info("Fetching account id={}", id);
            Account account = store.get(id);
            if (account == null) {
                log.warn("Account not found id={}", id);
                span.setAttribute(AttributeKey.stringKey("account.not_found"), "true");
                return ResponseEntity.notFound().build();
            }
            span.setAttribute(AttributeKey.stringKey("account.owner"), account.getOwnerName());
            span.setAttribute(AttributeKey.stringKey("account.status"), account.getStatus());
            return ResponseEntity.ok(account);
        } finally {
            span.end();
        }
    }

    @PostMapping("/accounts")
    public ResponseEntity<Account> createAccount(@RequestBody CreateAccountRequest req) {
        Span span = tracer.spanBuilder("account.create")
                .setAttribute(AttributeKey.stringKey("account.owner_name"), req.ownerName())
                .startSpan();
        try (var scope = span.makeCurrent()) {
            String id = "ACC" + UUID.randomUUID().toString().replace("-", "").substring(0, 6).toUpperCase();
            String ownerId = "OWN" + UUID.randomUUID().toString().replace("-", "").substring(0, 6).toUpperCase();

            Account account = new Account(
                    id, ownerId, req.ownerName(),
                    req.initialBalance() != null ? req.initialBalance() : BigDecimal.ZERO,
                    "THB", "ACTIVE", LocalDateTime.now());

            store.put(id, account);
            span.setAttribute(AttributeKey.stringKey("account.id"), id);
            log.info("Created account id={} owner={} balance={}", id, req.ownerName(), account.getBalance());
            return ResponseEntity.status(HttpStatus.CREATED).body(account);
        } finally {
            span.end();
        }
    }

    @GetMapping("/accounts/{id}/balance")
    public ResponseEntity<Map<String, Object>> getBalance(@PathVariable String id) {
        Span span = tracer.spanBuilder("account.get_balance")
                .setAttribute(AttributeKey.stringKey("account.id"), id)
                .startSpan();
        try (var scope = span.makeCurrent()) {
            log.info("Fetching balance for account id={}", id);
            Account account = store.get(id);
            if (account == null) {
                log.warn("Account not found for balance query id={}", id);
                return ResponseEntity.notFound().build();
            }
            Map<String, Object> response = Map.of(
                    "accountId", account.getId(),
                    "balance", account.getBalance(),
                    "currency", account.getCurrency());
            span.setAttribute(AttributeKey.stringKey("account.currency"), account.getCurrency());
            return ResponseEntity.ok(response);
        } finally {
            span.end();
        }
    }

    @GetMapping("/health")
    public ResponseEntity<Map<String, String>> health() {
        return ResponseEntity.ok(Map.of("status", "UP"));
    }

    // ---------------------------------------------------------------------------
    // Request DTO
    // ---------------------------------------------------------------------------

    public record CreateAccountRequest(String ownerName, BigDecimal initialBalance) {}
}
