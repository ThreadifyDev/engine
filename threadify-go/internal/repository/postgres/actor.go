package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

type ActorRepository struct {
	db *pgxpool.Pool
}

func NewActorRepository(db *pgxpool.Pool) *ActorRepository {
	return &ActorRepository{db: db}
}

// ResolveActors fetches user and service account information by IDs
// Uses a single UNION ALL query for optimal performance
func (r *ActorRepository) ResolveActors(ctx context.Context, ids []string) ([]*models.ActorInfo, error) {
	if len(ids) == 0 {
		return []*models.ActorInfo{}, nil
	}

	// Single optimized query using UNION ALL
	// Benefits:
	// - Single round-trip to database
	// - Uses primary key indexes on both tables
	// - LEFT JOIN uses foreign key indexes
	// - ANY($1) is more efficient than IN clause for arrays
	query := `
		SELECT u.id, u.email, 'user' as type, c.name as company_name
		FROM users u
		LEFT JOIN companies c ON u.company_id = c.id
		WHERE u.id = ANY($1)
		UNION ALL
		SELECT sa.id, sa.name, 'service_account' as type, c.name as company_name
		FROM service_accounts sa
		LEFT JOIN companies c ON sa.company_id = c.id
		WHERE sa.id = ANY($1)
	`

	rows, err := r.db.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve actors: %w", err)
	}
	defer rows.Close()

	actors := make([]*models.ActorInfo, 0, len(ids))
	for rows.Next() {
		var id, name, actorType string
		var companyName *string
		if err := rows.Scan(&id, &name, &actorType, &companyName); err != nil {
			continue // Skip invalid rows
		}
		actors = append(actors, &models.ActorInfo{
			ID:          id,
			Name:        name,
			Type:        actorType,
			CompanyName: companyName,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating actor rows: %w", err)
	}

	return actors, nil
}
