package main

import "encoding/json"

// In is any client→server message. Fields are optional; only those for the
// given type are set. See proto/protocol-mvp.md.
type In struct {
	T     string `json:"t"`
	Devid string `json:"devid,omitempty"`
	Name  string `json:"name,omitempty"`
	Plat  string `json:"platform,omitempty"`
	Ver   string `json:"ver,omitempty"`
	Proof string `json:"proof,omitempty"`
	Dx    int    `json:"dx,omitempty"`
	Dy    int    `json:"dy,omitempty"`
	B     string `json:"b,omitempty"`
	Down  *bool  `json:"down,omitempty"`
	S     string `json:"s,omitempty"`
	Code  int    `json:"code,omitempty"`
	Mods  int    `json:"mods,omitempty"`
	Ts    int64  `json:"ts,omitempty"`
}

func encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
