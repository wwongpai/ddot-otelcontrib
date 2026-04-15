package com.bankflow.account.config;

import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.trace.Tracer;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class OtelConfig {

    /**
     * Expose a Tracer bean sourced from GlobalOpenTelemetry.
     * The OTel Java agent initialises GlobalOpenTelemetry before the Spring
     * ApplicationContext starts, so this returns a fully configured Tracer.
     * When running without the agent (e.g., unit tests) it returns a no-op Tracer.
     */
    @Bean
    public Tracer tracer() {
        return GlobalOpenTelemetry.get().getTracer("com.bankflow.account-service", "1.0.0");
    }
}
