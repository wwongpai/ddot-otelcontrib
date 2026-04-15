package com.bankflow.account.client;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.http.ResponseEntity;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestTemplate;

import java.math.BigDecimal;
import java.util.Map;

/**
 * HTTP client for the transaction-service.
 *
 * The RestTemplate bean is defined here so that the OTel Java agent can
 * automatically instrument outgoing HTTP calls (via its RestTemplate
 * instrumentation) without any additional configuration.
 */
@Component
public class TransactionClient {

    private static final Logger log = LoggerFactory.getLogger(TransactionClient.class);

    private final RestTemplate restTemplate;
    private final String transactionServiceUrl;

    public TransactionClient(
            RestTemplate restTemplate,
            @Value("${transaction-service.url:http://transaction-service:8080}") String transactionServiceUrl) {
        this.restTemplate = restTemplate;
        this.transactionServiceUrl = transactionServiceUrl;
    }

    /**
     * Initiates a transfer via the transaction-service.
     *
     * @param fromAccountId source account
     * @param toAccountId   destination account
     * @param amount        amount to transfer (THB)
     * @return response body from transaction-service, or null on failure
     */
    public Map<?, ?> transfer(String fromAccountId, String toAccountId, BigDecimal amount) {
        String url = transactionServiceUrl + "/transfer";
        Map<String, Object> body = Map.of(
                "fromAccountId", fromAccountId,
                "toAccountId", toAccountId,
                "amount", amount,
                "currency", "THB");

        log.info("Calling transaction-service transfer url={} from={} to={} amount={}",
                url, fromAccountId, toAccountId, amount);

        try {
            ResponseEntity<Map> response = restTemplate.postForEntity(url, body, Map.class);
            log.info("Transfer response status={}", response.getStatusCode());
            return response.getBody();
        } catch (Exception ex) {
            log.error("Transfer call failed from={} to={} amount={}: {}",
                    fromAccountId, toAccountId, amount, ex.getMessage(), ex);
            throw ex;
        }
    }

    // ---------------------------------------------------------------------------
    // Bean configuration
    // ---------------------------------------------------------------------------

    @Configuration
    static class RestTemplateConfig {

        /**
         * Exposes a plain RestTemplate. The OTel Java agent instruments
         * RestTemplate at the class level via byte-buddy, so no manual
         * interceptor wiring is required.
         */
        @Bean
        public RestTemplate restTemplate() {
            return new RestTemplate();
        }
    }
}
