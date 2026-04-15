package com.bankflow.account.model;

import java.math.BigDecimal;
import java.time.LocalDateTime;

public class Account {

    private String id;
    private String ownerId;
    private String ownerName;
    private BigDecimal balance;
    private String currency;
    private String status;
    private LocalDateTime createdAt;

    public Account() {
        this.currency = "THB";
        this.status = "ACTIVE";
        this.createdAt = LocalDateTime.now();
    }

    public Account(String id, String ownerId, String ownerName, BigDecimal balance,
                   String currency, String status, LocalDateTime createdAt) {
        this.id = id;
        this.ownerId = ownerId;
        this.ownerName = ownerName;
        this.balance = balance;
        this.currency = currency;
        this.status = status;
        this.createdAt = createdAt;
    }

    // --- Getters ---

    public String getId() {
        return id;
    }

    public String getOwnerId() {
        return ownerId;
    }

    public String getOwnerName() {
        return ownerName;
    }

    public BigDecimal getBalance() {
        return balance;
    }

    public String getCurrency() {
        return currency;
    }

    public String getStatus() {
        return status;
    }

    public LocalDateTime getCreatedAt() {
        return createdAt;
    }

    // --- Setters ---

    public void setId(String id) {
        this.id = id;
    }

    public void setOwnerId(String ownerId) {
        this.ownerId = ownerId;
    }

    public void setOwnerName(String ownerName) {
        this.ownerName = ownerName;
    }

    public void setBalance(BigDecimal balance) {
        this.balance = balance;
    }

    public void setCurrency(String currency) {
        this.currency = currency;
    }

    public void setStatus(String status) {
        this.status = status;
    }

    public void setCreatedAt(LocalDateTime createdAt) {
        this.createdAt = createdAt;
    }
}
