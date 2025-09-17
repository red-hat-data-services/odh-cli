package doctor

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Runner struct {
	client client.Client
}

func NewRunner(c client.Client) *Runner {
	return &Runner{
		client: c,
	}
}

func (r *Runner) RunAllChecks() (*CheckResults, error) {
	ctx := context.Background()
	checks := []Check{
		&basicCheck{},
	}

	results := make([]Result, 0)
	summary := Summary{}

	for _, check := range checks {
		result := check.Execute(ctx, r.client)
		results = append(results, result...)
	}

	for _, result := range results {
		switch result.Status {
		case StatusOK:
			summary.OK++
		case StatusWarning:
			summary.Warning++
		case StatusError:
			summary.Error++
		}
	}

	return &CheckResults{
		Checks:  results,
		Summary: summary,
	}, nil
}

type basicCheck struct{}

func (c *basicCheck) Name() string {
	return "Basic Check"
}

func (c *basicCheck) Execute(_ context.Context, _ client.Client) []Result {
	return []Result{{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Basic check passed",
	}}
}
