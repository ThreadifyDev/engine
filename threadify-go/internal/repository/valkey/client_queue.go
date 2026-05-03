package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/domain"
)

type ClientQueue struct {
	valkey domain.ValkeyStringClient
	ttl    time.Duration
}

func NewClientQueue(valkey domain.ValkeyStringClient, ttlSeconds int) *ClientQueue {
	return &ClientQueue{
		valkey: valkey,
		ttl:    time.Duration(ttlSeconds) * time.Second,
	}
}

func (q *ClientQueue) AddClient(client *domain.ConnectedClient) error {
	data, err := json.Marshal(fromConnectedClientDomain(client))
	if err != nil {
		return fmt.Errorf("marshal client: %w", err)
	}

	key := fmt.Sprintf("client:%s", client.OwnerID)
	return q.valkey.Set(context.Background(), key, string(data), q.ttl)
}

func (q *ClientQueue) GetClient(ownerID string) (*domain.ConnectedClient, error) {
	key := fmt.Sprintf("client:%s", ownerID)
	data, err := q.valkey.Get(context.Background(), key)
	if err != nil {
		return nil, err
	}

	var model connectedClientModel
	if err := json.Unmarshal([]byte(data), &model); err != nil {
		return nil, fmt.Errorf("unmarshal client: %w", err)
	}

	return model.ToDomain(), nil
}

func (q *ClientQueue) RemoveClient(ownerID string) error {
	key := fmt.Sprintf("client:%s", ownerID)
	return q.valkey.Del(context.Background(), key)
}
