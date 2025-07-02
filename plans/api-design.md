# API Extensions and Metrics Design

## Overview

This document outlines the design for API extensions to the NTP Pool monitoring system, focusing on programmatic access to monitor metrics and data through authenticated endpoints.

## Monitor Metrics API

### API Architecture

#### Endpoint Design
```
GET /api/v1/monitors/{monitor_id}/metrics/{metric_name}
```

**Design Principles**:
- RESTful resource-oriented URLs
- Consistent parameter naming conventions
- Extensible for future metric types
- Secure by default with authentication required

#### URL Parameters
- `monitor_id`: Monitor identifier (e.g., "defra3-1gw2pjj")
- `metric_name`: Prometheus metric name (validated against allowlist)

#### Query Parameters
- `start`: Start time (RFC3339 format or Unix timestamp)
- `end`: End time (RFC3339 format or Unix timestamp)
- `step`: Query resolution step (e.g., "5m", "1h")
- `aggregation`: Aggregation function ("rate", "sum", "avg", "max", "min")
- `window`: Time window for rate calculations (e.g., "5m", "10m")
- `group_by`: Comma-separated labels for grouping results
- `filter`: Additional PromQL-style label filters

### Authentication and Security

#### Authentication Mechanisms
- **JWT Authentication**: Reuse existing monitor-api JWT tokens
- **Certificate-Based**: Client certificate validation for automated systems
- **API Tokens**: Account-level API tokens for programmatic access

#### Authorization Strategy
```go
func authorizeMetricsAccess(userAccount uint32, monitorID string) error {
    // Verify account owns the specified monitor
    monitor, err := db.GetMonitorByID(monitorID)
    if err != nil {
        return err
    }

    if monitor.AccountID != userAccount {
        return ErrUnauthorized
    }

    return nil
}
```

#### Security Controls
- **Input Validation**: Sanitize all parameters to prevent injection attacks
- **Rate Limiting**: Per-account request limits to prevent abuse
- **Query Complexity**: Limits on time ranges and result sizes
- **PromQL Injection Prevention**: Strict parameter validation and escaping

### Prometheus Integration

#### Query Construction
The system constructs secure Prometheus queries with mandatory monitor filtering:

**Base Query Pattern**:
```promql
{metric_name}{monitor="{monitor_id}"}[{time_range}]
```

**Rate Aggregation Pattern**:
```promql
sum by ({group_by_labels}) (
  rate({metric_name}{monitor="{monitor_id}"}[{window}])
)
```

**Security Constraints**:
- Always include `monitor="{monitor_id}"` filter
- Validate monitor ownership before query execution
- Sanitize all user inputs to prevent PromQL injection
- Enforce maximum time range limits

#### Configuration
- `PROMETHEUS_URL`: Prometheus endpoint URL
- `PROMETHEUS_TIMEOUT`: Query timeout (default: 30s)
- `MAX_TIME_RANGE`: Maximum query time range (default: 30 days)
- `MAX_RESULT_SIZE`: Maximum result set size

### Response Format

#### Success Response Structure
```json
{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": {
          "account": "1gw2pjj",
          "account_id": "593",
          "monitor": "defra3-1gw2pjj.test.mon.ntppool.dev",
          "ip_version": "v4",
          "result": "ok"
        },
        "values": [
          [1704067200, "255"],
          [1704067260, "258"],
          [1704067320, "261"]
        ]
      }
    ]
  },
  "meta": {
    "query": "tests_completed_total{monitor=\"defra3-1gw2pjj\"}",
    "execution_time": "0.123s",
    "result_count": 1
  }
}
```

#### Error Response Structure
```json
{
  "status": "error",
  "error": "validation_error",
  "message": "Invalid time range: start time cannot be after end time",
  "details": {
    "field": "start",
    "value": "2024-01-02T00:00:00Z"
  }
}
```

### Available Metrics

#### Core Monitoring Metrics
1. **tests_completed_total**
   - **Purpose**: Track completed NTP tests by result type
   - **Labels**: account, account_id, monitor, ip_version, result
   - **Result Values**: ok, timeout, offset, signature_validation, batch_out_of_order

2. **tests_requested_total**
   - **Purpose**: Track total test requests initiated
   - **Labels**: account, account_id, monitor, ip_version

3. **monitors_connected**
   - **Purpose**: Track monitor connection status
   - **Labels**: account, account_id, monitor

#### Performance Metrics
1. **rpc_request_duration_seconds**
   - **Purpose**: RPC call performance tracking
   - **Labels**: account, account_id, method

