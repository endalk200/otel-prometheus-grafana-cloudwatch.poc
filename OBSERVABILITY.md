# Observability Stack Documentation

This document explains the complete observability setup for the User API application, including how telemetry data flows from the application to visualization in Grafana.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Components](#components)
3. [Data Flow](#data-flow)
4. [Application Instrumentation](#application-instrumentation)
5. [Metrics](#metrics)
6. [Traces](#traces)
7. [Logs](#logs)
8. [Configuration Files](#configuration-files)
9. [Running the Stack](#running-the-stack)
10. [Accessing the Tools](#accessing-the-tools)
11. [Troubleshooting](#troubleshooting)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Application Layer                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         User API (Go/Gin)                            │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │    │
│  │  │   Metrics    │  │    Traces    │  │     Logs     │               │    │
│  │  │  (counters,  │  │  (HTTP spans │  │   (slog +    │               │    │
│  │  │   gauges)    │  │   + custom)  │  │   otelslog)  │               │    │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘               │    │
│  │         │                 │                 │                        │    │
│  │         └─────────────────┼─────────────────┘                        │    │
│  │                           │                                          │    │
│  │              OpenTelemetry SDK (OTLP Exporters)                      │    │
│  └───────────────────────────┼──────────────────────────────────────────┘    │
│                              │ OTLP (gRPC :4317)                             │
└──────────────────────────────┼──────────────────────────────────────────────┘
                               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Collection Layer                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        Grafana Alloy                                 │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │    │
│  │  │ OTLP Receiver│  │Batch Process │  │ Node Exporter│               │    │
│  │  │  (metrics,   │  │              │  │ (CPU, RAM,   │               │    │
│  │  │traces, logs) │  │              │  │   disk)      │               │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘               │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                    │                │                │                       │
└────────────────────┼────────────────┼────────────────┼───────────────────────┘
                     │                │                │
                     ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                             Storage Layer                                    │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐              │
│  │  Prometheus  │      │     Loki     │      │    Tempo     │              │
│  │  (metrics)   │      │   (logs)     │      │  (traces)    │              │
│  │   :9090      │      │   :3100      │      │   :3200      │              │
│  └──────────────┘      └──────────────┘      └──────────────┘              │
└─────────────────────────────────────────────────────────────────────────────┘
                     │                │                │
                     └────────────────┼────────────────┘
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Visualization Layer                                 │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                           Grafana                                    │    │
│  │                           :3000                                      │    │
│  │  ┌──────────────────┐  ┌──────────────────┐                         │    │
│  │  │  System Metrics  │  │  User API        │                         │    │
│  │  │    Dashboard     │  │   Dashboard      │                         │    │
│  │  └──────────────────┘  └──────────────────┘                         │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Components

### 1. User API (Application)

The Go application instrumented with OpenTelemetry to emit:
- **Metrics**: Custom business metrics and HTTP metrics
- **Traces**: Distributed traces for all HTTP requests
- **Logs**: Structured logs via OpenTelemetry bridge

### 2. Grafana Alloy

A telemetry collector that:
- Receives OTLP data from the application
- Collects system metrics (CPU, memory, disk) via node exporter
- Routes telemetry to appropriate backends

### 3. Prometheus

Time-series database for metrics:
- Stores application metrics
- Stores system metrics
- Supports PromQL queries

### 4. Loki

Log aggregation system:
- Stores application logs
- Supports LogQL queries
- Links logs to traces

### 5. Tempo

Distributed tracing backend:
- Stores trace data
- Supports TraceQL queries
- Links traces to logs and metrics

### 6. Grafana

Visualization platform:
- Pre-configured dashboards
- Unified view of metrics, logs, and traces
- Cross-linking between signals

---

## Data Flow

### Metrics Flow

```
Application                 Alloy                    Prometheus
    │                         │                          │
    │  Custom Metrics         │                          │
    │  (user_api_*)           │                          │
    ├────────OTLP────────────►│                          │
    │                         │                          │
    │  HTTP Metrics           │                          │
    │  (otelgin auto)         │    Remote Write          │
    ├────────OTLP────────────►├─────────────────────────►│
    │                         │                          │
    │                         │  Node Exporter           │
    │                         │  (CPU, RAM, disk)        │
    │                         ├─────────────────────────►│
                              │                          │
```

1. The application creates metrics using OpenTelemetry Meter
2. Metrics are exported via OTLP gRPC to Alloy (port 4317)
3. Alloy converts OTLP metrics to Prometheus format
4. Alloy writes metrics to Prometheus via remote write API
5. Alloy's node exporter also sends system metrics to Prometheus

### Traces Flow

```
Application                 Alloy                    Tempo
    │                         │                        │
    │  HTTP Traces            │                        │
    │  (otelgin auto)         │                        │
    ├────────OTLP────────────►│                        │
    │                         │                        │
    │  Custom Spans           │     OTLP gRPC          │
    │  (business logic)       ├───────────────────────►│
    ├────────OTLP────────────►│                        │
    │                         │                        │
```

1. otelgin middleware automatically creates spans for HTTP requests
2. Application can add custom spans for business logic
3. Traces are exported via OTLP gRPC to Alloy
4. Alloy forwards traces to Tempo via OTLP

### Logs Flow

```
Application                 Alloy                    Loki
    │                         │                        │
    │  Structured Logs        │                        │
    │  (slog + otelslog)      │     Loki Push API      │
    ├────────OTLP────────────►├───────────────────────►│
    │                         │                        │
```

1. Application uses `slog` logger with OpenTelemetry bridge
2. Logs are exported via OTLP gRPC to Alloy
3. Alloy converts and pushes logs to Loki

---

## Application Instrumentation

### OpenTelemetry Initialization (main.go)

The application initializes three providers:

```go
// Tracer Provider - for distributed tracing
func initTracerProvider(ctx context.Context, res *resource.Resource) {
    exporter, _ := otlptracegrpc.New(ctx,
        otlptracegrpc.WithInsecure(),
        otlptracegrpc.WithEndpoint("alloy:4317"),
    )
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(res),
    )
    otel.SetTracerProvider(tp)
}

// Meter Provider - for metrics
func initMeterProvider(ctx context.Context, res *resource.Resource) {
    exporter, _ := otlpmetricgrpc.New(ctx,
        otlpmetricgrpc.WithInsecure(),
        otlpmetricgrpc.WithEndpoint("alloy:4317"),
    )
    mp := sdkmetric.NewMeterProvider(
        sdkmetric.WithResource(res),
        sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
            sdkmetric.WithInterval(15*time.Second),
        )),
    )
    otel.SetMeterProvider(mp)
}

// Logger Provider - for logs
func initLoggerProvider(ctx context.Context, res *resource.Resource) {
    exporter, _ := otlploggrpc.New(ctx,
        otlploggrpc.WithInsecure(),
        otlploggrpc.WithEndpoint("alloy:4317"),
    )
    lp := sdklog.NewLoggerProvider(
        sdklog.WithResource(res),
        sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
    )
    global.SetLoggerProvider(lp)
}
```

### HTTP Middleware (otelgin)

Automatic tracing is added via middleware:

```go
router.Use(otelgin.Middleware(serviceName))
```

This automatically creates spans for every HTTP request with:
- HTTP method, route, status code
- Request/response timing
- Error recording

### Custom Metrics (handlers/user.go)

Custom business metrics are defined:

```go
// Current number of users (can go up and down)
userCounter, _ := meter.Int64UpDownCounter(
    "user_api_users_total",
    metric.WithDescription("Current number of users in the system"),
)

// Total users ever created (only increments)
usersCreated, _ := meter.Int64Counter(
    "user_api_users_created_total",
    metric.WithDescription("Total number of users created"),
)

// Total users ever deleted (only increments)
usersDeleted, _ := meter.Int64Counter(
    "user_api_users_deleted_total",
    metric.WithDescription("Total number of users deleted"),
)

// Operations counter with operation type label
userOperations, _ := meter.Int64Counter(
    "user_api_operations_total",
    metric.WithDescription("Total number of user operations"),
)
```

### Recording Metrics

Metrics are recorded in handler methods:

```go
// In Create handler
h.usersCreated.Add(ctx, 1)
h.userCounter.Add(ctx, 1)

// In Delete handler
h.usersDeleted.Add(ctx, 1)
h.userCounter.Add(ctx, -1)

// For all operations
h.userOperations.Add(ctx, 1, metric.WithAttributes(
    attribute.String("operation", "create"),
))
```

### Enriching Traces

Spans are enriched with attributes:

```go
span := trace.SpanFromContext(ctx)
span.SetAttributes(
    attribute.String("user.id", user.ID),
    attribute.String("user.email", user.Email),
)

// Record errors
if err != nil {
    span.RecordError(err)
}
```

---

## Metrics

### Application Metrics

| Metric Name | Type | Description |
|-------------|------|-------------|
| `user_api_users_total` | UpDownCounter | Current number of users |
| `user_api_users_created_total` | Counter | Total users created |
| `user_api_users_deleted_total` | Counter | Total users deleted |
| `user_api_operations_total` | Counter | Operations by type |

### HTTP Metrics (auto-generated by otelgin)

| Metric Name | Type | Description |
|-------------|------|-------------|
| `http_server_request_duration_seconds` | Histogram | Request latency |
| `http_server_request_duration_seconds_count` | Counter | Request count |

### System Metrics (from node exporter)

| Metric Name | Description |
|-------------|-------------|
| `node_cpu_seconds_total` | CPU time by mode |
| `node_memory_MemTotal_bytes` | Total memory |
| `node_memory_MemAvailable_bytes` | Available memory |
| `node_filesystem_size_bytes` | Filesystem size |
| `node_filesystem_avail_bytes` | Available disk space |

---

## Traces

### Automatic Spans

The `otelgin` middleware creates spans for every HTTP request:

```
GET /users
├── Attributes:
│   ├── http.method: GET
│   ├── http.route: /users
│   ├── http.status_code: 200
│   ├── http.target: /users
│   └── net.host.name: localhost
└── Duration: 2.5ms
```

### Custom Span Attributes

Business context is added to spans:

```
POST /users
├── Attributes:
│   ├── http.method: POST
│   ├── http.route: /users
│   ├── user.id: abc-123
│   ├── user.email: john@example.com
│   └── http.status_code: 201
└── Duration: 5.2ms
```

### Trace Context Propagation

Trace context is propagated using W3C Trace Context headers:
- `traceparent`: Contains trace ID and span ID
- `tracestate`: Vendor-specific data

---

## Logs

### Log Format

Logs are emitted as structured JSON via OpenTelemetry:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "severity": "INFO",
  "body": "Creating new user",
  "attributes": {
    "name": "John Doe",
    "email": "john@example.com"
  },
  "resource": {
    "service.name": "user-api",
    "service.version": "1.0.0"
  },
  "trace_id": "abc123...",
  "span_id": "def456..."
}
```

### Log Levels

| Level | Usage |
|-------|-------|
| INFO | Normal operations |
| WARN | Recoverable issues (e.g., user not found) |
| ERROR | Failures that need attention |

### Trace Correlation

Logs automatically include trace context, enabling:
- Click from log to related trace in Grafana
- Filter logs by trace ID
- See all logs for a specific request

---

## Configuration Files

### Alloy Configuration (observability/alloy/config.alloy)

```river
// Receive OTLP telemetry from application
otelcol.receiver.otlp "default" {
  grpc { endpoint = "0.0.0.0:4317" }
  http { endpoint = "0.0.0.0:4318" }
  output {
    metrics = [otelcol.processor.batch.default.input]
    traces  = [otelcol.processor.batch.default.input]
    logs    = [otelcol.processor.batch.default.input]
  }
}

// Batch for efficiency
otelcol.processor.batch "default" {
  output {
    metrics = [otelcol.exporter.prometheus.default.input]
    traces  = [otelcol.exporter.otlp.tempo.input]
    logs    = [otelcol.exporter.loki.default.input]
  }
}

// Export metrics to Prometheus
prometheus.remote_write "default" {
  endpoint { url = "http://prometheus:9090/api/v1/write" }
}

// Export traces to Tempo
otelcol.exporter.otlp "tempo" {
  client {
    endpoint = "tempo:4317"
    tls { insecure = true }
  }
}

// Export logs to Loki
loki.write "default" {
  endpoint { url = "http://loki:3100/loki/api/v1/push" }
}

// Collect system metrics
prometheus.exporter.unix "node" {}
prometheus.scrape "node" {
  targets = prometheus.exporter.unix.node.targets
  forward_to = [prometheus.remote_write.default.receiver]
}
```

### Grafana Datasources (observability/grafana/provisioning/datasources/datasources.yml)

```yaml
datasources:
  - name: Prometheus
    type: prometheus
    url: http://prometheus:9090
    isDefault: true
    
  - name: Loki
    type: loki
    url: http://loki:3100
    jsonData:
      derivedFields:
        - name: TraceID
          datasourceUid: tempo
          
  - name: Tempo
    type: tempo
    url: http://tempo:3200
    jsonData:
      tracesToLogs:
        datasourceUid: loki
      tracesToMetrics:
        datasourceUid: prometheus
```

---

## Running the Stack

### Start Everything

```bash
docker-compose up -d
```

### View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f user-api
docker-compose logs -f alloy
```

### Rebuild After Code Changes

```bash
docker-compose build user-api
docker-compose up -d user-api
```

### Stop Everything

```bash
docker-compose down
```

### Clean Up (including volumes)

```bash
docker-compose down -v
```

---

## Accessing the Tools

| Service | URL | Credentials |
|---------|-----|-------------|
| **Grafana** | http://localhost:3000 | admin / admin |
| **Prometheus** | http://localhost:9090 | - |
| **Alloy UI** | http://localhost:12345 | - |
| **User API** | http://localhost:8080 | - |

### Testing the API

```bash
# Create a user
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"name": "John Doe", "email": "john@example.com"}'

# Get all users
curl http://localhost:8080/users

# Delete a user
curl -X DELETE http://localhost:8080/users/{id}
```

### Viewing in Grafana

1. Open http://localhost:3000
2. Login with admin/admin
3. Navigate to Dashboards > User API
4. Select the dashboard:
   - **System Metrics**: CPU, memory, disk usage
   - **User API Dashboard**: User metrics, HTTP metrics, logs, traces

---

## Troubleshooting

### No Metrics in Prometheus

1. Check Alloy is receiving data:
   ```bash
   curl http://localhost:12345/metrics
   ```

2. Check Prometheus targets:
   - Go to http://localhost:9090/targets
   - Verify Alloy target is UP

3. Check application logs:
   ```bash
   docker-compose logs user-api | grep -i otel
   ```

### No Traces in Tempo

1. Verify Tempo is running:
   ```bash
   curl http://localhost:3200/ready
   ```

2. Check Alloy pipeline:
   - Go to http://localhost:12345
   - Look for the traces pipeline

### No Logs in Loki

1. Check Loki is ready:
   ```bash
   curl http://localhost:3100/ready
   ```

2. Query logs directly:
   ```bash
   curl -G "http://localhost:3100/loki/api/v1/query_range" \
     --data-urlencode 'query={service_name="user-api"}'
   ```

### Connection Refused Errors

Ensure services are on the same network:
```bash
docker network inspect otel-prometheus-grafana-cloudwatchpoc_observability
```

### High Memory Usage

For development, you can reduce retention:
- Prometheus: Add `--storage.tsdb.retention.time=1h`
- Loki: Reduce `limits_config.retention_period`
- Tempo: Reduce `compactor.compaction.block_retention`

---

## Summary

This observability stack provides:

1. **Metrics**: Business metrics (user counts) and HTTP metrics stored in Prometheus
2. **Traces**: Distributed tracing for all requests stored in Tempo
3. **Logs**: Structured logs with trace correlation stored in Loki
4. **System Metrics**: CPU, memory, disk via Alloy's node exporter
5. **Visualization**: Pre-built Grafana dashboards with cross-linking

The key insight is that all telemetry flows through a single collector (Alloy), which normalizes and routes data to specialized backends. Grafana then provides a unified view with the ability to jump between metrics, logs, and traces for efficient debugging and monitoring.
