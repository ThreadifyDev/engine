package valkey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/internal/database"
)

const releaseOTelCreationLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`

const completeOTelSpanScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    redis.call("SET", KEYS[1], "done", "PX", ARGV[2])
    return 1
end
return 0
`

// OTelTraceRepository stores distributed OTLP trace correlation state.
type OTelTraceRepository struct {
	valkey  *database.ValkeyService
	mapTTL  time.Duration
	lockTTL time.Duration
}

func NewOTelTraceRepository(valkey *database.ValkeyService, mapTTL, lockTTL time.Duration) *OTelTraceRepository {
	return &OTelTraceRepository{valkey: valkey, mapTTL: mapTTL, lockTTL: lockTTL}
}

func (r *OTelTraceRepository) GetThreadID(ctx context.Context, companyID, traceID string) (string, error) {
	threadID, err := r.valkey.Get(ctx, otelTraceKey(companyID, traceID))
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get OTLP trace correlation: %w", err)
	}
	_ = r.valkey.Expire(ctx, otelTraceKey(companyID, traceID), r.mapTTL)
	return threadID, nil
}

func (r *OTelTraceRepository) SetThreadIDIfAbsent(ctx context.Context, companyID, traceID, threadID string) (bool, error) {
	set, err := r.valkey.SetNX(ctx, otelTraceKey(companyID, traceID), threadID, r.mapTTL)
	if err != nil {
		return false, fmt.Errorf("set OTLP trace correlation: %w", err)
	}
	return set, nil
}

func (r *OTelTraceRepository) IsTraceCompleted(ctx context.Context, companyID, traceID string) (bool, error) {
	state, err := r.valkey.Get(ctx, otelTraceCompletedKey(companyID, traceID))
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get OTLP trace completion: %w", err)
	}
	return state != "", nil
}

func (r *OTelTraceRepository) MarkTraceCompleted(ctx context.Context, companyID, traceID string) error {
	if err := r.valkey.Set(ctx, otelTraceCompletedKey(companyID, traceID), "done", r.mapTTL); err != nil {
		return fmt.Errorf("mark OTLP trace completed: %w", err)
	}
	return nil
}

func (r *OTelTraceRepository) AcquireCreationLock(ctx context.Context, companyID, traceID, token string) (bool, error) {
	acquired, err := r.valkey.SetNX(ctx, otelTraceLockKey(companyID, traceID), token, r.lockTTL)
	if err != nil {
		return false, fmt.Errorf("acquire OTLP trace creation lock: %w", err)
	}
	return acquired, nil
}

func (r *OTelTraceRepository) GetSpanState(ctx context.Context, companyID, traceID, spanID string) (string, error) {
	state, err := r.valkey.Get(ctx, otelSpanKey(companyID, traceID, spanID))
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get OTLP span state: %w", err)
	}
	return state, nil
}

func (r *OTelTraceRepository) ClaimSpan(ctx context.Context, companyID, traceID, spanID, token string) (bool, error) {
	claimed, err := r.valkey.SetNX(ctx, otelSpanKey(companyID, traceID, spanID), token, r.lockTTL)
	if err != nil {
		return false, fmt.Errorf("claim OTLP span: %w", err)
	}
	return claimed, nil
}

func (r *OTelTraceRepository) CompleteSpan(ctx context.Context, companyID, traceID, spanID, token string) error {
	ttlMillis := r.mapTTL.Milliseconds()
	if ttlMillis < 1 {
		ttlMillis = 1
	}
	result, err := r.valkey.Eval(
		ctx,
		completeOTelSpanScript,
		[]string{otelSpanKey(companyID, traceID, spanID)},
		token,
		ttlMillis,
	)
	if err != nil {
		return fmt.Errorf("complete OTLP span claim: %w", err)
	}
	completed, ok := result.(int64)
	if !ok {
		return fmt.Errorf("complete OTLP span claim returned %T, expected int64", result)
	}
	if completed != 1 {
		return errors.New("OTLP span claim ownership was lost")
	}
	return nil
}

func (r *OTelTraceRepository) ReleaseSpan(ctx context.Context, companyID, traceID, spanID, token string) error {
	if _, err := r.valkey.Eval(ctx, releaseOTelCreationLockScript, []string{otelSpanKey(companyID, traceID, spanID)}, token); err != nil {
		return fmt.Errorf("release OTLP span claim: %w", err)
	}
	return nil
}

func (r *OTelTraceRepository) ReleaseCreationLock(ctx context.Context, companyID, traceID, token string) error {
	if _, err := r.valkey.Eval(ctx, releaseOTelCreationLockScript, []string{otelTraceLockKey(companyID, traceID)}, token); err != nil {
		return fmt.Errorf("release OTLP trace creation lock: %w", err)
	}
	return nil
}

func otelTraceKey(companyID, traceID string) string {
	return fmt.Sprintf("otel:trace:%s:%s", companyID, traceID)
}

func otelTraceLockKey(companyID, traceID string) string {
	return otelTraceKey(companyID, traceID) + ":create"
}

func otelTraceCompletedKey(companyID, traceID string) string {
	return otelTraceKey(companyID, traceID) + ":completed"
}

func otelSpanKey(companyID, traceID, spanID string) string {
	return fmt.Sprintf("otel:span:%s:%s:%s", companyID, traceID, spanID)
}
