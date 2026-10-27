//go:build !windows

package nerd

import (
	"context"
	"errors"
	"testing"

	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/nerd/nerdfakes"
	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/errdefs"
	"github.com/containerd/errdefs/pkg/errgrpc"
	"google.golang.org/grpc"
)

type createdDeleteService struct {
	tasksapi.TasksClient
	delete func(context.Context, *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error)
}

func (service createdDeleteService) Delete(ctx context.Context, request *tasksapi.DeleteTaskRequest, _ ...grpc.CallOption) (*tasksapi.DeleteResponse, error) {
	return service.delete(ctx, request)
}

func TestDeleteCreatedTask(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		rpcError   error
		closeError error
		wantClosed bool
	}{
		{name: "success", wantClosed: true},
		{name: "already absent", rpcError: errgrpc.ToGRPC(errdefs.ErrNotFound), wantClosed: true},
		{name: "runtime failure", rpcError: errgrpc.ToGRPC(errdefs.ErrFailedPrecondition)},
		{name: "cancelled", rpcError: context.Canceled},
		{name: "close failure", closeError: errors.New("close failed"), wantClosed: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			taskIO := &nerdfakes.FakeIO{}
			taskIO.CloseReturns(testCase.closeError)
			called := false
			service := createdDeleteService{delete: func(ctx context.Context, request *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
				called = true
				if request.ContainerID != "created-task" || taskIO.CancelCallCount() != 0 || taskIO.CloseCallCount() != 0 {
					t.Fatal("Delete must precede IO cancellation and target exact task")
				}
				return &tasksapi.DeleteResponse{}, testCase.rpcError
			}}
			err := deleteCreatedTask(context.Background(), service, "created-task", taskIO)
			if !called {
				t.Fatal("public Delete was not called")
			}
			wantError := testCase.closeError != nil || (testCase.rpcError != nil && !testCase.wantClosed)
			if (err != nil) != wantError {
				t.Fatalf("error = %v, want error %v", err, wantError)
			}
			if (taskIO.CancelCallCount() == 1 && taskIO.WaitCallCount() == 1 && taskIO.CloseCallCount() == 1) != testCase.wantClosed {
				t.Fatalf("unexpected IO lifecycle cancel=%d wait=%d close=%d", taskIO.CancelCallCount(), taskIO.WaitCallCount(), taskIO.CloseCallCount())
			}
		})
	}
}

func TestDeleteCreatedTaskLoadedIO(t *testing.T) {
	deleted := false
	closed := false
	taskIO, err := cio.Load(cio.NewFIFOSet(cio.Config{}, func() error {
		if !deleted {
			t.Fatal("loaded IO closed before public task deletion")
		}
		closed = true
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	service := createdDeleteService{delete: func(context.Context, *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
		deleted = true
		return &tasksapi.DeleteResponse{}, nil
	}}
	if err := deleteCreatedTask(t.Context(), service, "created-task", taskIO); err != nil {
		t.Fatal(err)
	}
	if !deleted || !closed {
		t.Fatalf("loaded IO cleanup incomplete deleted=%t closed=%t", deleted, closed)
	}
}
