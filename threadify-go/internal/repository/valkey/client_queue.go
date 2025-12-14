package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/models"
)

type ClientQueue struct {
	valkey ValkeyClient
	ttl    time.Duration
}

func NewClientQueue(valkey ValkeyClient, ttlSeconds int) *ClientQueue {
	return &ClientQueue{
		valkey: valkey,
		ttl:    time.Duration(ttlSeconds) * time.Second,
	}
}

func (q *ClientQueue) AddClient(client *models.ConnectedClient) error {
	data, err := json.Marshal(client)
	if err != nil {
		return fmt.Errorf("marshal client: %w", err)
	}

	key := fmt.Sprintf("client:%s", client.OwnerID)
	return q.valkey.Set(context.Background(), key, string(data), q.ttl)
}

func (q *ClientQueue) GetClient(ownerID string) (*models.ConnectedClient, error) {
	key := fmt.Sprintf("client:%s", ownerID)
	data, err := q.valkey.Get(context.Background(), key)
	if err != nil {
		return nil, err
	}

	var client models.ConnectedClient
	if err := json.Unmarshal([]byte(data), &client); err != nil {
		return nil, fmt.Errorf("unmarshal client: %w", err)
	}

	return &client, nil
}

func (q *ClientQueue) RemoveClient(ownerID string) error {
	key := fmt.Sprintf("client:%s", ownerID)
	return q.valkey.Delete(context.Background(), key)
}
