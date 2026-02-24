package tests

import "context"

type LuaScriptManager struct {
	scriptHashes map[string]string
}

func NewLuaScriptManager(_ *MockValkeyClient) *LuaScriptManager {
	return &LuaScriptManager{
		scriptHashes: map[string]string{},
	}
}

func (m *LuaScriptManager) LoadScripts(_ context.Context) error {
	m.scriptHashes["update_thread_status"] = "mock-sha-hash"
	return nil
}

func (m *LuaScriptManager) GetScriptHash(name string) (string, bool) {
	hash, exists := m.scriptHashes[name]
	return hash, exists
}

func (m *LuaScriptManager) UpdateThreadStatus(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	_ string,
	_ string,
) (string, error) {
	return "completed", nil
}
