package database

const (
	// PlanCachePrefix is the prefix for cached plan limits
	PlanCachePrefix = "plan:limits:"

	// BalanceKeyPrefix is the prefix for real-time balance tracking
	BalanceKeyPrefix = "plan:balance:"

	// UsageContractKeyPrefix is the prefix for contract quota tracking
	UsageContractKeyPrefix = "plan:usage:contracts:"

	// UsageUserKeyPrefix is the prefix for user seat quota tracking
	UsageUserKeyPrefix = "plan:usage:users:"

	// UsageOutboxStreamKey is the key for the usage sync outbox stream
	UsageOutboxStreamKey = "usage:outbox:events"

	// SuspendedPlanPrefix is the prefix for suspended plans due to payment failure
	SuspendedPlanPrefix = "plan:suspended:"

	// UsageSyncSubject is the NATS subject for synchronizing usage events
	UsageSyncSubject = "usage.sync"
)
