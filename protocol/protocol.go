package protocol

import (
	"encoding/json"
)

type Request struct {
	Version string          `json:"version"`
	Cmd     string          `json:"cmd"`
	Payload json.RawMessage `json:"payload"`
}

type RequestDataCmd struct {
	Data string `json:"data"`
}

func (*RequestDataCmd) Command() {}

type ResponseDataCmd struct {
	IsError bool   `json:"is_error"`
	Data    string `json:"data"`
}

func (*ResponseDataCmd) Command() {}

type TerminateCmd struct {
	Code int `json:"code"`
}

func (*TerminateCmd) Command() {}
