package utils

import (
	"hash/fnv"
)

// GetPartitionForThread calculates the partition number for a thread ID
// Uses FNV hash for consistent distribution across partitions
func GetPartitionForThread(threadID string) int {
	const numPartitions = 10 // Should match archiver config
	h := fnv.New32a()
	h.Write([]byte(threadID))
	return int(h.Sum32() % uint32(numPartitions))
}

// GetPartitionForUserID calculates the partition number for a user ID
// Uses FNV hash for consistent distribution across partitions
func GetPartitionForUserID(userID string) int {
	const numPartitions = 10 // Should match archiver config
	h := fnv.New32a()
	h.Write([]byte(userID))
	return int(h.Sum32() % uint32(numPartitions))
}
