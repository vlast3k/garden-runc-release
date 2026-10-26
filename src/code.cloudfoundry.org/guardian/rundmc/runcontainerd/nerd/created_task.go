//go:build !windows

package nerd

import (
	"context"

	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/errdefs"
	"github.com/containerd/errdefs/pkg/errgrpc"
)

func deleteCreatedTask(ctx context.Context, service tasksapi.TasksClient, containerID string, taskIO cio.IO) error {
	_, err := service.Delete(ctx, &tasksapi.DeleteTaskRequest{ContainerID: containerID})
	if err != nil && !errdefs.IsNotFound(errgrpc.ToNative(err)) {
		return errgrpc.ToNative(err)
	}
	if taskIO != nil {
		taskIO.Cancel()
		taskIO.Wait()
		return taskIO.Close()
	}
	return nil
}
