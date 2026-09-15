package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
)

type nullableOwnerThreadRow struct {
	ownerID any
}

func (r nullableOwnerThreadRow) Scan(dest ...any) error {
	*(dest[0].(*string)) = "thread-1"
	*(dest[1].(**string)) = nil
	*(dest[2].(**string)) = nil
	*(dest[3].(**string)) = nil
	*(dest[4].(**int)) = nil
	if err := dest[5].(*sql.NullString).Scan(r.ownerID); err != nil {
		return err
	}
	*(dest[6].(*string)) = "company-1"
	*(dest[7].(**string)) = nil
	*(dest[8].(**string)) = nil
	*(dest[9].(*time.Time)) = time.Unix(1, 0)
	*(dest[10].(*time.Time)) = time.Unix(2, 0)
	*(dest[11].(**time.Time)) = nil
	*(dest[12].(*[]string)) = []string{}
	return nil
}

func TestScanThreadRowAcceptsNullableOwnerID(t *testing.T) {
	for _, test := range []struct {
		name    string
		ownerID any
		want    string
	}{
		{name: "deleted service account", ownerID: nil, want: ""},
		{name: "active service account", ownerID: "service-1", want: "service-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var thread domain.Thread

			err := scanThreadRow(nullableOwnerThreadRow{ownerID: test.ownerID}, &thread)

			require.NoError(t, err)
			require.Equal(t, test.want, thread.OwnerID)
			require.Equal(t, "company-1", thread.CompanyID)
			require.Equal(t, domain.ThreadStatusActive, thread.Status)
		})
	}
}
