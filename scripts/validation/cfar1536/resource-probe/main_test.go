package main

import "testing"

func TestMemoryExitRequiresBackendAndIndependentOOMCounter(t *testing.T) {
	for _, testCase := range []struct {
		name, backend, stdout, stderr string
		status                        int
		want                          bool
	}{
		{name: "stock SIGKILL", backend: "stock", status: 137, want: true},
		{name: "runsc SIGKILL", backend: "runsc", status: 137, want: true},
		{name: "runsc wait failure", backend: "runsc", status: 128, stderr: "containerManager.WaitPID failed: EOF", want: true},
		{name: "stock generic error rejected", backend: "stock", status: 128, stderr: "containerManager.WaitPID failed: EOF"},
		{name: "unrelated runsc error rejected", backend: "runsc", status: 128, stderr: "other failure"},
		{name: "successful allocation rejected", backend: "runsc", status: 0, stdout: "UNEXPECTED_MEMORY_ALLOCATION_SUCCESS"},
		{name: "output on killed worker rejected", backend: "runsc", status: 137, stdout: "UNEXPECTED_MEMORY_ALLOCATION_SUCCESS"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := validMemoryExit(testCase.backend, testCase.status, testCase.stdout, testCase.stderr); got != testCase.want {
				t.Fatalf("got %v want %v", got, testCase.want)
			}
		})
	}
	before := map[string]uint64{"owned": 0}
	if !oomIncreased(before, map[string]uint64{"owned": 1}) {
		t.Fatal("owned increase rejected")
	}
	if oomIncreased(before, map[string]uint64{"owned": 0, "unrelated": 5}) {
		t.Fatal("unrelated OOM accepted")
	}
	if oomIncreased(before, map[string]uint64{}) {
		t.Fatal("missing cgroup accepted")
	}
}
