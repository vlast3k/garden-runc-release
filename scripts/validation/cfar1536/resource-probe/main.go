package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/garden"
	gardenClient "code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"
)

func main() {
	handle := flag.String("handle", "", "Unique disposable handle")
	backend := flag.String("backend", "stock", "Expected stock or runsc backend")
	flag.Parse()
	if *handle == "" || (*backend != "stock" && *backend != "runsc") {
		panic("unique handle and valid backend required")
	}
	if err := exercise(*handle, *backend); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

func exercise(handle, backend string) (result error) {
	client := gardenClient.New(connection.New("unix", "/var/vcap/data/garden/garden.sock"))
	before, err := client.Containers(nil)
	if err != nil {
		return err
	}
	baseline := make(map[string]bool)
	for _, container := range before {
		baseline[container.Handle()] = true
	}
	if baseline[handle] {
		return errors.New("refusing existing handle")
	}
	fmt.Printf("contract handle=%s memory=384MiB disk_exclusive=128MiB positive_memory=64MiB disk=16MiB cpu=5s negative_memory=768MiB disk=256MiB\n", handle)
	container, err := client.Create(garden.ContainerSpec{
		Handle: handle, RootFSPath: "/var/vcap/packages/cflinuxfs4/rootfs.tar", GraceTime: 5 * time.Minute,
		Limits: garden.Limits{
			Memory: garden.MemoryLimits{LimitInBytes: 384 << 20},
			Disk:   garden.DiskLimits{ByteHard: 128 << 20, Scope: garden.DiskLimitScopeExclusive},
			CPU:    garden.CPULimits{LimitInShares: 256},
		},
	})
	if err != nil {
		return err
	}
	defer func() {
		destroyErr := client.Destroy(handle)
		after, inventoryErr := client.Containers(nil)
		if inventoryErr == nil {
			if len(after) != len(baseline) {
				inventoryErr = errors.New("inventory count changed")
			}
			for _, current := range after {
				if !baseline[current.Handle()] {
					inventoryErr = errors.New("unexpected surviving handle")
				}
			}
		}
		result = errors.Join(result, destroyErr, inventoryErr)
		fmt.Printf("cleanup_destroy=%v inventory=%v\n", destroyErr, inventoryErr)
	}()
	initial, err := container.Metrics()
	if err != nil {
		return err
	}
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	defer inputReader.Close()
	defer inputWriter.Close()
	defer outputReader.Close()
	defer outputWriter.Close()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(outputReader)
		if scanner.Scan() {
			ready <- scanner.Text()
		}
	}()
	var positiveStderr bytes.Buffer
	worker, err := container.Run(garden.ProcessSpec{Path: "/bin/python3", User: "root", Args: []string{"-c", `import sys,time
memory=bytearray(64*1024*1024)
with open('/tmp/cfar1536-pressure','wb') as output:
    output.write(b'x'*(16*1024*1024))
    output.flush()
deadline=time.monotonic()+5
while time.monotonic()<deadline:
    pass
print('RESOURCE_READY',flush=True)
sys.stdin.readline()
`}}, garden.ProcessIO{Stdin: inputReader, Stdout: outputWriter, Stderr: &positiveStderr})
	if err != nil {
		return err
	}
	select {
	case message := <-ready:
		if message != "RESOURCE_READY" {
			return fmt.Errorf("unexpected readiness %q", message)
		}
	case <-time.After(30 * time.Second):
		return errors.New("positive readiness deadline")
	}
	loaded, err := container.Metrics()
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(map[string]any{"initial": initial, "loaded": loaded})
	fmt.Printf("metrics=%s\n", encoded)
	if loaded.MemoryStat.TotalUsageTowardLimit < initial.MemoryStat.TotalUsageTowardLimit+(32<<20) {
		return errors.New("memory accounting failed to reflect held allocation")
	}
	if loaded.CPUStat.Usage < initial.CPUStat.Usage+1_000_000_000 {
		return errors.New("CPU accounting failed to reflect five-second busy loop")
	}
	if loaded.DiskStat.ExclusiveBytesUsed < initial.DiskStat.ExclusiveBytesUsed+(8<<20) {
		return errors.New("disk accounting failed to reflect sixteen-MiB write")
	}
	if _, err := io.WriteString(inputWriter, "release\n"); err != nil {
		return err
	}
	status, err := worker.Wait()
	if err != nil || status != 0 || positiveStderr.Len() != 0 {
		return fmt.Errorf("positive worker status=%d error=%v stderr=%q", status, err, positiveStderr.String())
	}
	status, stdout, stderr, err := python(container, `import errno,os
os.unlink('/tmp/cfar1536-pressure')
written=0
try:
    with open('/tmp/cfar1536-disk-negative','wb',buffering=0) as output:
        for block in range(256):
            written+=output.write(b'x'*(1024*1024))
except OSError as error:
    assert error.errno in (errno.ENOSPC,errno.EDQUOT),error
    print('DISK_ENFORCED errno=%d bytes=%d'%(error.errno,written),flush=True)
else:
    raise AssertionError('256MiB write bypassed 128MiB disk quota')
finally:
    os.unlink('/tmp/cfar1536-disk-negative')
`)
	fmt.Printf("disk_negative_exit=%d stdout=%q stderr=%q error=%v\n", status, stdout, stderr, err)
	if err != nil || status != 0 || !strings.HasPrefix(stdout, "DISK_ENFORCED ") {
		return errors.New("disk quota enforcement failed")
	}
	beforeOOM, err := oomCounters(handle)
	if err != nil || len(beforeOOM) == 0 {
		return fmt.Errorf("missing independent cgroup evidence: counters=%v error=%v", beforeOOM, err)
	}
	status, stdout, stderr, err = python(container, `memory=bytearray(768*1024*1024)
print('UNEXPECTED_MEMORY_ALLOCATION_SUCCESS',flush=True)
`)
	fmt.Printf("memory_negative_exit=%d stdout=%q stderr=%q error=%v\n", status, stdout, stderr, err)
	if err != nil || !validMemoryExit(backend, status, stdout, stderr) {
		return errors.New("memory pressure did not produce backend-specific exit contract")
	}
	afterOOM, err := oomCounters(handle)
	fmt.Printf("cgroup_oom_before=%v after=%v error=%v\n", beforeOOM, afterOOM, err)
	if err != nil || !oomIncreased(beforeOOM, afterOOM) {
		return errors.New("memory failure lacks independent cgroup oom_kill increase")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		info, err := container.Info()
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(strings.Join(info.Events, " ")), "out of memory") {
			fmt.Printf("memory_events=%q\n", info.Events)
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("confirmed cgroup OOM lacks Garden event: %q", info.Events)
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("PASS: actual memory/disk enforcement and memory/disk/CPU accounting")
	return nil
}

func python(container garden.Container, script string) (int, string, string, error) {
	var stdout, stderr bytes.Buffer
	process, err := container.Run(garden.ProcessSpec{Path: "/bin/python3", User: "root", Args: []string{"-c", script}}, garden.ProcessIO{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		return -1, "", "", err
	}
	status, err := process.Wait()
	return status, stdout.String(), stderr.String(), err
}

func validMemoryExit(backend string, status int, stdout, stderr string) bool {
	if stdout != "" {
		return false
	}
	if status == 137 {
		return true
	}
	return backend == "runsc" && status == 128 && strings.Contains(stderr, "containerManager.WaitPID") && strings.Contains(stderr, "EOF")
}

func oomIncreased(before, after map[string]uint64) bool {
	for path, count := range before {
		if current, exists := after[path]; exists && current > count {
			return true
		}
	}
	return false
}

func oomCounters(handle string) (map[string]uint64, error) {
	counters := make(map[string]uint64)
	err := filepath.WalkDir("/sys/fs/cgroup", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() != "memory.events" || !strings.Contains(path, handle) {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(contents), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == "oom_kill" {
				count, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					return err
				}
				counters[path] = count
			}
		}
		return nil
	})
	return counters, err
}
