package archiver

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseUsageTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    time.Time
		wantErr bool
	}{
		{
			name:  "RFC3339 with fractional seconds (nano)",
			input: "2026-03-18T03:59:34.414345Z",
			want:  time.Date(2026, 3, 18, 3, 59, 34, 414345000, time.UTC),
		},
		{
			name:  "RFC3339 without fractional seconds",
			input: "2026-03-18T03:59:34Z",
			want:  time.Date(2026, 3, 18, 3, 59, 34, 0, time.UTC),
		},
		{
			name:    "Invalid format",
			input:   "2026-03-18 03:59:34",
			wantErr: true,
		},
		{
			name:  "Empty string",
			input: "",
			want:  time.Time{},
		},
		{
			name:  "Nil input",
			input: nil,
			want:  time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUsageTimestamp(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, got.Equal(tt.want), "got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseUsageAmount(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    int64
		wantErr bool
	}{
		{name: "float64", input: float64(42), want: 42},
		{name: "int64", input: int64(42), want: 42},
		{name: "int", input: int(42), want: 42},
		{name: "json.Number", input: json.Number("42"), want: 42},
		{name: "string", input: "42", want: 42},
		{name: "invalid string", input: "bad", wantErr: true},
		{name: "unsupported type", input: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUsageAmount(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
