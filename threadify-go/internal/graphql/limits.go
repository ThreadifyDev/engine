package graphql

// Query complexity and pagination limits to protect against abuse
const (
	// MaxStepsPerQuery is the maximum number of steps that can be requested in a single query
	MaxStepsPerQuery = 100

	// MaxHistoryPerQuery is the maximum number of history records that can be requested
	MaxHistoryPerQuery = 1000

	// MaxValidationsPerQuery is the maximum number of validation results that can be requested
	MaxValidationsPerQuery = 100

	// MaxThreadsPerQuery is the maximum number of threads that can be requested
	MaxThreadsPerQuery = 100

	// MaxThreadChainDepth is the maximum depth for thread chain traversal
	MaxThreadChainDepth = 10
)

// EnforceLimit ensures a limit value doesn't exceed the maximum allowed
func EnforceLimit(limit *int, maxLimit int, defaultLimit int) int {
	if limit == nil {
		return defaultLimit
	}

	if *limit <= 0 {
		return defaultLimit
	}

	if *limit > maxLimit {
		return maxLimit
	}

	return *limit
}
