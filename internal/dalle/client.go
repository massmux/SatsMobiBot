package dalle

import (
	"context"
	"github.com/massmux/SatsMobiBot/internal"
	"io"
)

var Enabled bool

func init() {
	if internal.Configuration.Generate.DalleKey != "" {
		Enabled = true
	}
}

type Client interface {
	Generate(ctx context.Context, prompt string) (*Task, error)
	ListTasks(ctx context.Context, req *ListTasksRequest) (*ListTasksResponse, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
	Download(ctx context.Context, generationID string) (io.ReadCloser, error)
	Share(ctx context.Context, generationID string) (string, error)
}
