package interfaces

import (
	"threadify-go/shared/billing"
)

type InvoiceProvider interface {
	billing.CheckoutSessionProvider
	billing.InvoiceProvider
}

type WebhookProvider interface {
	billing.WebhookProvider
}
