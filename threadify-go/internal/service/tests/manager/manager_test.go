package service_test

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

func TestServiceManager_Lifecycle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := zap.NewNop()
	m := service.NewServiceManager(logger)

	s1 := enginemocks.NewMockBackgroundService(ctrl)
	s2 := enginemocks.NewMockBackgroundService(ctrl)

	m.Register(s1)
	m.Register(s2)

	gomock.InOrder(
		s1.EXPECT().Start().Return(nil),
		s2.EXPECT().Start().Return(nil),
	)

	err := m.StartAll()
	require.NoError(t, err)

	gomock.InOrder(
		s2.EXPECT().Stop().Return(nil),
		s1.EXPECT().Stop().Return(nil),
	)

	m.StopAll()
}

func TestServiceManager_StartFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := zap.NewNop()
	m := service.NewServiceManager(logger)

	s1 := enginemocks.NewMockBackgroundService(ctrl)
	s2 := enginemocks.NewMockBackgroundService(ctrl)

	m.Register(s1)
	m.Register(s2)

	s1.EXPECT().Start().Return(errors.New("start error"))

	err := m.StartAll()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "start error")
}

func TestServiceManager_StopFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := zap.NewNop()
	m := service.NewServiceManager(logger)

	s1 := enginemocks.NewMockBackgroundService(ctrl)
	m.Register(s1)

	s1.EXPECT().Stop().Return(errors.New("stop error"))

	m.StopAll()
}
