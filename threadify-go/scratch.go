package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	var msg map[string]json.RawMessage
	json.Unmarshal(body, &msg)
	method := msg["method"]
	fmt.Printf("Method string: %s\n", string(method))
	
	if string(method) == `"initialize"` {
		fmt.Println("Matched initialize!")
		params, hasParams := msg["params"]
		if !hasParams || string(params) == "null" || string(params) == "{}" {
			msg["params"] = json.RawMessage(`{"protocolVersion":"2024-11-05"}`)
			patched, _ := json.Marshal(msg)
			fmt.Printf("Patched: %s\n", string(patched))
		}
	}
}