2. **tests_response_time_seconds**
   - **Purpose**: NTP test response time measurements
   - **Labels**: account, account_id, monitor, server_id

#### System Health Metrics
1. **monitor_health_score**
   - **Purpose**: Composite health scoring
   - **Labels**: account, account_id, monitor

2. **constraint_violations_total**
   - **Purpose**: Track constraint violations in selector
   - **Labels**: account, account_id, monitor, constraint_type

### API Usage Examples

#### Basic Metric Query
```bash
curl -H "Authorization: Bearer $JWT_TOKEN" \
  "https://api.mon.ntppool.dev/api/v1/monitors/defra3-1gw2pjj/metrics/tests_completed_total?start=2024-01-01T00:00:00Z&end=2024-01-02T00:00:00Z"
```

#### Rate Calculation with Grouping
```bash
curl -H "Authorization: Bearer $JWT_TOKEN" \
  "https://api.mon.ntppool.dev/api/v1/monitors/defra3-1gw2pjj/metrics/tests_completed_total?aggregation=rate&window=10m&group_by=ip_version,result"
```

#### Filtered Query
```bash
curl -H "Authorization: Bearer $JWT_TOKEN" \
  "https://api.mon.ntppool.dev/api/v1/monitors/defra3-1gw2pjj/metrics/tests_completed_total?filter=result=\"ok\"&start=-1h"
```

## Alternative API Designs

### GraphQL Interface
More flexible querying capability for complex data needs:

```graphql
query MonitorMetrics($monitorId: String!, $start: Time!) {
  monitor(id: $monitorId) {
    id
    name
    status
    metrics(timeRange: { start: $start, end: $end }) {
      testsCompleted(groupBy: [IP_VERSION, RESULT]) {
        series {
          labels
          values
        }
      }
      healthScore {
        current
        trend
      }
    }
  }
}
```

**Benefits**:
- Single request for multiple metrics
- Flexible field selection
- Strong typing and validation
- Better caching opportunities

### Webhook Integration
Real-time metric streaming for monitoring applications:

```go
type WebhookConfig struct {
    URL        string   `json:"url"`
    Secret     string   `json:"secret"`
    Metrics    []string `json:"metrics"`
    Conditions []string `json:"conditions"`
}

// Example webhook payload
type MetricEvent struct {
    MonitorID string                 `json:"monitor_id"`
    Metric    string                 `json:"metric"`
    Value     float64                `json:"value"`
    Labels    map[string]string      `json:"labels"`
    Timestamp time.Time              `json:"timestamp"`
}
```

### Predefined Metric Bundles
Common metric combinations for specific use cases:

```
GET /api/v1/monitors/{monitor_id}/metrics/health
GET /api/v1/monitors/{monitor_id}/metrics/performance
GET /api/v1/monitors/{monitor_id}/metrics/availability
```

## Implementation Strategy

### Phase 1: Core API Implementation
**Scope**: Basic metrics endpoint with authentication
- Single metric queries with time range support
- JWT authentication validation
- Basic Prometheus integration
- Error handling and validation

**Deliverables**:
- REST endpoint implementation
- Authentication middleware
- Prometheus query builder
- Basic response formatting

### Phase 2: Advanced Features
**Scope**: Enhanced querying capabilities
- Aggregation functions (rate, sum, avg)
- Label-based filtering and grouping
- Response caching for performance
- Query complexity limits

**Deliverables**:
- Advanced query parameter support
- Caching layer implementation
- Performance optimization
- Extended metric support

### Phase 3: Integration and Scaling
**Scope**: Production-ready features
- Webhook integration for real-time data
- GraphQL endpoint option
- Result pagination for large datasets
- Comprehensive monitoring and alerting

## Performance Considerations

### Caching Strategy
- **Query Result Caching**: Cache Prometheus responses for frequently accessed data
- **Authentication Caching**: Cache JWT validation results with appropriate TTL
- **Metric Metadata Caching**: Cache available metrics and their schemas

### Query Optimization
- **Time Range Limits**: Enforce maximum query time ranges to prevent expensive queries
- **Result Size Limits**: Limit response size to prevent memory issues
- **Query Complexity Scoring**: Score and limit complex aggregations

### Rate Limiting
```go
type RateLimiter struct {
    RequestsPerMinute int
    BurstSize        int
    WindowSize       time.Duration
}

// Per-account rate limiting
func (rl *RateLimiter) Allow(accountID uint32) bool {
    // Implementation using token bucket or sliding window
}
```

