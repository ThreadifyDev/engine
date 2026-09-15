package handlers

import (
	"errors"
	"net/http"
	"testing"

	shareddomain "threadify-go/shared/domain"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestWebhookHandler_HandleWebhook(t *testing.T) {
	const (
		sigHeader = "Test-Provider-Signature"
		sigValue  = "valid_sig"
	)

	tests := []struct {
		name       string
		header     string
		body       string
		setupMock  func(d *MockedEngineHandlers)
		wantStatus int
	}{
		{
			name:   "success_invoice_paid",
			header: sigValue,
			body:   `{"type":"invoice.paid"}`,
			setupMock: func(d *MockedEngineHandlers) {
				d.WebhookProvider.EXPECT().SignatureHeader().Return(sigHeader)
				d.WebhookProvider.EXPECT().Name().Return("test-provider").AnyTimes()
				d.WebhookProvider.EXPECT().
					VerifyAndParse(gomock.Any(), sigValue).
					Return(&shareddomain.WebhookEvent{
						Type:              "invoice.paid",
						ExternalInvoiceID: "inv_123",
					}, nil)

				snapshot := &shareddomain.BillingSnapshot{
					ID:                "snap_1",
					ExternalInvoiceID: "inv_123",
					CompanyID:         testCompanyID,
				}
				d.BillingWebhookSvc.EXPECT().
					FindSnapshotByInvoiceID(gomock.Any(), "inv_123").
					Return(snapshot, nil)

				d.BillingWebhookSvc.EXPECT().
					MarkSnapshotPaid(gomock.Any(), "inv_123").
					Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "invalid_signature",
			header: "bad_sig",
			body:   `{}`,
			setupMock: func(d *MockedEngineHandlers) {
				d.WebhookProvider.EXPECT().SignatureHeader().Return(sigHeader)
				d.WebhookProvider.EXPECT().Name().Return("test-provider").AnyTimes()
				d.WebhookProvider.EXPECT().
					VerifyAndParse(gomock.Any(), "bad_sig").
					Return(nil, errors.New("invalid signature"))
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "checkout_session_completed",
			header: sigValue,
			body:   `{}`,
			setupMock: func(d *MockedEngineHandlers) {
				d.WebhookProvider.EXPECT().SignatureHeader().Return(sigHeader)
				d.WebhookProvider.EXPECT().Name().Return("test-provider").AnyTimes()
				d.WebhookProvider.EXPECT().
					VerifyAndParse(gomock.Any(), sigValue).
					Return(&shareddomain.WebhookEvent{
						Type:               "checkout.session.completed",
						ExternalCustomerID: "cus_1",
						AmountMillicents:   1000,
						Metadata:           map[string]string{"company_id": testCompanyID},
					}, nil)

				d.BillingWebhookSvc.EXPECT().
					ProvisionSubscription(gomock.Any(), testCompanyID, "cus_1", int64(1000)).
					Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "unhandled_event_type",
			header: sigValue,
			body:   `{"type":"unknown.event"}`,
			setupMock: func(d *MockedEngineHandlers) {
				d.WebhookProvider.EXPECT().SignatureHeader().Return(sigHeader)
				d.WebhookProvider.EXPECT().Name().Return("test-provider").AnyTimes()
				d.WebhookProvider.EXPECT().
					VerifyAndParse(gomock.Any(), sigValue).
					Return(&shareddomain.WebhookEvent{
						Type: "unknown.event",
					}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "processing_error",
			header: sigValue,
			body:   `{"type":"invoice.paid"}`,
			setupMock: func(d *MockedEngineHandlers) {
				d.WebhookProvider.EXPECT().SignatureHeader().Return(sigHeader)
				d.WebhookProvider.EXPECT().Name().Return("test-provider").AnyTimes()
				d.WebhookProvider.EXPECT().
					VerifyAndParse(gomock.Any(), sigValue).
					Return(&shareddomain.WebhookEvent{
						Type:              "invoice.paid",
						ExternalInvoiceID: "inv_err",
					}, nil)

				d.BillingWebhookSvc.EXPECT().
					FindSnapshotByInvoiceID(gomock.Any(), "inv_err").
					Return(nil, errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewMockedEngineHandlers(t)
			tt.setupMock(d)

			h := NewWebhookHandler(d.WebhookProvider, d.BillingWebhookSvc, d.Logger)
			r := SetupTestRouter()

			req := BuildRequest(t, http.MethodPost, "/webhooks/billing", tt.body)
			req.Header.Set(sigHeader, tt.header)

			r.POST("/webhooks/billing", h.HandleWebhook)
			resp := DoRequestFromReq(t, r, req)

			assert.Equal(t, tt.wantStatus, resp.Code)
		})
	}
}
