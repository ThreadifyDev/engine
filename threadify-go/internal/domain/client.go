package domain

import "time"

type ConnectedClient struct {
	OwnerID          string
	CompanyID        string
	ApiKey           string
	ServiceName      string
	ConnectedAt      time.Time
	SubscribedEvents []string
}
