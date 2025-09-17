package doctor

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Status string

const (
	StatusOK      Status = "OK"
	StatusWarning Status = "WARNING"
	StatusError   Status = "ERROR"
)

type Result struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message"`
}

type Check interface {
	Name() string
	Execute(ctx context.Context, client client.Client) []Result
}

type Summary struct {
	OK      int `json:"ok"`
	Warning int `json:"warning"`
	Error   int `json:"error"`
}

type CheckResults struct {
	Checks  []Result `json:"checks"`
	Summary Summary  `json:"summary"`
}
