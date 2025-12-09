package models

import "time"

type ConnectedClient struct {
	OwnerID          string    `json:"ownerId"`
	ApiKey           string    `json:"apiKey"`
	ConnectedAt      time.Time `json:"connectedAt"`
	SubscribedEvents []string  `json:"subscribedEvents"`
}
