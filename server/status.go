package main

import "encoding/json"

// encodeClients serializes a client snapshot for the status IPC pipe. Only the
// exported metadata (Name/Addr/Since) is emitted; the password is never part of
// this payload. Order is preserved so the UI shows the service's sort order.
func encodeClients(cs []Client) []byte {
	if cs == nil {
		cs = []Client{}
	}
	b, err := json.Marshal(cs)
	if err != nil {
		return []byte("[]")
	}
	return b
}

// decodeClients parses the payload produced by encodeClients.
func decodeClients(b []byte) ([]Client, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var out []Client
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
