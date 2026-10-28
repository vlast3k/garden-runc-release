package nerd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	apievents "github.com/containerd/containerd/api/events"
	containersapi "github.com/containerd/containerd/api/services/containers/v1"
	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	runcoptions "github.com/containerd/containerd/api/types/runc/options"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/typeurl/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	cgrouputils "github.com/opencontainers/cgroups"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/emptypb"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client/connection"
	"code.cloudfoundry.org/garden/gardenfakes"
	"code.cloudfoundry.org/garden/server"
	"code.cloudfoundry.org/guardian/gqt/containerdrunner"
	"code.cloudfoundry.org/guardian/metrics"
	"code.cloudfoundry.org/guardian/rundmc"
	"code.cloudfoundry.org/guardian/rundmc/processes"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/nerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/runcontainerdfakes"
	"code.cloudfoundry.org/guardian/rundmc/rundmcfakes"
	"code.cloudfoundry.org/guardian/rundmc/users"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
)

type policyContainersServer struct {
	containersapi.UnimplementedContainersServer
	created chan *containersapi.Container
	stored  *containersapi.Container
}

func (rpc *policyContainersServer) Create(_ context.Context, request *containersapi.CreateContainerRequest) (*containersapi.CreateContainerResponse, error) {
	rpc.stored = request.Container
	rpc.created <- request.Container
	return &containersapi.CreateContainerResponse{Container: request.Container}, nil
}

func (rpc *policyContainersServer) Get(context.Context, *containersapi.GetContainerRequest) (*containersapi.GetContainerResponse, error) {
	return &containersapi.GetContainerResponse{Container: rpc.stored}, nil
}

func (rpc *policyContainersServer) Update(_ context.Context, request *containersapi.UpdateContainerRequest) (*containersapi.UpdateContainerResponse, error) {
	rpc.stored = request.Container
	return &containersapi.UpdateContainerResponse{Container: request.Container}, nil
}

type policyTasksServer struct {
	tasksapi.UnimplementedTasksServer
	created chan *tasksapi.CreateTaskRequest
	killed  chan string
	deleted chan string
}

func (rpc *policyTasksServer) Delete(_ context.Context, request *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
	rpc.deleted <- request.ContainerID
	return nil, status.Error(codes.PermissionDenied, "policy fixture stops before public task deletion")
}

func (rpc *policyTasksServer) Create(_ context.Context, request *tasksapi.CreateTaskRequest) (*tasksapi.CreateTaskResponse, error) {
	rpc.created <- request
	return nil, status.Error(codes.PermissionDenied, "policy fixture stops before runtime creation")
}

func (rpc *policyTasksServer) Get(_ context.Context, request *tasksapi.GetRequest) (*tasksapi.GetResponse, error) {
	return &tasksapi.GetResponse{Process: &tasktypes.Process{
		ContainerID: request.ContainerID, ID: request.ContainerID, Pid: ^uint32(0), Status: tasktypes.Status_CREATED,
	}}, nil
}

func (rpc *policyTasksServer) Wait(_ context.Context, request *tasksapi.WaitRequest) (*tasksapi.WaitResponse, error) {
	return nil, status.Error(codes.PermissionDenied, "policy fixture stops before task signaling")
}

func (rpc *policyTasksServer) Kill(_ context.Context, request *tasksapi.KillRequest) (*emptypb.Empty, error) {
	rpc.killed <- request.ContainerID
	return nil, status.Error(codes.PermissionDenied, "policy fixture stops before task signaling")
}

func TestNerdRuntimePolicyCreatePayload(t *testing.T) {
	for _, test := range []struct {
		runtimeType       string
		wantRuntime       string
		wantWindows       bool
		wantOptions       bool
		annotations       map[string]string
		wantContainerType string
		wantSandboxID     string
	}{
		{wantRuntime: "io.containerd.runc.v2", wantWindows: true, wantOptions: true},
		{runtimeType: "io.containerd.runc.v2", wantRuntime: "io.containerd.runc.v2", wantWindows: true, wantOptions: true},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", wantContainerType: "sandbox"},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", annotations: map[string]string{"sentinel": "retained"}, wantContainerType: "sandbox"},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", annotations: map[string]string{"io.kubernetes.cri.container-type": "container"}, wantContainerType: "container"},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", annotations: map[string]string{"container-type": "pea", "sandbox-container": "garden-parent", "sentinel": "retained"}, wantContainerType: "container", wantSandboxID: "garden-parent"},
	} {
		t.Run(test.runtimeType, func(t *testing.T) {
			socketDir, err := os.MkdirTemp("", "nerd-policy-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(socketDir) })
			socket := filepath.Join(socketDir, "containerd.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			containerRPC := &policyContainersServer{created: make(chan *containersapi.Container, 1)}
			taskRPC := &policyTasksServer{created: make(chan *tasksapi.CreateTaskRequest, 1), killed: make(chan string, 1), deleted: make(chan string, 1)}
			rpcServer := grpc.NewServer()
			containersapi.RegisterContainersServer(rpcServer, containerRPC)
			tasksapi.RegisterTasksServer(rpcServer, taskRPC)
			serveResult := make(chan error, 1)
			go func() { serveResult <- rpcServer.Serve(listener) }()
			t.Cleanup(func() {
				rpcServer.Stop()
				if err := <-serveResult; err != nil && err != grpc.ErrServerStopped {
					t.Error(err)
				}
			})
			containerdClient, err := client.New(socket, client.WithDefaultRuntime(test.wantRuntime))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { containerdClient.Close() })
			policy, err := runcontainerd.ResolveRuntimePolicy(test.runtimeType)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), "garden"), 5*time.Second)
			defer cancel()
			ctx = leases.WithLease(ctx, "lease-is-off")
			policyLog := lager.NewLogger("nerd-policy")
			manager := nerd.New(containerdClient, ctx, t.TempDir(), metrics.NewMetricsProvider(policyLog, t.TempDir()), policy)
			wantSentinel := test.annotations["sentinel"]
			spec := &specs.Spec{Version: specs.Version, Windows: &specs.Windows{}, Process: &specs.Process{Args: []string{"/policy-sentinel"}, Cwd: "/"}, Annotations: test.annotations}
			err = manager.Create(lager.NewLogger("nerd-policy"), "policy-container", spec, 4242, 4343, func() (io.Reader, io.Writer, io.Writer) { return nil, nil, nil })
			if err == nil || !strings.Contains(err.Error(), "policy fixture stops before runtime creation") {
				t.Fatalf("Create error = %v, want bounded fixture rejection", err)
			}
			// The fixture deliberately exposes a task after failed Create. Owned
			// rollback must reach its runtime-specific delete path before returning.
			rollback := taskRPC.deleted
			if test.wantOptions {
				rollback = taskRPC.killed
			}
			select {
			case id := <-rollback:
				if id != "policy-container" {
					t.Errorf("rollback ID = %q", id)
				}
			default:
				t.Fatal("failed Create did not attempt owned task rollback")
			}
			select {
			case request := <-containerRPC.created:
				if request.ID != "policy-container" || request.Runtime.Name != test.wantRuntime {
					t.Errorf("container identity/runtime = %+v", request)
				}
				if !test.wantOptions {
					options := request.Runtime.Options
					if options == nil || options.TypeUrl != "runtimeoptions.v1.Options" {
						t.Fatalf("runsc runtime options = %v, want runtimeoptions.v1.Options", options)
					}
					field, wireType, tagSize := protowire.ConsumeTag(options.Value)
					if field != 2 || wireType != protowire.BytesType || tagSize < 0 {
						t.Fatalf("runsc config_path tag = %d/%d/%d, want field 2 string", field, wireType, tagSize)
					}
					configPath, pathSize := protowire.ConsumeString(options.Value[tagSize:])
					if configPath != "/etc/containerd/runsc/config.toml" || pathSize < 0 || tagSize+pathSize != len(options.Value) {
						t.Fatalf("runsc config_path payload = %q (%d bytes), want exact configured path only", configPath, pathSize)
					}
				}
				var emitted specs.Spec
				if err := json.Unmarshal(request.Spec.Value, &emitted); err != nil {
					t.Fatal(err)
				}
				if (emitted.Windows != nil) != test.wantWindows || (spec.Windows != nil) != test.wantWindows {
					t.Errorf("Windows section: emitted=%v input=%v, want presence %v", emitted.Windows, spec.Windows, test.wantWindows)
				}
				if len(emitted.Process.Args) != 1 || emitted.Process.Args[0] != "/policy-sentinel" {
					t.Errorf("unrelated process arguments changed: %v", emitted.Process.Args)
				}
				if emitted.Annotations["io.kubernetes.cri.container-type"] != test.wantContainerType {
					t.Errorf("container type = %q, want %q", emitted.Annotations["io.kubernetes.cri.container-type"], test.wantContainerType)
				}
				if emitted.Annotations["io.kubernetes.cri.sandbox-id"] != test.wantSandboxID {
					t.Errorf("sandbox ID = %q, want %q", emitted.Annotations["io.kubernetes.cri.sandbox-id"], test.wantSandboxID)
				}
				if emitted.Annotations["sentinel"] != wantSentinel {
					t.Error("unrelated annotation lost")
				}
			default:
				t.Fatal("no container creation payload")
			}
			select {
			case request := <-taskRPC.created:
				if request.ContainerID != "policy-container" {
					t.Errorf("task container ID = %q", request.ContainerID)
				}
				if test.wantOptions {
					if request.Options == nil {
						t.Fatal("stock task options missing")
					}
					decoded, err := typeurl.UnmarshalAny(request.Options)
					if err != nil {
						t.Fatal(err)
					}
					options, ok := decoded.(*runcoptions.Options)
					if !ok || !options.NoNewKeyring || options.IoUid != 4242 || options.IoGid != 4343 {
						t.Errorf("stock options = %+v, want NoNewKeyring and UID/GID 4242/4343", decoded)
					}
				} else if request.Options != nil {
					t.Errorf("runsc inherited task options: %v", request.Options)
				}
			default:
				t.Fatal("no task creation payload")
			}
			err = manager.Delete(lager.NewLogger("nerd-policy-delete"), "policy-container")
			if test.wantOptions {
				if err == nil || !strings.Contains(err.Error(), "policy fixture stops before task signaling") {
					t.Errorf("stock Delete error = %v, want ordinary task cleanup", err)
				}
				select {
				case id := <-taskRPC.killed:
					if id != "policy-container" {
						t.Errorf("Kill container = %q", id)
					}
				case <-ctx.Done():
					t.Error("stock Delete did not reach ordinary task cleanup before the test deadline")
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "policy fixture stops before public task deletion") {
					t.Errorf("runsc Delete error = %v, want public TaskService.Delete rejection", err)
				}
				select {
				case id := <-taskRPC.deleted:
					if id != "policy-container" {
						t.Errorf("Delete container = %q", id)
					}
				case <-ctx.Done():
					t.Error("runsc Delete did not reach public task deletion before the test deadline")
				}
				select {
				case id := <-taskRPC.killed:
					t.Errorf("runsc CREATED-task path unexpectedly reached stock Kill for %q", id)
				default:
				}
			}
		})
	}
}

