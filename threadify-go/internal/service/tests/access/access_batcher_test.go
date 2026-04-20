package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

type batcher struct {
	mockRepo *enginemocks.MockAccessRepository
	mockLua  *enginemocks.MockLuaScriptManager
	logger   *zap.Logger
}

func newbatcher(ctrl *gomock.Controller) *batcher {
	return &batcher{
		mockRepo: enginemocks.NewMockAccessRepository(ctrl),
		mockLua:  enginemocks.NewMockLuaScriptManager(ctrl),
		logger:   zap.NewNop(),
	}
}

func makeWrites(n int, threadID string, resultCh chan error) []*service.AccessWrite {
	writes := make([]*service.AccessWrite, n)
	for i := range writes {
		writes[i] = &service.AccessWrite{
			ThreadID:    threadID,
			UserID:      fmt.Sprintf("u%d", i+1),
			Role:        "member",
			RuntimeRole: "scope",
			InvitedBy:   "inviter",
			ResultChan:  resultCh,
		}
	}
	return writes
}

func collectResults(t *testing.T, ch chan error, n int, deadline time.Duration) []error {
	t.Helper()
	results := make([]error, 0, n)
	timeout := time.After(deadline)
	for i := 0; i < n; i++ {
		select {
		case err := <-ch:
			results = append(results, err)
		case <-timeout:
			t.Fatalf("timed out after %s waiting for flush results (%d/%d received)",
				deadline, len(results), n)
		}
	}
	return results
}

func expectGrantCalls(h *batcher, writes []*service.AccessWrite, retErr error) {
	for _, w := range writes {
		h.mockRepo.EXPECT().GrantOrUpdateAccess(
			gomock.Any(),
			w.ThreadID, w.UserID, w.Role, w.RuntimeRole,
			gomock.Any(), w.InvitedBy,
			h.mockLua, nil, nil,
		).Return(nil, retErr).Times(1)
	}
}

