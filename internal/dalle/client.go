package dalle

import (
	"context"
	"io"
)

type Client interface {
	Generate(ctx context.Context, prompt string) (*Task, error)
	ListTasks(ctx context.Context, req *ListTasksRequest) (*ListTasksResponse, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
	Download(ctx context.Context, generationID string) (io.ReadCloser, error)
	Share(ctx context.Context, generationID string) (string, error)
}
