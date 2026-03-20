package interfaces

import (
	"threadify-go/shared/billing"
)

type InvoiceProvider interface {
	billing.InvoiceProvider
}

type WebhookProvider interface {
	billing.WebhookProvider
}
