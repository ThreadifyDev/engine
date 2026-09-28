package validation

import (
	"testing"

	"threadify-go/shared/management/dto"
)

func TestRetentionDaysRange(t *testing.T) {
	for _, days := range []int{-1, 36501} {
		req := &dto.UpdateProfileRequest{FullName: "Test", JobRole: "Engineer", ThreadRetentionDays: &days}
		if err := ValidateUpdateProfileRequest(req); err == nil {
			t.Fatalf("expected %d to be rejected", days)
		}
	}
	for _, days := range []int{0, 1, 36500} {
		req := &dto.UpdateProfileRequest{FullName: "Test", JobRole: "Engineer", ThreadRetentionDays: &days}
		if err := ValidateUpdateProfileRequest(req); err != nil {
			t.Fatalf("expected %d to be accepted: %v", days, err)
		}
	}
}