## Testing Strategy

### Unit Testing
- **Query Construction**: Validate PromQL query generation
- **Input Validation**: Test parameter sanitization and validation
- **Authentication**: Verify authorization logic
- **Error Handling**: Test error response formatting

### Integration Testing
- **Prometheus Integration**: Mock Prometheus responses
- **End-to-End Flow**: Complete API request/response cycles
- **Performance Testing**: Load testing with concurrent requests
- **Security Testing**: Injection attack prevention

### Documentation Testing
- **API Documentation**: Validate example requests work correctly
- **Response Schema**: Ensure response format matches documentation
- **Error Scenarios**: Document and test all error conditions

## Security Best Practices

### Input Validation
- **Parameter Sanitization**: Escape all user inputs
- **Allowlist Validation**: Validate metric names against known list
- **Time Range Validation**: Enforce reasonable time range limits
- **Label Filter Validation**: Prevent PromQL injection in filters

### Access Control
- **Monitor Ownership**: Always verify account owns requested monitor
- **Scope Limitation**: Limit API access to owned resources only
- **Audit Logging**: Log all API access for security monitoring

### Data Protection
- **Response Filtering**: Never include sensitive labels in responses
- **Query Isolation**: Ensure queries cannot access other accounts' data
- **Error Information**: Avoid exposing system internals in error messages

## Monitor Configuration Management API

### CLI Configuration Editor

The monitor-api includes a kubectl-style configuration management system for editing monitor configurations:

#### Command Structure
```bash
monitor-api config get <monitor-name> [v4|v6]     # Show current config
monitor-api config edit <monitor-name> [v4|v6]    # Edit config in $EDITOR
monitor-api config set <monitor-name> [v4|v6] <key> <value>  # Set specific field

# Special case for system defaults:
monitor-api config get defaults [v4|v6]          # Show system defaults
monitor-api config edit defaults [v4|v6]         # Edit system defaults
monitor-api config set defaults [v4|v6] <key> <value>  # Set default field
```

#### Key Features

**Interactive Editing**:
- Uses `$EDITOR` environment variable (fallback to `vi`) for JSON editing
- Multi-version editing: Shows both v4/v6 configs when IP version omitted
- JSON validation with confirmation prompt for invalid JSON
- Temporary file handling for secure editor sessions

**Configuration Management**:
- **Defaults handling**: "defaults" maps to `settings-v4.system` and `settings-v6.system`
- **Config merging**: `get` operations show merged config (defaults + specific)
- **Dot notation**: `set` command supports nested JSON key paths
- **IP version logic**: Auto-detects existing versions, edits both if unspecified

#### Implementation Status

**Completed Features ✅**:
- kubectl-style command interface with embedded `configCmd` in `ApiCmd`
- Complete functionality in `server/cmd/config.go`
- JSON validation and pretty-printing
- Multi-version editing support
- Dot notation for granular updates
- Error handling for missing monitors/versions
- `UpdateMonitorConfig` SQL query integration

**Usage Examples**:
```bash
# View system defaults for IPv4
monitor-api config get defaults v4

# Edit all configurations for a monitor
monitor-api config edit my-monitor

# Set specific nested value
monitor-api config set defaults v4 samples 10

# Edit only IPv6 configuration
monitor-api config edit my-monitor v6
```

#### Database Integration

The system uses the `UpdateMonitorConfig` SQL query for persistence:
```sql
-- Updates monitor configuration with JSON validation
UPDATE monitors
SET config = ?
WHERE monitor_name = ? AND ip_version = ?
```

**Behavior Logic**:
- Regular monitors: Query database for existing IP versions
- "defaults" monitor: Maps to system monitor configuration entries
- Multi-version operations: Present structured JSON with `v4` and `v6` keys
- Configuration inheritance: Shows merged view combining defaults with monitor-specific settings

This configuration management system provides operators with powerful, safe tools for managing monitor behavior through familiar command-line interfaces.

## Future API Enhancements

### Real-time Configuration Updates
- WebSocket-based configuration change notifications
- Hot-reload capabilities for monitor configurations
- Configuration validation API for testing changes before applying

### Configuration History and Rollback
- Track configuration change history with timestamps and authors
- API endpoints for rolling back to previous configurations
- Diff APIs for comparing configuration versions

This API design provides a secure, scalable foundation for programmatic access to monitoring data while maintaining the system's security and performance requirements.
