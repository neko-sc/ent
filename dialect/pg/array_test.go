package pg

import (
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIntegerArrayScan(t *testing.T) {
	for _, oid := range []uint32{pgtype.Int4ArrayOID, pgtype.Int8ArrayOID} {
		var integers []int
		var wide []int64
		require.NoError(t, pgtype.NewMap().Scan(oid, pgtype.TextFormatCode, []byte("{1,2,-3}"), &integers))
		require.NoError(t, pgtype.NewMap().Scan(oid, pgtype.TextFormatCode, []byte("{1,2,-3}"), &wide))
		require.Equal(t, []int{1, 2, -3}, integers)
		require.Equal(t, []int64{1, 2, -3}, wide)
	}
}