var randomGenerator *rand.Rand
var _ = Describe("Nerd", func() {
	var (
		testLogger    lager.Logger
		cnerd         *nerd.Nerd
		containerID   string
		processID     string
		fifoDir       string
		initProcessIO func() (io.Reader, io.Writer, io.Writer)
		processIO     func() (io.Reader, io.Writer, io.Writer, bool)
		stdin         io.Reader
		stdout        io.Writer
		stderr        io.Writer
		spec          *specs.Spec
		mp            *metrics.MetricsProvider
		rootfsMounted bool
	)

	const (
		maximusUID = 5000
		maximusGID = 5000
	)

	BeforeEach(func() {
		rootfsMounted = false
		randomGenerator = rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID = fmt.Sprintf("test-container-%s", randomString(10))
		processID = fmt.Sprintf("test-process-%s", randomString(10))
		stdout = gbytes.NewBuffer()
		stderr = gbytes.NewBuffer()
		stdin = nil
		initProcessIO = func() (io.Reader, io.Writer, io.Writer) {
			return stdin, stdout, stderr
		}
		processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
			return stdin, stdout, stderr, false
		}
		testLogger = lagertest.NewTestLogger("nerd-test")

		mp = metrics.NewMetricsProvider(testLogger, fifoDir)

		var err error
		fifoDir, err = os.MkdirTemp("", "nerd-fifo")
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		cnerd = nerd.New(containerdClient, containerdContext, fifoDir, mp, testRuntimePolicy)
	})

	AfterEach(func() {
		Expect(os.RemoveAll(fifoDir)).To(Succeed())
		if spec != nil && spec.Root != nil {
			if rootfsMounted {
				Expect(syscall.Unmount(spec.Root.Path, syscall.MNT_DETACH)).To(Succeed())
			}
			Expect(os.RemoveAll(filepath.Dir(spec.Root.Path))).To(Succeed())
		}
	})

	Describe("exec archive prerequisites and round trip", func() {
		AfterEach(func() {
			Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())
		})

		It("uses rootfs mkdir and tar, preserves content and cleans up execs", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			runtime := new(rundmcfakes.FakeOCIRuntime)
			sequence := 0
			var execIDs []string
			runtime.ExecStub = func(log lager.Logger, handle string, processSpec garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
				sequence++
				id := fmt.Sprintf("archive-%d", sequence)
				execIDs = append(execIDs, id)
				resolvedUser, err := users.LookupUser(spec.Root.Path, processSpec.User)
				if err != nil {
					return nil, err
				}
				backingProcess, err := cnerd.Exec(log, handle, id, &specs.Process{
					Args: append([]string{processSpec.Path}, processSpec.Args...),
					Cwd:  "/",
					User: specs.User{UID: uint32(resolvedUser.Uid), GID: uint32(resolvedUser.Gid)},
				}, func() (io.Reader, io.Writer, io.Writer, bool) {
					return processIO.Stdin, processIO.Stdout, processIO.Stderr, false
				})
				if err != nil {
					return nil, err
				}
				return runcontainerd.NewProcess(log, backingProcess, true), nil
			}
			streamer := rundmc.NewExecStreamer(runtime)
			payload := bytes.Repeat([]byte("runtime checksum\x00"), 8192)
			var archive bytes.Buffer
			writer := tar.NewWriter(&archive)
			for _, name := range []string{"file with spaces", "--checkpoint=1", "nested/child"} {
				Expect(writer.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(payload))})).To(Succeed())
				_, err := writer.Write(payload)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(writer.Close()).To(Succeed())
			Expect(streamer.StreamIn(testLogger, containerID, garden.StreamInSpec{Path: "/archive dir", TarStream: bytes.NewReader(archive.Bytes())})).To(Succeed())
			reader, err := streamer.StreamOut(testLogger, containerID, garden.StreamOutSpec{Path: "/archive dir/"})
			Expect(err).NotTo(HaveOccurred())
			data, err := io.ReadAll(reader)
			Expect(reader.Close()).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
			archiveReader := tar.NewReader(bytes.NewReader(data))
			files := 0
			for {
				header, err := archiveReader.Next()
				if err == io.EOF {
					break
				}
				Expect(err).NotTo(HaveOccurred())
				if header.Typeflag != tar.TypeReg {
					continue
				}
				contents, err := io.ReadAll(archiveReader)
				Expect(err).NotTo(HaveOccurred())
				Expect(contents).To(Equal(payload))
				files++
			}
			Expect(files).To(Equal(3))
			for _, userName := range []string{"root", "65534", "nobody"} {
				resolvedUser, err := users.LookupUser(spec.Root.Path, userName)
				Expect(err).NotTo(HaveOccurred())
				path := "/tmp/archive-" + userName
				Expect(streamer.StreamIn(testLogger, containerID, garden.StreamInSpec{Path: path, User: userName, TarStream: bytes.NewReader(archive.Bytes())})).To(Succeed())
				reader, err := streamer.StreamOut(testLogger, containerID, garden.StreamOutSpec{Path: path + "/", User: userName})
				Expect(err).NotTo(HaveOccurred())
				data, err := io.ReadAll(reader)
				Expect(reader.Close()).To(Succeed())
				Expect(err).NotTo(HaveOccurred())
				archiveReader := tar.NewReader(bytes.NewReader(data))
				files := 0
				for {
					header, err := archiveReader.Next()
					if err == io.EOF {
						break
					}
					Expect(err).NotTo(HaveOccurred())
					if header.Typeflag != tar.TypeReg {
						continue
					}
					Expect(header.Uid).To(Equal(resolvedUser.Uid))
					contents, err := io.ReadAll(archiveReader)
					Expect(err).NotTo(HaveOccurred())
					Expect(contents).To(Equal(payload))
					files++
				}
				Expect(files).To(Equal(3))
			}
			reader, err = streamer.StreamOut(testLogger, containerID, garden.StreamOutSpec{Path: "/archive dir/"})
			Expect(err).NotTo(HaveOccurred())
			Expect(reader.Close()).To(Succeed())
			for _, id := range execIDs {
				Eventually(func() error {
					_, err := cnerd.GetProcess(testLogger, containerID, id)
					return err
				}, "10s").Should(MatchError(runcontainerd.ProcessNotFoundError{Handle: containerID, ID: id}))
			}
			_, state, err := cnerd.State(testLogger, containerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(string(client.Running)))
		})
	})

	Describe("validated pea exec contract", func() {
		const (
			assetDestination       = "/etc/cf-assets/healthcheck"
			envoyDestination       = "/etc/cf-assets/envoy"
			envoyConfigDestination = "/etc/cf-assets/envoy_config"
			packagedImage          = "/var/vcap/packages/cflinuxfs4/rootfs.tar"
			envoyWrapper           = "trap 'kill -9 0' TERM; /etc/cf-assets/envoy/envoy -c /etc/cf-assets/envoy_config/envoy.yaml --drain-time-s 0 --log-level critical& pid=$!; wait $pid"
		)
		var (
			runtime           *rundmcfakes.FakeOCIRuntime
			peaCreator        *rundmcfakes.FakePeaCreator
			peaUserResolver   *rundmcfakes.FakePeaUsernameResolver
			gardenServer      *server.GardenServer
			gardenClient      connection.Connection
			served            chan error
			parentCreated     bool
			parentPID         int
			assetSource       string
			imageRootfs       string
			newMountSource    string
			parentUser        string
			imageUser         string
			sentinel          string
			configSentinel    string
			parentAssetMounts []specs.Mount
			request           garden.ProcessSpec
			envoyRequest      garden.ProcessSpec
			requestIDs        []string
			failedIDs         []string
			runRequest        func(garden.ProcessSpec, garden.ProcessIO) (garden.Process, error)
			expectRemoved     func(string)
			expectHealthy     func()
		)

		BeforeEach(func() {
			parentCreated = false
			gardenServer = nil
			requestIDs = nil
			failedIDs = nil
			parentUser = "parent" + randomString(10)
			imageUser = "image" + randomString(10)
			sentinel = "pea-contract-" + randomString(20)
			fixtureDir, err := os.MkdirTemp("", "validated-pea-assets")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(os.RemoveAll(fixtureDir)).To(Succeed()) })
			assetSource = filepath.Join(fixtureDir, "healthcheck")
			imageRootfs = filepath.Join(fixtureDir, "candidate-image")
			newMountSource = filepath.Join(fixtureDir, "new-mount")
			envoySource := filepath.Join(fixtureDir, "envoy")
			envoyConfigSource := filepath.Join(fixtureDir, "envoy-config")
			configSentinel = "envoy-config-" + sentinel
			for _, directory := range []string{assetSource, filepath.Join(imageRootfs, "bin"), filepath.Join(imageRootfs, "etc"), newMountSource, envoySource, envoyConfigSource} {
				Expect(os.MkdirAll(directory, 0755)).To(Succeed())
			}
			assetScript := "#!/bin/sh\nset -eu\n" +
				"printf 'asset:%s\\n' \"$(/bin/cat /etc/cf-assets/healthcheck/sentinel)\"\n" +
				"printf 'uid:%s\\n' \"$(/usr/bin/id -u)\"\n" +
				"if [ \"${1-}\" = hold ]; then printf 'ready\\n'; exec /bin/sleep 999999; fi\n" +
				"IFS= read -r input\n" +
				"printf 'stdin:%s\\n' \"$input\"\n" +
				"printf 'env:%s\\n' \"$PEA_CONTRACT\"\n" +
				"printf 'cwd:%s\\n' \"$(pwd)\"\n" +
				"printf 'stderr:%s\\n' \"$input\" >&2\nexit 17\n"
			envoyScript := "#!/bin/sh\nset -eu\n" +
				"[ \"$#\" -eq 6 ]\n" +
				"[ \"$1\" = -c ]\n" +
				"[ \"$2\" = /etc/cf-assets/envoy_config/envoy.yaml ]\n" +
				"[ \"$3\" = --drain-time-s ]\n" +
				"[ \"$4\" = 0 ]\n" +
				"[ \"$5\" = --log-level ]\n" +
				"[ \"$6\" = critical ]\n" +
				"[ \"$(/bin/cat \"$2\")\" = \"contract: $PEA_ENVOY_CONFIG\" ]\n" +
				"printf 'envoy:%s\\nconfig:%s\\nuid:%s\\n' \"$PEA_CONTRACT\" \"$PEA_ENVOY_CONFIG\" \"$(/usr/bin/id -u)\"\n" +
				"printf 'envoy-stderr:%s\\n' \"$PEA_ENVOY_CONFIG\" >&2\nexit 23\n"
			imageScript := "#!/bin/sh\nprintf 'image-only:" + sentinel + "\\n'\n"
			Expect(os.WriteFile(filepath.Join(assetSource, "healthcheck"), []byte(assetScript), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(assetSource, "sentinel"), []byte(sentinel), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(envoySource, "envoy"), []byte(envoyScript), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(envoyConfigSource, "envoy.yaml"), []byte("contract: "+configSentinel+"\n"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(imageRootfs, "bin", "image-only"), []byte(imageScript), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(imageRootfs, "etc", "passwd"), []byte(imageUser+":x:5252:5252:Image only:/:/bin/sh\n"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(imageRootfs, "etc", "group"), []byte("image:x:5252:\n"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(newMountSource, "sentinel"), []byte("mount-only:"+sentinel), 0644)).To(Succeed())
			contents, err := os.ReadFile(filepath.Join(assetSource, "healthcheck"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(Equal(assetScript))
			contents, err = os.ReadFile(filepath.Join(imageRootfs, "bin", "image-only"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(Equal(imageScript))
			contents, err = os.ReadFile(filepath.Join(newMountSource, "sentinel"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(Equal("mount-only:" + sentinel))
			request = garden.ProcessSpec{
				ID: containerID + "-asset", Path: assetDestination + "/healthcheck", Dir: "/", User: parentUser,
				Env: []string{"PEA_CONTRACT=" + sentinel}, Image: garden.ImageRef{URI: "preloaded:cflinuxfs4"},
				BindMounts: []garden.BindMount{{SrcPath: assetSource, DstPath: assetDestination, Origin: garden.BindMountOriginHost, Mode: garden.BindMountModeRO}},
			}
			parentAssetMounts = []specs.Mount{
				{Type: "bind", Source: envoySource, Destination: envoyDestination, Options: []string{"rbind", "ro"}},
				{Type: "bind", Source: envoyConfigSource, Destination: envoyConfigDestination, Options: []string{"rbind", "ro"}},
			}
			envoyRequest = garden.ProcessSpec{
				ID: containerID + "-envoy", Path: "sh", Args: []string{"-c", envoyWrapper}, Dir: "/", User: parentUser,
				Env:   []string{"PEA_CONTRACT=" + sentinel, "PEA_ENVOY_CONFIG=" + configSentinel},
				Image: garden.ImageRef{URI: packagedImage}, OverrideContainerLimits: &garden.ProcessLimits{},
				BindMounts: []garden.BindMount{
					{SrcPath: envoySource, DstPath: envoyDestination, Origin: garden.BindMountOriginHost, Mode: garden.BindMountModeRO},
					{SrcPath: envoyConfigSource, DstPath: envoyConfigDestination, Origin: garden.BindMountOriginHost, Mode: garden.BindMountModeRO},
				},
			}
		})

		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			passwd, err := os.OpenFile(filepath.Join(spec.Root.Path, "etc", "passwd"), os.O_WRONLY|os.O_APPEND, 0)
			Expect(err).NotTo(HaveOccurred())
			_, err = fmt.Fprintf(passwd, "\n%s:x:4242:4242:Parent only:/:/bin/sh\n", parentUser)
			Expect(passwd.Close()).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
			resolvedUser, err := users.LookupUser(spec.Root.Path, parentUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolvedUser.Uid).To(Equal(4242))
			_, err = users.LookupUser(imageRootfs, parentUser)
			Expect(err).To(HaveOccurred())
			_, err = users.LookupUser(spec.Root.Path, imageUser)
			Expect(err).To(HaveOccurred())
			resolvedUser, err = users.LookupUser(imageRootfs, imageUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolvedUser.Uid).To(Equal(5252))
			for _, absentPath := range []string{"bin/image-only", "etc/cf-assets/contract-extra/sentinel", "etc/cf-assets/healthcheck/healthcheck", "etc/cf-assets/envoy/envoy", "etc/cf-assets/envoy_config/envoy.yaml"} {
				_, err := os.Stat(filepath.Join(spec.Root.Path, absentPath))
				Expect(os.IsNotExist(err)).To(BeTrue(), absentPath)
			}
			spec.Mounts = append(spec.Mounts, specs.Mount{Type: "bind", Source: assetSource, Destination: assetDestination, Options: []string{"rbind", "ro"}})
			spec.Mounts = append(spec.Mounts, parentAssetMounts...)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			parentCreated = true
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			var state string
			parentPID, state, err = cnerd.State(testLogger, containerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(parentPID).To(BeNumerically(">", 0))
			Expect(state).To(Equal(string(client.Running)))

			mkdirer := new(runcontainerdfakes.FakeMkdirer)
			mkdirer.MkdirAsStub = func(rootfs string, _, _ int, _ os.FileMode, _ bool, paths ...string) error {
				for _, path := range paths {
					if _, err := os.Stat(filepath.Join(rootfs, path)); err != nil {
						return err
					}
				}
				return nil
			}
			builder := processes.NewBuilder(processes.EnvFunc(processes.UnixEnvFor), nil)
			actualRuntime := runcontainerd.New(cnerd, cnerd, builder, users.LookupFunc(users.LookupUser), nil, nil, true, nil, mkdirer, nil, true, nil)
			runtime = new(rundmcfakes.FakeOCIRuntime)
			runtime.BundleInfoStub = actualRuntime.BundleInfo
			runtime.ExecStub = actualRuntime.Exec
			peaCreator = new(rundmcfakes.FakePeaCreator)
			peaUserResolver = new(rundmcfakes.FakePeaUsernameResolver)
			containerizer := rundmc.New(nil, nil, runtime, nil, nil, nil, nil, peaCreator, peaUserResolver, 0, nil, nil, true)
			container := new(gardenfakes.FakeContainer)
			container.HandleReturns(containerID)
			container.RunStub = func(processSpec garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
				return containerizer.Run(testLogger, containerID, processSpec, processIO)
			}
			backend := new(gardenfakes.FakeBackend)
			backend.LookupReturns(container, nil)
			socket := filepath.Join(fifoDir, "garden.sock")
			listener, err := net.Listen("unix", socket)
			Expect(err).NotTo(HaveOccurred())
			gardenServer = server.New("unix", socket, time.Minute, time.Second, backend, testLogger)
			Expect(gardenServer.SetupBomberman()).To(Succeed())
			served = make(chan error, 1)
			go func() { served <- gardenServer.Serve(listener) }()
			gardenClient = connection.New("unix", socket)
			Expect(gardenClient.Ping()).To(Succeed())
			runRequest = func(processSpec garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
				if processSpec.ID == "" {
					processSpec.ID = fmt.Sprintf("%s-contract-%d", containerID, len(requestIDs)+1)
				}
				requestIDs = append(requestIDs, processSpec.ID)
				process, err := gardenClient.Run(containerID, processSpec, processIO)
				if err != nil {
					failedIDs = append(failedIDs, processSpec.ID)
				}
				return process, err
			}
			expectRemoved = func(id string) {
				Eventually(func() error {
					_, err := cnerd.GetProcess(testLogger, containerID, id)
					return err
				}, "10s").Should(MatchError(runcontainerd.ProcessNotFoundError{Handle: containerID, ID: id}))
			}
			expectHealthy = func() {
				pid, state, err := cnerd.State(testLogger, containerID)
				Expect(err).NotTo(HaveOccurred())
				Expect(pid).To(Equal(parentPID))
				Expect(state).To(Equal(string(client.Running)))
				controlOutput := gbytes.NewBuffer()
				control, err := runRequest(garden.ProcessSpec{
					Path: "/bin/sh", Dir: "/", User: "root",
					Args: []string{"-c", "test ! -e /bin/image-only && test ! -e /etc/cf-assets/contract-extra/sentinel && printf 'parent-healthy:%s\\n' \"$1\"", "control", sentinel},
				}, garden.ProcessIO{Stdout: controlOutput})
				Expect(err).NotTo(HaveOccurred())
				exitStatus, err := control.Wait()
				Expect(err).NotTo(HaveOccurred())
				Expect(exitStatus).To(BeZero())
				Expect(string(controlOutput.Contents())).To(Equal("parent-healthy:" + sentinel + "\n"))
				for _, id := range requestIDs {
					expectRemoved(id)
				}
				handles, err := cnerd.BundleIDs()
				Expect(err).NotTo(HaveOccurred())
				Expect(handles).To(ConsistOf(containerID))
				Expect(peaCreator.CreatePeaCallCount()).To(BeZero())
				Expect(peaUserResolver.ResolveUserCallCount()).To(BeZero())
			}
		})

		AfterEach(func() {
			defer func() {
				if gardenServer != nil {
					Expect(gardenServer.Stop()).To(Succeed())
					Eventually(served, "10s").Should(Receive(BeNil()))
				}
			}()
			if parentCreated {
				for _, id := range failedIDs {
					if process, err := cnerd.GetProcess(testLogger, containerID, id); err == nil {
						Expect(process.Delete()).To(Succeed())
					}
				}
				Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())
			}
		})

		It("executes the parent CF asset with preloaded and packaged images zero limits real stdin stdout stderr exit 17 and numeric and parent-only named UID", func() {
			for index, user := range []string{"4242:4242", parentUser} {
				request.ID = fmt.Sprintf("%s-asset-%d", containerID, index)
				request.User = user
				if index == 1 {
					request.Image.URI = packagedImage
					request.OverrideContainerLimits = &garden.ProcessLimits{}
				}
				output, errors := gbytes.NewBuffer(), gbytes.NewBuffer()
				input := "stdin-" + sentinel
				process, err := runRequest(request, garden.ProcessIO{Stdin: bytes.NewBufferString(input + "\n"), Stdout: output, Stderr: errors})
				Expect(err).NotTo(HaveOccurred())
				Expect(process.ID()).To(Equal(request.ID))
				exitStatus, err := process.Wait()
				Expect(err).NotTo(HaveOccurred())
				Expect(exitStatus).To(Equal(17))
				Expect(string(output.Contents())).To(Equal("asset:" + sentinel + "\nuid:4242\nstdin:" + input + "\nenv:" + sentinel + "\ncwd:/\n"))
				Expect(string(errors.Contents())).To(Equal("stderr:" + input + "\n"))
				_, _, translated, _ := runtime.ExecArgsForCall(index)
				Expect(translated.Image).To(Equal(garden.ImageRef{}))
				Expect(translated.BindMounts).To(BeNil())
				Expect(translated.OverrideContainerLimits).To(Equal(request.OverrideContainerLimits))
				expectRemoved(process.ID())
			}
			expectHealthy()
		})

		It("executes the pinned Envoy shell wrapper with packaged image parent binary and config mounts real IO and wait exit 23", func() {
			output, errors := gbytes.NewBuffer(), gbytes.NewBuffer()
			process, err := runRequest(envoyRequest, garden.ProcessIO{Stdout: output, Stderr: errors})
			Expect(err).NotTo(HaveOccurred())
			Expect(process.ID()).To(Equal(envoyRequest.ID))
			exitStatus, err := process.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(exitStatus).To(Equal(23))
			Expect(string(output.Contents())).To(Equal("envoy:" + sentinel + "\nconfig:" + configSentinel + "\nuid:4242\n"))
			Expect(string(errors.Contents())).To(Equal("envoy-stderr:" + configSentinel + "\n"))
			Expect(runtime.ExecCallCount()).To(Equal(1))
			_, _, translated, _ := runtime.ExecArgsForCall(0)
			expected := envoyRequest
			expected.Image = garden.ImageRef{}
			expected.BindMounts = nil
			Expect(translated).To(Equal(expected))
			expectRemoved(process.ID())
			expectHealthy()
		})

		It("rejects mutated Envoy wrappers and arbitrary shell commands before Exec over Garden", func() {
			cases := []struct {
				name string
				args []string
			}{
				{"changed config path", []string{"-c", strings.Replace(envoyWrapper, "envoy_config/envoy.yaml", "envoy_config/other.yaml", 1)}},
				{"extra Envoy argument", []string{"-c", strings.Replace(envoyWrapper, "critical&", "critical --extra&", 1)}},
				{"shell trailer", []string{"-c", envoyWrapper + "; printf 'unapproved\\n'"}},
				{"arbitrary shell command", []string{"-c", "printf 'unapproved\\n'"}},
				{"extra shell argument", []string{"-c", envoyWrapper, "extra"}},
				{"negative drain time", []string{"-c", strings.Replace(envoyWrapper, "--drain-time-s 0", "--drain-time-s -1", 1)}},
				{"noncanonical drain time", []string{"-c", strings.Replace(envoyWrapper, "--drain-time-s 0", "--drain-time-s 00", 1)}},
				{"overflowing drain time", []string{"-c", strings.Replace(envoyWrapper, "--drain-time-s 0", "--drain-time-s 18446744073709551616", 1)}},
				{"changed log level", []string{"-c", strings.Replace(envoyWrapper, "critical&", "info&", 1)}},
			}
			for index, testCase := range cases {
				By(testCase.name)
				rejected := envoyRequest
				rejected.ID = fmt.Sprintf("%s-envoy-rejected-%d", containerID, index)
				rejected.Args = testCase.args
				output, errors := gbytes.NewBuffer(), gbytes.NewBuffer()
				process, err := runRequest(rejected, garden.ProcessIO{Stdout: output, Stderr: errors})
				Expect(err).To(MatchError(ContainSubstring("unsupported pea exec: executable must be an existing parent CF asset")))
				Expect(process).To(BeNil())
				Expect(runtime.ExecCallCount()).To(BeZero())
				Expect(output.Contents()).To(BeEmpty())
				Expect(errors.Contents()).To(BeEmpty())
				expectRemoved(rejected.ID)
			}
			expectHealthy()
		})

		for _, missingDestination := range []string{envoyDestination, envoyConfigDestination} {
			Context("without the parent mount at "+missingDestination, func() {
				BeforeEach(func() {
					var mounts []specs.Mount
					for _, mount := range parentAssetMounts {
						if mount.Destination != missingDestination {
							mounts = append(mounts, mount)
						}
					}
					parentAssetMounts = mounts
				})

				It("rejects the exact Envoy wrapper before Exec over Garden and leaves the parent healthy", func() {
					Expect(spec.Mounts).NotTo(ContainElement(HaveField("Destination", missingDestination)))
					output, errors := gbytes.NewBuffer(), gbytes.NewBuffer()
					process, err := runRequest(envoyRequest, garden.ProcessIO{Stdout: output, Stderr: errors})
					Expect(err).To(MatchError(ContainSubstring("unsupported pea exec: executable is not covered by a parent CF asset bind mount")))
					Expect(process).To(BeNil())
					Expect(runtime.ExecCallCount()).To(BeZero())
					Expect(output.Contents()).To(BeEmpty())
					Expect(errors.Contents()).To(BeEmpty())
					expectRemoved(envoyRequest.ID)
					expectHealthy()
				})
			})
		}

		It("rejects image-only executables new or changed mounts and per-pea CPU or memory limits before Exec over Garden", func() {
			cases := []struct {
				name   string
				change func(*garden.ProcessSpec)
				error  string
			}{
				{"distinct image", func(request *garden.ProcessSpec) { request.Image.URI = "file://" + imageRootfs }, "only preloaded CF asset compatibility requests"},
				{"image-only executable", func(request *garden.ProcessSpec) { request.Path = "/bin/image-only" }, "executable must be an existing parent CF asset"},
				{"new mount", func(request *garden.ProcessSpec) {
					request.BindMounts = append(request.BindMounts, garden.BindMount{SrcPath: newMountSource, DstPath: "/etc/cf-assets/contract-extra", Origin: garden.BindMountOriginHost, Mode: garden.BindMountModeRO})
				}, "requested bind mount is not identical to the parent mount"},
				{"changed source", func(request *garden.ProcessSpec) { request.BindMounts[0].SrcPath = newMountSource }, "requested bind mount is not identical to the parent mount"},
				{"changed permissions", func(request *garden.ProcessSpec) { request.BindMounts[0].Mode = garden.BindMountModeRW }, "requested bind mount is not identical to the parent mount"},
				{"CPU limit", func(request *garden.ProcessSpec) {
					request.OverrideContainerLimits = &garden.ProcessLimits{CPU: garden.CPULimits{LimitInShares: 123}}
				}, "independent CPU or memory limits are unavailable"},
				{"memory limit", func(request *garden.ProcessSpec) {
					request.OverrideContainerLimits = &garden.ProcessLimits{Memory: garden.MemoryLimits{LimitInBytes: 1024 * 1024}}
				}, "independent CPU or memory limits are unavailable"},
			}
			for index, testCase := range cases {
				By(testCase.name)
				rejected := request
				rejected.ID = fmt.Sprintf("%s-rejected-%d", containerID, index)
				rejected.BindMounts = append([]garden.BindMount{}, request.BindMounts...)
				testCase.change(&rejected)
				output, errors := gbytes.NewBuffer(), gbytes.NewBuffer()
				process, err := runRequest(rejected, garden.ProcessIO{Stdout: output, Stderr: errors})
				Expect(err).To(MatchError(ContainSubstring("unsupported pea exec: " + testCase.error)))
				Expect(process).To(BeNil())
				Expect(runtime.ExecCallCount()).To(BeZero())
				Expect(output.Contents()).To(BeEmpty())
				Expect(errors.Contents()).To(BeEmpty())
				expectRemoved(rejected.ID)
			}
			expectHealthy()
		})

		It("rejects a named user present only in the candidate image rootfs without creating an exec process", func() {
			request.User = imageUser
			process, err := runRequest(request, garden.ProcessIO{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to find user " + imageUser))
			Expect(process).To(BeNil())
			Expect(runtime.ExecCallCount()).To(Equal(1))
			expectRemoved(request.ID)
			expectHealthy()
		})

		It("removes an exec whose CF asset executable fails to start and keeps the parent healthy", func() {
			request.Path = assetDestination + "/does-not-exist"
			_, err := os.Stat(filepath.Join(assetSource, "does-not-exist"))
			Expect(os.IsNotExist(err)).To(BeTrue())
			process, err := runRequest(request, garden.ProcessIO{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does-not-exist"))
			Expect(process).To(BeNil())
			Expect(runtime.ExecCallCount()).To(Equal(1))
			expectHealthy()
		})

		It("kills and waits for a real Garden CF asset process removes its exec ID and runs another parent process", func() {
			request.Args = []string{"hold"}
			output := gbytes.NewBuffer()
			process, err := runRequest(request, garden.ProcessIO{Stdout: output})
			Expect(err).NotTo(HaveOccurred())
			Expect(process.ID()).To(Equal(request.ID))
			Eventually(output, "10s").Should(gbytes.Say("ready"))
			backing, err := cnerd.GetProcess(testLogger, containerID, request.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(backing.ID()).To(Equal(request.ID))
			Expect(process.Signal(garden.SignalKill)).To(Succeed())
			exitStatus, err := process.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(exitStatus).To(Equal(128 + int(syscall.SIGKILL)))
			expectRemoved(request.ID)
			expectHealthy()
		})
	})

	Describe("Create", func() {
		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("creates the containerd container by id", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())

			containers := listContainers(testConfig.CtrBin, testConfig.Socket)
			Expect(containers).To(ContainSubstring(containerID))
		})

		It("starts an init process in the container", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())

			processes := listProcesses(testConfig.CtrBin, testConfig.Socket, containerID)
			// init processes look like this in ctr:
			// PID    INFO
			// 12345  -
			Expect(processes).To(MatchRegexp(`\d+\s+-`))
		})

		It("adds the annotations as labels", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Annotations = map[string]string{"hello": "world", "goodbye": "potato"}

			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(containerLabels(testConfig.CtrBin, testConfig.Socket, containerID)).To(HaveKeyWithValue("hello", "world"))
			Expect(containerLabels(testConfig.CtrBin, testConfig.Socket, containerID)).To(HaveKeyWithValue("goodbye", "potato"))
		})

		It("writes stdout", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Process.Args = []string{"/bin/sh", "-c", "echo hi && sleep 60"}
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			Eventually(stdout, "30s").Should(gbytes.Say("hi"))
		})

		It("writes stderr", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Process.Args = []string{"/bin/sh", "-c", "echo hi 1>&2 && sleep 60"}
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			Eventually(stderr, "30s").Should(gbytes.Say("hi"))
		})

		It("reads from stdin", func() {
			stdin = bytes.NewBufferString("hi")
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Process.Args = []string{"/bin/sh", "-c", "cat && sleep 60"}
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			Eventually(stdout, "30s").Should(gbytes.Say("hi"))
		})
	})

	Describe("Spec", func() {
		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("returns the container spec", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())

			actualSpec, err := cnerd.Spec(testLogger, containerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(actualSpec).To(Equal(spec))
		})

		It("fails when container id not found", func() {
			_, err := cnerd.Spec(testLogger, "notAContainerId")
			Expect(err).To(MatchError("container notAContainerId not found"))
		})
	})

	Describe("Exec", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("execs a process in the container", func() {
			processSpec := &specs.Process{
				Args: []string{"/bin/sleep", "30"},
				Cwd:  "/",
			}

			proc, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())
			Expect(proc.ID()).To(Equal(processID))

			loaded, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded.ID()).To(Equal(processID))
		})

		Describe("process IO", func() {
			It("reads from stdin", func() {
				processSpec := &specs.Process{
					Args: []string{"/bin/cat", "/proc/self/fd/0"},
					Cwd:  "/",
				}

				stdout := gbytes.NewBuffer()
				processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
					stdin := gbytes.BufferWithBytes([]byte("hello nerd"))
					stdout := io.MultiWriter(stdout, GinkgoWriter)

					return stdin, stdout, nil, false
				}

				_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
				Expect(err).NotTo(HaveOccurred())
				Eventually(stdout).Should(gbytes.Say("hello nerd"))
			})

			It("writes to stdout", func() {
				processSpec := &specs.Process{
					Args: []string{"/bin/echo", "hello nerd"},
					Cwd:  "/",
				}

				stdout := gbytes.NewBuffer()
				processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
					stdout := io.MultiWriter(stdout, GinkgoWriter)
					return nil, stdout, nil, false
				}

				_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
				Expect(err).NotTo(HaveOccurred())
				Eventually(stdout).Should(gbytes.Say("hello nerd"))
			})

			It("writes to stderr", func() {
				processSpec := &specs.Process{
					Args: []string{"/bin/cat", "notafile.txt"},
					Cwd:  "/",
				}

				stderr := gbytes.NewBuffer()
				processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
					stderr := io.MultiWriter(stderr, GinkgoWriter)
					return nil, nil, stderr, false
				}

				_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
				Expect(err).NotTo(HaveOccurred())
				Eventually(stderr).Should(gbytes.Say("No such file"))
			})

			It("has access to a pseudo terminal", func() {
				processSpec := &specs.Process{
					Args: []string{"/bin/test", "-t", "1"},
					Cwd:  "/",
				}

				processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
					return nil, nil, nil, true
				}

				proc, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
				Expect(err).NotTo(HaveOccurred())

				exitCode, err := proc.Wait()
				Expect(err).NotTo(HaveOccurred())
				Expect(exitCode).To(BeZero())
			})
		})
	})

	Describe("Wait", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())

			processSpec := &specs.Process{
				Args: []string{"/bin/sh", "-c", "exit 17"},
				Cwd:  "/",
			}

			_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("succeeds", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())
			_, err = proc.Wait()
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the exit code", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())
			exitCode, err := proc.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(17))
		})

		It("allows you to call Wait more than once", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())
			_, err = proc.Wait()
			Expect(err).NotTo(HaveOccurred())

			_, err = proc.Wait()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Signal", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			stdoutBuffer := gbytes.NewBuffer()
			processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
				return nil, stdoutBuffer, nil, false
			}

			processSpec := &specs.Process{
				Args: []string{"/bin/sh", "-c", `
					trap 'exit 42' TERM

					while true; do
					  echo 'sleeping'
					  sleep 1
					done
				`},
				Cwd: "/",
			}

			_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())

			Eventually(stdoutBuffer).Should(gbytes.Say("sleeping"))
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("should forward signals to the process", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())

			Expect(proc.Signal(syscall.SIGTERM)).To(Succeed())

			status := make(chan int)
			go func() {
				exit, err := proc.Wait()
				Expect(err).NotTo(HaveOccurred())
				status <- exit
			}()

			Eventually(status, 5*time.Second).Should(Receive(BeEquivalentTo(42)))
		})
	})

	Describe("GetProcess", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())

			processSpec := &specs.Process{
				Args: []string{"/bin/sleep", "30"},
				Cwd:  "/",
			}

			_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("execs a process in the container", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())

			Expect(proc.ID()).To(Equal(processID))
		})

		Context("when the container does not exist", func() {
			JustBeforeEach(func() {
				cnerd.Delete(testLogger, containerID)
			})

			It("fails", func() {
				_, err := cnerd.GetProcess(testLogger, containerID, processID)
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})

		Context("when the process does not exist", func() {
			It("fails", func() {
				_, err := cnerd.GetProcess(testLogger, containerID, "not-existing-process-id")
				Expect(err).To(MatchError(ContainSubstring("process not-existing-process-id not found in container %s", containerID)))
			})
		})
	})

	Describe("GetTask", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("gets a backing process object containing the task for a given container ID", func() {
			task, err := cnerd.GetTask(testLogger, containerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(task.ID()).To(Equal(containerID))
		})

		It("returns an error when the container doesn't exist", func() {
			_, err := cnerd.GetTask(testLogger, "does-not-exist")
			Expect(err).To(MatchError(runcontainerd.ContainerNotFoundError{Handle: "does-not-exist"}))
		})
	})

	Describe("RemoveBundle", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())
		})

		It("deletes the containerd container by id", func() {
			Expect(cnerd.RemoveBundle(testLogger, containerID)).To(Succeed())

			containers := listContainers(testConfig.CtrBin, testConfig.Socket)
			Expect(containers).NotTo(ContainSubstring(containerID))
		})
	})

	Describe("Delete", func() {
		var setup deleteSpecSetup

		BeforeEach(func() {
			setup = deleteSpecSetup{}
		})

		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			if testRuntimePolicy.IsAlternative() {
				Expect(syscall.Mount(spec.Root.Path, spec.Root.Path, "", syscall.MS_BIND|syscall.MS_REC, "")).To(Succeed())
				rootfsMounted = true
			}
			if os.Getenv("NERD_TEST_CREATED_INVALID_START") == "true" {
				spec.Process.Args = []string{"/cfar1508-command-does-not-exist"}
			}
			Expect(setup.create(func() error {
				return cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)
			})).To(Succeed())
		})

		AfterEach(func() {
			setup.cleanup(func() {
				_, _, err := cnerd.State(testLogger, containerID)
				if err == nil {
					Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())
				} else {
					Expect(err).To(BeAssignableToTypeOf(runcontainerd.TaskNotFoundError{}))
				}
				Expect(cnerd.RemoveBundle(testLogger, containerID)).To(Succeed())
				containers, err := containerdClient.Containers(containerdContext)
				Expect(err).NotTo(HaveOccurred())
				for _, container := range containers {
					Expect(container.ID()).NotTo(Equal(containerID), "RemoveBundle left containerd metadata")
				}
			})
		})

		It("deletes the containerd task by container id", func() {
			Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())

			tasks := listTasks(testConfig.CtrBin, testConfig.Socket)
			Expect(tasks).NotTo(ContainSubstring(containerID))
		})

		Context("with an alternative Runtime v2 shim", func() {
			BeforeEach(func() {
				if !testRuntimePolicy.IsAlternative() {
					Skip("requires an alternative Runtime v2 shim")
				}
			})

			It("deletes a created alternative-runtime task through public TaskService", func() {
				pid, status, err := cnerd.State(testLogger, containerID)
				Expect(err).NotTo(HaveOccurred())
				Expect(pid).NotTo(BeZero())
				Expect(status).To(Equal(string(client.Created)))
				identities, err := captureTaskProcesses(pid, containerID)
				Expect(err).NotTo(HaveOccurred(), "cannot prove pre-delete task runtime process identities")
				fmt.Fprintf(GinkgoWriter, "task %s pre-delete runtime identities: %+v\n", containerID, identities)

				Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())

				tasks := listTasks(testConfig.CtrBin, testConfig.Socket)
				Expect(tasks).NotTo(ContainSubstring(containerID))
				containers := listContainers(testConfig.CtrBin, testConfig.Socket)
				Expect(containers).To(ContainSubstring(containerID))
				Eventually(func() error { return taskProcessesGone(identities, procStat) }, "10s", "100ms").Should(Succeed())

				if cgrouputils.IsCgroup2UnifiedMode() {
					err := os.Remove(filepath.Join("/sys/fs/cgroup", fmt.Sprintf("nerdspace%d", GinkgoParallelProcess()), containerID))
					Expect(err == nil || os.IsNotExist(err)).To(BeTrue())
				}
			})

			It("converges the standard pre-start task lifecycle without cleanup7", func() {
				if os.Getenv("NERD_TEST_STANDARD_CREATED_DELETE") != "true" {
					Skip("opt-in CFAR-1508 replacement regression")
				}
				pid, state, err := cnerd.State(testLogger, containerID)
				Expect(err).NotTo(HaveOccurred())
				Expect(pid).NotTo(BeZero())
				Expect(state).To(Equal(string(client.Created)))
				identities, err := captureTaskProcesses(pid, containerID)
				Expect(err).NotTo(HaveOccurred())
				if os.Getenv("NERD_TEST_CREATED_RESTART") == "true" {
					Expect(containerdProcess.Signal(syscall.SIGTERM)).To(Succeed())
					waitStatus, waitErr := containerdProcess.Wait()
					Expect(waitErr).NotTo(HaveOccurred())
					Expect(waitStatus.ExitCode()).To(BeZero())
					containerdProcess = containerdrunner.NewContainerdProcess(testConfig.RunDir, containerdrunner.ContainerdConfig(testConfig.RunDir))
					pidAfterRestart, stateAfterRestart, stateErr := cnerd.State(testLogger, containerID)
					Expect(stateErr).NotTo(HaveOccurred())
					Expect(pidAfterRestart).To(Equal(pid))
					Expect(stateAfterRestart).To(Equal(state))
					fmt.Fprintf(GinkgoWriter, "CFAR1508 private containerd restarted with task id=%s pid=%d\n", containerID, pid)
				}
				defer func() {
					for _, identity := range identities {
						if filepath.Base(identity.Executable) != "containerd-shim-runsc-v1" {
							continue
						}
						current, err := readProcIdentity(identity.PID)
						if os.IsNotExist(err) {
							continue
						}
						Expect(err).NotTo(HaveOccurred())
						if current.Start != identity.Start {
							continue
						}
						Expect(current.Executable).To(Equal(identity.Executable))
						Expect(procFlag(current.Args, "-id", containerID)).To(BeTrue())
						Expect(syscall.Kill(identity.PID, syscall.SIGKILL)).To(Succeed())
					}
					Eventually(func() error { return taskProcessesGone(identities, procStat) }, "10s", "100ms").Should(Succeed())
					Eventually(func() string { return listTasks(testConfig.CtrBin, testConfig.Socket) }, "10s", "100ms").ShouldNot(ContainSubstring(containerID))
				}()
				ctx, cancel := context.WithTimeout(containerdContext, 150*time.Second)
				defer cancel()
				standardDeleter := nerd.New(containerdClient, ctx, fifoDir, mp, runcontainerd.StockRuntime)
				started := time.Now()
				fmt.Fprintf(GinkgoWriter, "CFAR1508 standard-delete begin id=%s pid=%d state=%s identities=%+v\n", containerID, pid, state, identities)
				if os.Getenv("NERD_TEST_DIRECT_CREATED_DELETE") == "true" {
					eventCtx, cancelEvents := context.WithTimeout(containerdContext, 10*time.Second)
					defer cancelEvents()
					eventStream, eventErrors := containerdClient.Subscribe(eventCtx)
					barrierTopic := "/cfar1508/subscription-barrier"
					barrierID := containerID + "-barrier"
					barrierTicker := time.NewTicker(100 * time.Millisecond)
					defer barrierTicker.Stop()
					synchronized := false
					for !synchronized {
						select {
						case <-barrierTicker.C:
							Expect(containerdClient.EventService().Publish(eventCtx, barrierTopic, &apievents.TaskCreate{ContainerID: barrierID})).To(Succeed())
						case envelope := <-eventStream:
							Expect(envelope).NotTo(BeNil())
							if envelope.Topic == barrierTopic {
								decoded, decodeErr := typeurl.UnmarshalAny(envelope.Event)
								Expect(decodeErr).NotTo(HaveOccurred())
								barrier, ok := decoded.(*apievents.TaskCreate)
								Expect(ok).To(BeTrue())
								synchronized = barrier.ContainerID == barrierID
							}
						case streamErr := <-eventErrors:
							Fail(fmt.Sprintf("event subscription failed before barrier: %v", streamErr))
						case <-eventCtx.Done():
							Fail("event subscription barrier deadline exceeded")
						}
					}
					fmt.Fprintf(GinkgoWriter, "CFAR1508 subscription synchronized id=%s\n", containerID)
					if os.Getenv("NERD_TEST_CREATED_INVALID_START") == "true" {
						startErr := cnerd.Start(testLogger, containerID)
						Expect(startErr).To(HaveOccurred())
						fmt.Fprintf(GinkgoWriter, "CFAR1508 invalid command Start rejected id=%s error=%v\n", containerID, startErr)
					}
					if os.Getenv("NERD_TEST_CREATED_CALLER") == "true" {
						err = cnerd.Delete(testLogger, containerID)
					} else {
						_, err = containerdClient.TaskService().Delete(ctx, &tasksapi.DeleteTaskRequest{ContainerID: containerID})
					}
					Expect(err).NotTo(HaveOccurred())
					_, _, deletedStateErr := cnerd.State(testLogger, containerID)
					Expect(deletedStateErr).To(BeAssignableToTypeOf(runcontainerd.TaskNotFoundError{}))
					Eventually(func() error { return taskProcessesGone(identities, procStat) }, "10s", "100ms").Should(Succeed())
					fmt.Fprintf(GinkgoWriter, "CFAR1508 direct-delete task and captured runtime processes absent id=%s\n", containerID)
					deleted := false
					for !deleted {
						select {
						case envelope := <-eventStream:
							Expect(envelope).NotTo(BeNil())
							decoded, decodeErr := typeurl.UnmarshalAny(envelope.Event)
							Expect(decodeErr).NotTo(HaveOccurred())
							fmt.Fprintf(GinkgoWriter, "CFAR1508 event topic=%s event=%+v\n", envelope.Topic, decoded)
							switch event := decoded.(type) {
							case *apievents.TaskStart:
								Expect(event.ContainerID).NotTo(Equal(containerID))
							case *apievents.TaskExit:
								Expect(event.ContainerID).NotTo(Equal(containerID))
							case *apievents.TaskDelete:
								deleted = event.ContainerID == containerID
							}
						case streamErr := <-eventErrors:
							Fail(fmt.Sprintf("event stream ended before TaskDelete: %v", streamErr))
						case <-eventCtx.Done():
							Fail("no TaskDelete event before diagnostic deadline")
						}
					}
					if os.Getenv("NERD_TEST_CREATED_CALLER") == "true" {
						nextID := containerID + "-next"
						defer func() {
							Expect(cnerd.Delete(testLogger, nextID)).To(Succeed())
							Expect(cnerd.RemoveBundle(testLogger, nextID)).To(Succeed())
						}()
						nextSpec := *spec
						nextProcess := *spec.Process
						nextProcess.Args = []string{"/bin/sleep", "1000"}
						nextSpec.Process = &nextProcess
						Expect(cnerd.Create(testLogger, nextID, &nextSpec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
						nextPID, _, nextErr := cnerd.State(testLogger, nextID)
						Expect(nextErr).NotTo(HaveOccurred())
						nextIdentities, captureErr := captureTaskProcesses(nextPID, nextID)
						Expect(captureErr).NotTo(HaveOccurred())
						Expect(cnerd.Start(testLogger, nextID)).To(Succeed())
						Expect(cnerd.Delete(testLogger, nextID)).To(Succeed())
						Eventually(func() error { return taskProcessesGone(nextIdentities, procStat) }, "10s", "100ms").Should(Succeed())
						var lifecycle []string
						for len(lifecycle) < 3 {
							select {
							case envelope := <-eventStream:
								Expect(envelope).NotTo(BeNil())
								decoded, decodeErr := typeurl.UnmarshalAny(envelope.Event)
								Expect(decodeErr).NotTo(HaveOccurred())
								var eventID string
								switch event := decoded.(type) {
								case *apievents.TaskStart:
									eventID = event.ContainerID
								case *apievents.TaskExit:
									eventID = event.ContainerID
								case *apievents.TaskDelete:
									eventID = event.ContainerID
								}
								if eventID == nextID {
									lifecycle = append(lifecycle, envelope.Topic)
								}
							case streamErr := <-eventErrors:
								Fail(fmt.Sprintf("subsequent lifecycle event stream: %v", streamErr))
							case <-eventCtx.Done():
								Fail(fmt.Sprintf("subsequent lifecycle event deadline: %v", lifecycle))
							}
						}
						Expect(lifecycle).To(Equal([]string{"/tasks/start", "/tasks/exit", "/tasks/delete"}))
						fmt.Fprintf(GinkgoWriter, "CFAR1508 subsequent started lifecycle clean id=%s events=%v\n", nextID, lifecycle)
					}
				} else {
					err = standardDeleter.Delete(testLogger, containerID)
				}
				finalPID, finalState, stateErr := cnerd.State(testLogger, containerID)
				fmt.Fprintf(GinkgoWriter, "CFAR1508 standard-delete end elapsed=%s error=%v pid=%d state=%s state_error=%v\n", time.Since(started), err, finalPID, finalState, stateErr)
				Expect(err).NotTo(HaveOccurred())
				Expect(stateErr).To(BeAssignableToTypeOf(runcontainerd.TaskNotFoundError{}))
				Eventually(func() error { return taskProcessesGone(identities, procStat) }, "10s", "100ms").Should(Succeed())
				Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())
			})
		})
	})

	Describe("State", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("gets a nonzero pid and created status before Start", func() {
			pid, status, err := cnerd.State(testLogger, containerID)

			Expect(err).NotTo(HaveOccurred())
			Expect(pid).NotTo(BeZero())
			Expect(status).To(BeEquivalentTo(client.Created))
		})

		It("gets the pid and status of a running task", func() {
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			pid, status, err := cnerd.State(testLogger, containerID)

			Expect(err).NotTo(HaveOccurred())
			Expect(pid).NotTo(BeZero())
			Expect(status).To(BeEquivalentTo(client.Running))
		})
	})

	Describe("GetContainerPID", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("gets the container init process pid", func() {
			containerPid, err := cnerd.GetContainerPID(testLogger, containerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(containerPid).NotTo(BeZero())
			if testRuntimePolicy == runcontainerd.RunscRuntime {
				cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", containerPid))
				Expect(err).NotTo(HaveOccurred())
				arguments := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
				Expect(filepath.Base(arguments[0])).To(Equal("runsc-sandbox"))
				Expect(arguments).To(ContainElement("boot"))
				Expect(arguments).To(ContainElement(containerID))
			} else {
				procls := listProcesses(testConfig.CtrBin, testConfig.Socket, containerID)
				Expect(procls).To(ContainSubstring(strconv.Itoa(int(containerPid))))
			}
		})
	})

	Describe("OOMEvents", func() {
		var (
			processSpec *specs.Process
			events      <-chan *apievents.TaskOOM
		)

		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID, withLinuxResources(specs.LinuxResources{
				Memory: &specs.LinuxMemory{
					Limit: int64ptr(30 * 1024 * 1024),
					Swap:  int64ptr(30 * 1024 * 1024),
				},
			}))
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())

			events = cnerd.OOMEvents(testLogger)
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())

			proc, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())
			_, err = proc.Wait()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		Context("when OOM does not occur", func() {
			BeforeEach(func() {
				processSpec = &specs.Process{
					Args: []string{"/bin/echo", "hi"},
					Cwd:  "/",
				}
			})

			It("does not report OOM", func() {
				Consistently(events).ShouldNot(Receive())
			})
		})

		Context("when OOM occurs", func() {
			BeforeEach(func() {
				processSpec = &specs.Process{
					Args: []string{"/bin/usemem"},
					Cwd:  "/",
				}
			})

			It("reports OOM", func() {
				var event *apievents.TaskOOM
				Eventually(events, "10s").Should(Receive(&event), "<requesting dmesg>")
				Expect(event.ContainerID).To(Equal(containerID))
			})
		})
	})

	Context("when containerd is not running", func() {
		BeforeEach(func() {
			Expect(containerdProcess.Signal(syscall.SIGTERM)).To(Succeed())
			waitStatus, err := containerdProcess.Wait()
			Expect(err).NotTo(HaveOccurred())
			containerdProcess = nil
			Expect(waitStatus.ExitCode()).To(BeZero())
		})

		It("does not panic when loading the container metadata", func() {
			Expect(func() { cnerd.State(testLogger, "a-random-id") }).NotTo(Panic())
		})
	})

	Describe("BundleIDs", func() {
		AfterEach(func() {
			cnerd.Delete(testLogger, "banana")
			cnerd.Delete(testLogger, "banana2")
		})

		It("returns a filtered list of container handles", func() {
			spec = generateSpec(containerdContext, containerdClient, "banana")
			spec.Annotations = map[string]string{"hello": "potato", "abc": "boohoo"}
			Expect(cnerd.Create(testLogger, "banana", spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			spec = generateSpec(containerdContext, containerdClient, "banana2")
			spec.Annotations = map[string]string{"abc": "boohoo"}
			Expect(cnerd.Create(testLogger, "banana2", spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())

			handles, err := cnerd.BundleIDs(
				runcontainerd.ContainerFilter{
					Label:        "hello",
					Value:        "potato",
					ComparisonOp: "==",
				}, runcontainerd.ContainerFilter{
					Label:        "abc",
					Value:        "boohoo",
					ComparisonOp: "==",
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(handles).To(ConsistOf("banana"))
		})
	})
})

func createRootfs(modifyRootfs func(string), perm os.FileMode) string {
	var err error
	tmpDir, err := os.MkdirTemp("", "test-rootfs")
	Expect(err).NotTo(HaveOccurred())
	unpackedRootfs := filepath.Join(tmpDir, "unpacked")
	Expect(os.Mkdir(unpackedRootfs, perm)).To(Succeed())
	runCommand(exec.Command("tar", "xf", os.Getenv("GARDEN_TEST_ROOTFS"), "-C", unpackedRootfs))
	Expect(os.Chmod(tmpDir, perm)).To(Succeed())
	modifyRootfs(unpackedRootfs)
	return unpackedRootfs
}

func generateSpec(context context.Context, client *client.Client, containerID string, opts ...oci.SpecOpts) *specs.Spec {
	spec, err := oci.GenerateSpec(context, client, &containers.Container{ID: containerID}, opts...)
	Expect(err).NotTo(HaveOccurred())
	spec.Process.Args = []string{"/bin/sleep", "999999"}
	spec.Root = &specs.Root{
		Path: createRootfs(func(_ string) {}, 0755),
	}
	spec.Annotations = map[string]string{"container-type": "some-container-type"}

	return spec
}

func withLinuxResources(resources specs.LinuxResources) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, spec *oci.Spec) error {
		spec.Linux.Resources = &resources
		return nil
	}
}

func listContainers(ctr, socket string) string {
	return runCtr(ctr, socket, []string{"containers", "list"})
}

func listTasks(ctr, socket string) string {
	return runCtr(ctr, socket, []string{"tasks", "list"})
}

func listProcesses(ctr, socket, containerID string) string {
	return runCtr(ctr, socket, []string{"tasks", "ps", containerID})
}

func containerLabels(ctr, socket, handle string) map[string]interface{} {
	output := runCtr(ctr, socket, []string{"containers", "info", handle})

	var result map[string]interface{}
	Expect(json.Unmarshal([]byte(output), &result)).To(Succeed())
	return result["Labels"].(map[string]interface{})
}

func runCtr(ctr, socket string, args []string) string {
	defaultArgs := []string{"--address", socket, "--namespace", fmt.Sprintf("nerdspace%d", GinkgoParallelProcess())}
	cmd := exec.Command(ctr, append(defaultArgs, args...)...)

	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, "10s").Should(gexec.Exit(0))

	return string(session.Out.Contents())
}

func randomInt(min, max int) int {
	return min + randomGenerator.Intn(max-min)
}

func randomString(len int) string {
	bytes := make([]byte, len)
	for i := 0; i < len; i++ {
		bytes[i] = byte(randomInt(65, 90))
	}
	return string(bytes)
}

func int64ptr(i int64) *int64 {
	return &i
}
