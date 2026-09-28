package validation

import (
	"testing"
	"time"

	"threadify-go/shared/management/dto"
)

func TestValidateCreateAPIKeyExpiry(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-time.Hour)
	days := 30
	tests := []struct {
		name      string
		days      *int
		date      *time.Time
		wantField string
	}{
		{name: "custom date", date: &future},
		{name: "duration", days: &days},
		{name: "past date", date: &past, wantField: "expires_at"},
		{name: "both forms", days: &days, date: &future, wantField: "expires_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateAPIKeyRequest(&dto.CreateAPIKeyRequest{Name: "test", ExpiresIn: tt.days, ExpiresAt: tt.date})
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("unexpected validation error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			problems := err.(*RequestValidationError).Problems()
			if len(problems) != 1 || problems[0].Field != tt.wantField {
				t.Fatalf("wrong validation field: %+v", problems)
			}
		})
	}
}
