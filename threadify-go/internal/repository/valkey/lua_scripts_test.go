package valkey

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/interfaces"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
)

func TestNewLuaScriptManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	valkey := enginemocks.NewMockValkeyClient(ctrl)
	m := NewLuaScriptManager(valkey)

	assert.NotNil(t, m)
	assert.Equal(t, valkey, m.valkeyClient)
	assert.NotNil(t, m.scriptHashes)
}

func TestLuaScriptManager_LoadScripts(t *testing.T) {
	tests := []struct {
		name      string
		mockSetup func(m *enginemocks.MockValkeyClient)
		wantErr   bool
	}{
		{
			name: "success",
			mockSetup: func(m *enginemocks.MockValkeyClient) {
				m.EXPECT().ScriptLoad(gomock.Any(), gomock.Any()).Return("sha", nil).AnyTimes()
			},
			wantErr: false,
		},
		{
			name: "failure",
			mockSetup: func(m *enginemocks.MockValkeyClient) {
				m.EXPECT().ScriptLoad(gomock.Any(), gomock.Any()).Return("", errors.New("load error")).MaxTimes(1)
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			valkey := enginemocks.NewMockValkeyClient(ctrl)
			tc.mockSetup(valkey)

			m := NewLuaScriptManager(valkey)
			err := m.LoadScripts(context.Background())

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				hash, exists := m.GetScriptHash("grant_or_update_access")
				assert.True(t, exists)
				assert.Equal(t, "sha", hash)
			}
		})
	}
}

func TestLuaScriptManager_CheckRateLimit(t *testing.T) {
	tests := []struct {
		name        string
		scriptHash  string
		evalResult  interface{}
		evalErr     error
		wantAllowed bool
		wantErr     bool
	}{
		{
			name:        "allowed",
			scriptHash:  "hash1",
			evalResult:  int64(1),
			wantAllowed: true,
		},
		{
			name:        "limited",
			scriptHash:  "hash1",
			evalResult:  int64(0),
			wantAllowed: false,
		},
		{
			name:       "not loaded",
			scriptHash: "",
			wantErr:    true,
		},
		{
			name:       "eval error",
			scriptHash: "hash1",
			evalErr:    errors.New("eval error"),
			wantErr:    true,
		},
		{
			name:       "invalid result type",
			scriptHash: "hash1",
			evalResult: "string",
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			valkey := enginemocks.NewMockValkeyClient(ctrl)
			m := NewLuaScriptManager(valkey)

			if tc.scriptHash != "" {
				m.scriptHashes["check_company_rate_limit"] = tc.scriptHash
				valkey.EXPECT().EvalSHA(gomock.Any(), tc.scriptHash, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tc.evalResult, tc.evalErr)
			}

			allowed, err := m.CheckCompanyRateLimit(context.Background(), "c1", 100, 60)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantAllowed, allowed)
			}
		})
	}
}

func TestLuaScriptManager_DecrementCreditWithAutoTopup(t *testing.T) {
	params := &interfaces.DebitParams{
		BalanceKey:        "bal",
		ChargedKey:        "char",
		StreamKey:         "stream",
		PendingKey:        "pend",
		BillingCycleStart: time.Now(),
		OccurredAt:        time.Now(),
	}

	tests := []struct {
		name       string
		scriptHash string
		evalResult interface{}
		evalErr    error
		wantErr    bool
	}{
		{
			name:       "success",
			scriptHash: "hash2",
			evalResult: []interface{}{int64(90), int64(1), "spend-id", "topup-id", int64(1)},
			wantErr:    false,
		},
		{
			name:       "script not loaded",
			scriptHash: "",
			wantErr:    true,
		},
		{
			name:       "eval error",
			scriptHash: "hash2",
			evalErr:    errors.New("eval fail"),
			wantErr:    true,
		},
		{
			name:       "invalid shape result",
			scriptHash: "hash2",
			evalResult: []interface{}{int64(90)},
			wantErr:    true,
		},
		{
			name:       "invalid type in result",
			scriptHash: "hash2",
			evalResult: []interface{}{"bad-int", int64(1), "sid", "tid", int64(0)},
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			valkey := enginemocks.NewMockValkeyClient(ctrl)
			m := NewLuaScriptManager(valkey)

			if tc.scriptHash != "" {
				m.scriptHashes["decrement_credit_with_autotopup"] = tc.scriptHash
				valkey.EXPECT().EvalSHA(gomock.Any(), tc.scriptHash, gomock.Any(), gomock.Any()).
					Return(tc.evalResult, tc.evalErr)
			}

			res, err := m.DecrementCreditWithAutoTopup(context.Background(), params)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, int64(90), res.NewBalance)
				assert.Equal(t, int64(1), res.Allowed)
			}
		})
	}
}

func TestLuaScriptManager_GetAndResetCharged(t *testing.T) {
	tests := []struct {
		name       string
		scriptHash string
		evalResult interface{}
		evalErr    error
		wantErr    bool
	}{
		{
			name:       "success",
			scriptHash: "hash3",
			evalResult: []interface{}{int64(100), int64(20)},
		},
		{
			name:       "not loaded",
			scriptHash: "",
			wantErr:    true,
		},
		{
			name:       "eval error",
			scriptHash: "hash3",
			evalErr:    errors.New("eval error"),
			wantErr:    true,
		},
		{
			name:       "invalid type",
			scriptHash: "hash3",
			evalResult: "bad",
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			valkey := enginemocks.NewMockValkeyClient(ctrl)
			m := NewLuaScriptManager(valkey)

			if tc.scriptHash != "" {
				m.scriptHashes["get_and_reset_charged"] = tc.scriptHash
				valkey.EXPECT().EvalSHA(gomock.Any(), tc.scriptHash, []string{"b", "c"}).
					Return(tc.evalResult, tc.evalErr)
			}

			b, c, err := m.GetAndResetCharged(context.Background(), "b", "c")

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, int64(100), b)
				assert.Equal(t, int64(20), c)
			}
		})
	}
}

func TestHelpers(t *testing.T) {
	assert.Equal(t, "1", boolToIntString(true))
	assert.Equal(t, "0", boolToIntString(false))

	v, err := parseLuaInt64(int(10))
	assert.NoError(t, err)
	assert.Equal(t, int64(10), v)

	v, err = parseLuaInt64(float64(20.5))
	assert.NoError(t, err)
	assert.Equal(t, int64(20), v)

	v, err = parseLuaInt64("30")
	assert.NoError(t, err)
	assert.Equal(t, int64(30), v)

	v, err = parseLuaInt64([]byte("40"))
	assert.NoError(t, err)
	assert.Equal(t, int64(40), v)

	_, err = parseLuaInt64(nil)
	assert.Error(t, err)

	s, err := parseLuaString([]byte("bytes"))
	assert.NoError(t, err)
	assert.Equal(t, "bytes", s)

	s, err = parseLuaString(nil)
	assert.NoError(t, err)
	assert.Equal(t, "", s)

	_, err = parseLuaString(123)
	assert.Error(t, err)
}
