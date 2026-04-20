package service_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"
)

func TestConnectionService_ConnectAndRefresh_Table(t *testing.T) {
	tests := []struct {
		name           string
		first          [4]string
		second         *[4]string
		wantSessions   int
		wantCompany    string
		wantService    string
		wantApiKey     string
		expectExisting bool
	}{
		{
			name:         "first connect creates client and session",
			first:        [4]string{"o1", "k1", "svc1", "c1"},
			wantSessions: 1,
			wantCompany:  "c1",
			wantService:  "svc1",
			wantApiKey:   "k1",
		},
		{
			name:           "second connect refreshes client and increments sessions",
			first:          [4]string{"o1", "k1", "svc1", "c1"},
			second:         &[4]string{"o1", "k2", "svc2", "c2"},
			wantSessions:   2,
			wantCompany:    "c2",
			wantService:    "svc2",
			wantApiKey:     "k2",
			expectExisting: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewConnectionService(zap.NewNop()).(*service.ConnectionService)

			require.NoError(t, svc.ConnectWithOwnerAndCompany(tc.first[0], tc.first[1], tc.first[2], tc.first[3]))
			var firstConnectedAt time.Time
			if tc.expectExisting {
				c, ok := svc.GetClient(tc.first[0])
				require.True(t, ok)
				firstConnectedAt = c.ConnectedAt
			}

			if tc.second != nil {
				require.NoError(t, svc.ConnectWithOwnerAndCompany((*tc.second)[0], (*tc.second)[1], (*tc.second)[2], (*tc.second)[3]))
			}

			c, ok := svc.GetClient(tc.first[0])
			require.True(t, ok)
			require.Equal(t, tc.wantCompany, c.CompanyID)
			require.Equal(t, tc.wantService, c.ServiceName)
			require.Equal(t, tc.wantApiKey, c.ApiKey)
			if tc.expectExisting {
				require.True(t, c.ConnectedAt.After(firstConnectedAt) || c.ConnectedAt.Equal(firstConnectedAt))
			}

			require.True(t, svc.IsConnected(tc.first[0]))
			gotCompany, ok := svc.GetClientCompany(tc.first[0])
			require.True(t, ok)
			require.Equal(t, tc.wantCompany, gotCompany)
			require.Equal(t, tc.wantSessions, svc.GetSessionCount(tc.first[0]))
		})
	}
}

func TestConnectionService_Disconnect_Table(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(svc *service.ConnectionService)
		ownerID       string
		wantConnected bool
		wantSessions  int
	}{
		{
			name:          "unknown owner is no-op",
			setup:         func(_ *service.ConnectionService) {},
			ownerID:       "missing",
			wantConnected: false,
			wantSessions:  0,
		},
		{
			name: "decrements when multiple sessions",
			setup: func(svc *service.ConnectionService) {
				require.NoError(t, svc.ConnectWithOwnerAndCompany("o1", "k1", "svc", "c1"))
				require.NoError(t, svc.ConnectWithOwnerAndCompany("o1", "k1", "svc", "c1"))
			},
			ownerID:       "o1",
			wantConnected: true,
			wantSessions:  1,
		},
		{
			name: "removes client on last session",
			setup: func(svc *service.ConnectionService) {
				require.NoError(t, svc.ConnectWithOwnerAndCompany("o1", "k1", "svc", "c1"))
			},
			ownerID:       "o1",
			wantConnected: false,
			wantSessions:  0,
		},
		{
			name: "disconnect removes client on last session",
			setup: func(svc *service.ConnectionService) {
				require.NoError(t, svc.ConnectWithOwnerAndCompany("o1", "k1", "svc", "c1"))
			},
			ownerID:       "o1",
			wantConnected: false,
			wantSessions:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewConnectionService(zap.NewNop()).(*service.ConnectionService)
			tc.setup(svc)

			require.NoError(t, svc.Disconnect(tc.ownerID))

			require.Equal(t, tc.wantConnected, svc.IsConnected(tc.ownerID))
			require.Equal(t, tc.wantSessions, svc.GetSessionCount(tc.ownerID))
		})
	}
}

func TestConnectionService_ConcurrentConnectDisconnect_IsRaceSafe(t *testing.T) {
	svc := service.NewConnectionService(zap.NewNop()).(*service.ConnectionService)

	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = svc.ConnectWithOwnerAndCompany("o1", "k1", "svc", "c1")
			_ = svc.Disconnect("o1")
		}()
	}

	wg.Wait()
}
