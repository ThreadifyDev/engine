package models

import "time"

type ConnectedClient struct {
	OwnerID          string    `json:"ownerId"`
	ApiKey           string    `json:"apiKey"`
	ServiceName      string    `json:"serviceName"`
	ConnectedAt      time.Time `json:"connectedAt"`
	SubscribedEvents []string  `json:"subscribedEvents"`
}
