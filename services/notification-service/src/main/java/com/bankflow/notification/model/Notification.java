package com.bankflow.notification.model;

import java.time.LocalDateTime;

public class Notification {

    private String id;
    private String accountId;
    private String type;       // "TRANSFER" | "ALERT" | "FRAUD_ALERT"
    private String message;
    private LocalDateTime timestamp;
    private boolean sent;

    public Notification() {
        this.timestamp = LocalDateTime.now();
        this.sent = false;
    }

    public Notification(String id, String accountId, String type,
                        String message, LocalDateTime timestamp, boolean sent) {
        this.id = id;
        this.accountId = accountId;
        this.type = type;
        this.message = message;
        this.timestamp = timestamp;
        this.sent = sent;
    }

    // --- Getters ---

    public String getId() {
        return id;
    }

    public String getAccountId() {
        return accountId;
    }

    public String getType() {
        return type;
    }

    public String getMessage() {
        return message;
    }

    public LocalDateTime getTimestamp() {
        return timestamp;
    }

    public boolean isSent() {
        return sent;
    }

    // --- Setters ---

    public void setId(String id) {
        this.id = id;
    }

    public void setAccountId(String accountId) {
        this.accountId = accountId;
    }

    public void setType(String type) {
        this.type = type;
    }

    public void setMessage(String message) {
        this.message = message;
    }

    public void setTimestamp(LocalDateTime timestamp) {
        this.timestamp = timestamp;
    }

    public void setSent(boolean sent) {
        this.sent = sent;
    }
}
