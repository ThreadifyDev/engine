package database

const (
	// PlanCachePrefix is the prefix for cached plan limits
	PlanCachePrefix = "plan:limits:"

	// UsageOutboxStreamKey is the key for the usage sync outbox stream
	UsageOutboxStreamKey = "usage:outbox:events"

	// UsageSyncSubject is the NATS subject for synchronizing usage events
	UsageSyncSubject = "usage.sync"

	// CreditBalanceKeyPrefix stores current credit balance (millicents).
	CreditBalanceKeyPrefix = "plan:credit:balance:"

	// CreditMonthlyChargedKeyPrefix stores current cycle charged amount (millicents).
	CreditMonthlyChargedKeyPrefix = "plan:credit:charged:"

	// CreditTopupPendingKeyPrefix stores pending top-up amount (millicents) awaiting payment.
	CreditTopupPendingKeyPrefix = "plan:credit:topup:pending:"

	// CreditTopupAppliedKeyPrefix stores applied top-up invoice IDs for idempotency.
	CreditTopupAppliedKeyPrefix = "plan:credit:topup:applied:"

	// CreditRolloverLockKey is a global lock for monthly credit rollover.
	CreditRolloverLockKey = "plan:credit:rollover:lock"
)
