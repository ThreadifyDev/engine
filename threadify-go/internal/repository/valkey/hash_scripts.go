package valkey

import (
	"embed"
	"fmt"
)

//go:embed lua/get_thread_hash.lua lua/update_thread_hash.lua
var hashScripts embed.FS

func HashScript(name string) (string, error) {
	if name != "get_thread_hash.lua" && name != "update_thread_hash.lua" {
		return "", fmt.Errorf("unknown hash script %q", name)
	}
	data, err := hashScripts.ReadFile("lua/" + name)
	return string(data), err
}
