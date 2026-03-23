# Timeout Infrastructure

## Overview

Standalone NATS JetStream-based timeout monitoring infrastructure for proactive timeout detection. This system is **not yet integrated** into the main application flow.

## Components

### 1. NATS JetStream Stream
- **Name**: `TRANSITION_TIMEOUTS`
- **Subjects**: `timeout.>`
- **Retention**: WorkQueuePolicy (7 days)
- **Storage**: FileStorage
- **Purpose**: Queue for scheduled timeout events

### 2. NATS KV Bucket
- **Name**: `timeout_cancellations`
- **TTL**: 7 days (auto-cleanup)
- **Purpose**: Store cancellation flags to prevent false positives

### 3. Timeout Monitor Service
- **File**: `timeout_monitor.go`
- **Consumer**: `timeout-monitor` (durable)
- **Functionality**:
  - Consumes timeout events from stream
  - Checks KV bucket for cancellation flags
  - Logs timeout fires (violation logic not implemented yet)

## Timeout Types

### Transition Timeout
Monitors time between step completion and next step start.

```go
TimeoutEvent{
    Type:       TimeoutTypeTransition,
    FromStep:   "order_placed",
    ToStep:     "payment_validation",
    DeadlineAt: step.startedAt + transition.timeout,
}
```

### Thread Max Duration
Monitors overall thread execution time.

```go
TimeoutEvent{
    Type:       TimeoutTypeMaxDuration,
    DeadlineAt: thread.startedAt + contract.validation.max_duration,
}
```

### Step Timeout
Monitors individual step execution duration (already handled reactively).

```go
TimeoutEvent{
    Type:       TimeoutTypeStep,
    StepName:   "payment_processing",
    DeadlineAt: step.startedAt + step.timeout,
}
```

## Usage (When Integrated)

### Schedule Timeout
```go
monitor.ScheduleTimeout(models.TimeoutEvent{
    ID:          uuid.New().String(),
    ThreadID:    threadID,
    Type:        models.TimeoutTypeTransition,
    FromStep:    "order_placed",
    ToStep:      "payment_validation",
    Timeout:     "2m",
    ScheduledAt: time.Now(),
    DeadlineAt:  startedAt.Add(2 * time.Minute),
})
```

### Cancel Timeout
```go
monitor.CancelTimeout(timeoutID, threadID, "next_step_started")
```

## Integration Points (TODO)

### 1. Schedule Timeouts
**Location**: `notification_service.go` → `PerformAsyncValidation()`

**When to schedule:**
- **Transition timeout**: When step completes successfully
- **Max duration**: When thread starts
- **Step timeout**: When step starts (optional, already handled reactively)

**Logic:**
```go
// After step completes successfully
if transition.Timeout != "" {
    deadline := step.StartedAt.Add(parseDuration(transition.Timeout))
    monitor.ScheduleTimeout(models.TimeoutEvent{
        ID:         generateTimeoutID(threadID, fromStep, toStep),
        ThreadID:   threadID,
        Type:       models.TimeoutTypeTransition,
        FromStep:   currentStep,
        ToStep:     nextStep,
        DeadlineAt: deadline,
    })
}
```

### 2. Cancel Timeouts
**Location**: `step_event.go` → `RecordStepEvent()`

**When to cancel:**
- When next step starts (cancel transition timeout)
- When thread reaches terminal state (cancel all timeouts)
- When step is retried (cancel old timeout, schedule new one)

**Logic:**
```go
// When next step starts
if previousStep != "" {
    timeoutID := generateTimeoutID(threadID, previousStep, currentStep)
    monitor.CancelTimeout(timeoutID, threadID, "next_step_started")
}
```

### 3. Fire Violations
**Location**: `timeout_monitor.go` → `handleTimeoutEvent()`

**Current state**: Logs timeout fires, no violation logic

**TODO**: Create violation notification and publish to NATS
```go
// In handleTimeoutEvent(), after checking cancellation
violation := models.ValidationNotification{
    ThreadID:   event.ThreadID,
    Type:       models.ViolationTransitionTimeoutExceeded,
    Severity:   "critical",
    Message:    fmt.Sprintf("Transition from %s to %s exceeded timeout", event.FromStep, event.ToStep),
    // ... other fields
}
notificationService.PublishNotification(violation)
```

## Key Design Decisions

### 1. Deadline Calculation
- **Transition timeout**: `step.startedAt + transition.timeout`
- **Max duration**: `thread.startedAt + max_duration`
- **Step timeout**: Handled reactively (not proactive)

### 2. Cancellation Strategy
- Write-ahead cancellation flags in KV store
- Consumer checks KV before firing violation
- TTL auto-cleanup prevents memory leaks

### 3. Deduplication
- Use `Nats-Msg-Id` header for message deduplication
- Timeout ID format: `{threadID}:{fromStep}:{toStep}:{type}`

### 4. Reliability
- Durable consumer with 3 max deliveries
- 30-second ack wait
- File storage for persistence

## Testing

See `timeout_monitor_test.go` for usage examples.

## Status

✅ **Implemented (Standalone)**:
- NATS stream and KV bucket initialization
- Timeout consumer with cancellation checking
- Schedule and cancel timeout methods
- Message structures and types

❌ **Not Yet Integrated**:
- Scheduling timeouts from async validator
- Cancelling timeouts on step completion
- Firing violation notifications
- Timeout calculation logic

## Next Steps (When Ready to Integrate)

1. Add timeout monitor to main server initialization
2. Schedule timeouts in `PerformAsyncValidation()`
3. Cancel timeouts in `RecordStepEvent()`
4. Implement violation notification in `handleTimeoutEvent()`
5. Add timeout ID generation utility
6. Add metrics and monitoring
