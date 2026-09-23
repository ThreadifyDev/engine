package domain

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
)

var ErrProfileMetricBinding = errors.New("metric does not belong to this profile type")

var ErrProfileViewConflict = errors.New("profile view revision conflict")

type ProfileViewDefinition struct {
	SchemaVersion int                `json:"schemaVersion"`
	Title         string             `json:"title"`
	Description   string             `json:"description"`
	Range         string             `json:"range"`
	Columns       int                `json:"columns"`
	Blocks        []ProfileViewBlock `json:"blocks"`
}
type ProfileViewBlock struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Title    string `json:"title"`
	MetricID string `json:"metricId,omitempty"`
	Display  string `json:"display,omitempty"`
}
type ProfileViewRequest struct {
	BlockID    string            `json:"blockId"`
	Operation  string            `json:"operation"`
	Field      string            `json:"field"`
	Parameters map[string]string `json:"parameters"`
}
type SavedProfileView struct {
	ProfileTypeID string                 `json:"profile_type_id"`
	Definition    *ProfileViewDefinition `json:"definition"`
	Requests      []ProfileViewRequest   `json:"requests"`
	Revision      int64                  `json:"revision"`
	UpdatedAt     *time.Time             `json:"updated_at"`
	UpdatedBy     string                 `json:"updated_by"`
}

type profileViewSource struct{ operation, field string }

// This catalog is deliberately finite. History remains in its dedicated tab.
var profileViewSources = map[string]profileViewSource{
	"totalDeliveries":       {"entityProfile", "metrics.totalDeliveries"},
	"completedSuccessfully": {"entityProfile", "metrics.completedSuccessfully"},
	"validationViolations":  {"entityProfile", "metrics.validationViolations"},
	"averageDeliveryTimeMs": {"entityProfile", "metrics.averageDeliveryTimeMs"},
	"deliveryHealthScore":   {"entityProfile", "metrics.deliveryHealthScore"},
	"details":               {"entityProfile", ""},
	"configuredMetric":      {"getComputedMetrics", "computedMetrics"},
	"computedMetrics":       {"getComputedMetrics", "computedMetrics"},
	"deliveryHealth":        {"getDeliveryHealthMetrics", "deliveryHealth"},
	"deliveryTrendChart":    {"getDeliveryHealthMetrics", "deliveryHealth"},
	"deliveryHealthChart":   {"getDeliveryHealthMetrics", "deliveryHealth"},
}
var profileViewBlockID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func viewTextLength(s string) int { return len(utf16.Encode([]rune(s))) }

func (v *ProfileViewDefinition) Validate() error {
	if v == nil || v.SchemaVersion != 1 {
		return errors.New("unsupported profile view version")
	}
	if strings.TrimSpace(v.Title) == "" || viewTextLength(v.Title) > 100 {
		return errors.New("view title must contain 1–100 characters")
	}
	if viewTextLength(v.Description) > 500 {
		return errors.New("description must contain at most 500 characters")
	}
	if v.Range != "7d" && v.Range != "30d" && v.Range != "90d" {
		return errors.New("range must be 7d, 30d, or 90d")
	}
	if v.Columns != 1 && v.Columns != 2 {
		return errors.New("columns must be 1 or 2")
	}
	if len(v.Blocks) < 1 || len(v.Blocks) > 100 {
		return errors.New("a view must contain 1–100 blocks")
	}
	ids, sources := map[string]bool{}, map[string]bool{}
	for i := range v.Blocks {
		b := &v.Blocks[i]
		if !profileViewBlockID.MatchString(b.ID) || ids[b.ID] {
			return errors.New("block IDs must be unique letters, numbers, dashes, or underscores")
		}
		if _, ok := profileViewSources[b.Source]; !ok {
			return fmt.Errorf("unsupported Overview data source: %s", b.Source)
		}
		binding := b.Source
		if b.Source == "configuredMetric" {
			if _, err := uuid.Parse(b.MetricID); err != nil {
				return errors.New("choose a saved metric definition")
			}
			if b.Display != "card" && b.Display != "table" && b.Display != "line" && b.Display != "bar" {
				return errors.New("unsupported metric presentation")
			}
			binding = "metric:" + b.MetricID
		} else if b.MetricID != "" || b.Display != "" {
			return errors.New("metric bindings require configuredMetric source")
		}
		if sources[binding] {
			return errors.New("each data source can be used only once")
		}
		if strings.TrimSpace(b.Title) == "" || viewTextLength(b.Title) > 100 {
			return errors.New("block title must contain 1–100 characters")
		}
		ids[b.ID], sources[binding] = true, true
	}
	return nil
}

// CompileProfileView validates and canonicalizes the definition, then derives the
// request plan on the server. Clients never choose arbitrary calls or entity scope.
func CompileProfileView(v ProfileViewDefinition) (*ProfileViewDefinition, []ProfileViewRequest, error) {
	if err := v.Validate(); err != nil {
		return nil, nil, err
	}
	v.Title = strings.TrimSpace(v.Title)
	v.Blocks = append([]ProfileViewBlock(nil), v.Blocks...)
	requests := make([]ProfileViewRequest, 0, len(v.Blocks))
	for i := range v.Blocks {
		b := &v.Blocks[i]
		b.Title = strings.TrimSpace(b.Title)
		source := profileViewSources[b.Source]
		params := map[string]string{"refKey": "$entity.refKey", "type": "$profileType.name"}
		if b.Source == "configuredMetric" || b.Source == "computedMetrics" || b.Source == "deliveryHealth" || b.Source == "deliveryHealthChart" || b.Source == "deliveryTrendChart" {
			params["range"] = v.Range
		}
		if b.MetricID != "" {
			params["metricId"] = b.MetricID
		}
		requests = append(requests, ProfileViewRequest{BlockID: b.ID, Operation: source.operation, Field: source.field, Parameters: params})
	}
	return &v, requests, nil
}
