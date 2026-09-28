package postgres

import (
	"errors"
	"testing"

	shderrors "threadify-go/shared/errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestContractErrDuplicateName(t *testing.T) {
	for _, constraint := range []string{
		constrContractNameActive,
		constrContractNameCompanyActive,
		constrUniqueContractName,
	} {
		t.Run(constraint, func(t *testing.T) {
			err := contractErr(&pgconn.PgError{Code: pgerrcode.UniqueViolation, ConstraintName: constraint})
			if !errors.Is(err, shderrors.ErrContractAlreadyExists) {
				t.Fatalf("duplicate name error was not mapped: %v", err)
			}
		})
	}

	err := contractErr(&pgconn.PgError{Code: pgerrcode.UniqueViolation, ConstraintName: "contracts_pkey"})
	if errors.Is(err, shderrors.ErrContractAlreadyExists) {
		t.Fatalf("unrelated unique violation was mapped as a duplicate name: %v", err)
	}
}