func TestAccessBatcher(t *testing.T) {
	tests := []struct {
		name          string
		bufferSize    int
		batchSize     int
		flushInterval time.Duration
		startBatcher  bool
		writes        func(h *batcher) []*service.AccessWrite
		setupMocks    func(h *batcher, writes []*service.AccessWrite)
		run           func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, h *batcher)
	}{
		{
			name:          "flushes when batch size is reached",
			bufferSize:    10,
			batchSize:     2,
			flushInterval: time.Second,
			startBatcher:  true,
			writes: func(h *batcher) []*service.AccessWrite {
				return makeWrites(2, "t1", make(chan error, 2))
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
				expectGrantCalls(h, writes, nil)
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				for _, w := range writes {
					require.NoError(t, b.Write(w))
				}
				results := collectResults(t, writes[0].ResultChan, len(writes), 2*time.Second)
				for _, err := range results {
					assert.NoError(t, err)
				}
			},
		},

		{
			name:          "flushes on interval when batch size not reached",
			bufferSize:    10,
			batchSize:     10,
			flushInterval: 100 * time.Millisecond,
			startBatcher:  true,
			writes: func(h *batcher) []*service.AccessWrite {
				return makeWrites(1, "t1", make(chan error, 1))
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
				expectGrantCalls(h, writes, nil)
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				require.NoError(t, b.Write(writes[0]))
				results := collectResults(t, writes[0].ResultChan, 1, 500*time.Millisecond)
				assert.NoError(t, results[0])
			},
		},

		{
			name:          "repo error is propagated through result channel",
			bufferSize:    10,
			batchSize:     1,
			flushInterval: time.Second,
			startBatcher:  true,
			writes: func(h *batcher) []*service.AccessWrite {
				return makeWrites(1, "t1", make(chan error, 1))
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
				expectGrantCalls(h, writes, errors.New("repo error"))
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				require.NoError(t, b.Write(writes[0]))
				results := collectResults(t, writes[0].ResultChan, 1, 2*time.Second)
				assert.EqualError(t, results[0], "repo error")
			},
		},

		{
			name:          "nil ResultChan does not panic during batch flush",
			bufferSize:    10,
			batchSize:     1,
			flushInterval: time.Second,
			startBatcher:  true,
			writes: func(h *batcher) []*service.AccessWrite {
				return []*service.AccessWrite{
					{ThreadID: "t1", UserID: "u1", Role: "member", RuntimeRole: "scope", InvitedBy: "inviter"},
				}
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
				h.mockRepo.EXPECT().GrantOrUpdateAccess(
					gomock.Any(),
					writes[0].ThreadID, writes[0].UserID, writes[0].Role, writes[0].RuntimeRole,
					gomock.Any(), writes[0].InvitedBy,
					h.mockLua, nil, nil,
				).Return(nil, nil).Times(1)
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				require.NoError(t, b.Write(writes[0]))
				time.Sleep(200 * time.Millisecond)
			},
		},
		{
			name:          "falls back to sync write when buffer is full",
			bufferSize:    0,
			batchSize:     5,
			flushInterval: time.Hour,
			startBatcher:  false,
			writes: func(h *batcher) []*service.AccessWrite {
				return []*service.AccessWrite{
					{ThreadID: "t1", UserID: "u1", Role: "member", RuntimeRole: "scope", InvitedBy: "inviter"},
				}
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
				h.mockRepo.EXPECT().GrantOrUpdateAccess(
					gomock.Any(),
					writes[0].ThreadID, writes[0].UserID, writes[0].Role, writes[0].RuntimeRole,
					gomock.Any(), writes[0].InvitedBy,
					h.mockLua, nil, nil,
				).Return(nil, errors.New("sync error")).Times(1)
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				err := b.Write(writes[0])
				assert.EqualError(t, err, "sync error")
			},
		},
		{
			name:          "drains buffer and flushes partial batch on graceful stop",
			bufferSize:    10,
			batchSize:     5,
			flushInterval: time.Hour,
			startBatcher:  true,
			writes: func(h *batcher) []*service.AccessWrite {
				return makeWrites(1, "t1", make(chan error, 1))
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
				expectGrantCalls(h, writes, nil)
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				require.NoError(t, b.Write(writes[0]))
				b.Stop()
				results := collectResults(t, writes[0].ResultChan, 1, time.Second)
				assert.NoError(t, results[0])
			},
		},

		{
			name:          "Write after Stop returns context.Canceled",
			bufferSize:    10,
			batchSize:     5,
			flushInterval: time.Hour,
			startBatcher:  true,
			writes: func(h *batcher) []*service.AccessWrite {
				return makeWrites(1, "t1", nil)
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				b.Stop()
				err := b.Write(writes[0])
				assert.ErrorIs(t, err, context.Canceled)
			},
		},

		{
			name:          "second batch flushes independently after ticker reset",
			bufferSize:    10,
			batchSize:     2,
			flushInterval: time.Second,
			startBatcher:  true,
			writes: func(h *batcher) []*service.AccessWrite {
				return makeWrites(4, "t1", make(chan error, 4))
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
				expectGrantCalls(h, writes, nil)
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				require.NoError(t, b.Write(writes[0]))
				require.NoError(t, b.Write(writes[1]))
				collectResults(t, writes[0].ResultChan, 2, 2*time.Second)

				require.NoError(t, b.Write(writes[2]))
				require.NoError(t, b.Write(writes[3]))
				collectResults(t, writes[0].ResultChan, 2, 2*time.Second)
			},
		},

		{
			name:          "concurrent writes do not corrupt batch or panic",
			bufferSize:    100,
			batchSize:     10,
			flushInterval: 50 * time.Millisecond,
			startBatcher:  true,
			writes: func(h *batcher) []*service.AccessWrite {
				return makeWrites(50, "t1", make(chan error, 50))
			},
			setupMocks: func(h *batcher, writes []*service.AccessWrite) {
				h.mockRepo.EXPECT().GrantOrUpdateAccess(
					gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(),
					h.mockLua, nil, nil,
				).Return(nil, nil).Times(len(writes))
			},
			run: func(t *testing.T, b *service.AccessBatcher, writes []*service.AccessWrite, _ *batcher) {
				var wg sync.WaitGroup
				for _, w := range writes {
					wg.Add(1)
					go func(w *service.AccessWrite) {
						defer wg.Done()
						assert.NoError(t, b.Write(w))
					}(w)
				}
				wg.Wait()
				collectResults(t, writes[0].ResultChan, len(writes), 2*time.Second)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			h := newbatcher(ctrl)
			writes := tc.writes(h)
			tc.setupMocks(h, writes)

			batcher := service.NewAccessBatcher(
				tc.bufferSize,
				tc.batchSize,
				tc.flushInterval,
				h.mockRepo,
				h.mockLua,
				h.logger,
			)
			if tc.startBatcher {
				batcher.Start()
				defer batcher.Stop()
			}

			tc.run(t, batcher, writes, h)
		})
	}
}
