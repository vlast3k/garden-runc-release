package rundmc_test

import (
	"bytes"
	"errors"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/gardenfakes"
	"code.cloudfoundry.org/guardian/rundmc"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	fakes "code.cloudfoundry.org/guardian/rundmc/rundmcfakes"
	"code.cloudfoundry.org/lager/v3/lagertest"
	specs "github.com/opencontainers/runtime-spec/specs-go"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// CFAR-1519: Validated parent-rootfs exec compatibility for preloaded CF assets.
// Image and BindMounts are cleared only after checking the executable's parent
// bind mount and exact source, destination, origin, and permissions of each
// requested mount. New images, mounts, credentials, and independent limits fail
// before exec. Stock pea requests keep their original behavior.
// These fakes verify validation and forwarding, not filesystem availability,
// image isolation, or the runtime's parent-rootfs user lookup.

var _ = Describe("CFAR-1519: Pea Exec Contract", func() {
	var (
		fakeDepot               *fakes.FakeDepot
		fakeBundleGenerator     *fakes.FakeBundleGenerator
		fakeOCIRuntime          *fakes.FakeOCIRuntime
		fakeNstarRunner         *fakes.FakeNstarRunner
		fakeProcessesStopper    *fakes.FakeProcessesStopper
		fakeEventStore          *fakes.FakeEventStore
		fakeStateStore          *fakes.FakeStateStore
		fakePeaCreator          *fakes.FakePeaCreator
		fakePeaUsernameResolver *fakes.FakePeaUsernameResolver
		fakeRuntimeStopper      *fakes.FakeRuntimeStopper
		fakeCPUCgrouper         *fakes.FakeCPUCgrouper

		stockContainerizer  *rundmc.Containerizer
		gvisorContainerizer *rundmc.Containerizer
		parentBundle        goci.Bndl
		logger              = lagertest.NewTestLogger("pea-contract")
	)

	BeforeEach(func() {
		fakeDepot = new(fakes.FakeDepot)
		fakeBundleGenerator = new(fakes.FakeBundleGenerator)
		fakeOCIRuntime = new(fakes.FakeOCIRuntime)
		fakeNstarRunner = new(fakes.FakeNstarRunner)
		fakeProcessesStopper = new(fakes.FakeProcessesStopper)
		fakeEventStore = new(fakes.FakeEventStore)
		fakeStateStore = new(fakes.FakeStateStore)
		fakePeaCreator = new(fakes.FakePeaCreator)
		fakePeaUsernameResolver = new(fakes.FakePeaUsernameResolver)
		fakeRuntimeStopper = new(fakes.FakeRuntimeStopper)
		fakeCPUCgrouper = new(fakes.FakeCPUCgrouper)
		fakeBundleGenerator.GenerateReturns(goci.Bndl{Spec: specs.Spec{Version: "test"}}, nil)
		parentBundle = goci.Bndl{Spec: specs.Spec{Mounts: []specs.Mount{
			{Source: "/host/healthcheck", Destination: "/etc/cf-assets/healthcheck", Type: "bind", Options: []string{"bind", "ro"}},
			{Source: "/host/envoy-binary", Destination: "/etc/cf-assets/envoy", Type: "bind", Options: []string{"bind", "ro"}},
			{Source: "/host/envoy-config", Destination: "/etc/cf-assets/envoy_config", Type: "bind", Options: []string{"bind", "ro"}},
			{Source: "/host/diego-sshd", Destination: "/etc/cf-assets/diego-sshd", Type: "bind", Options: []string{"bind", "ro"}},
			{Source: "/host/sidecar", Destination: "/etc/cf-assets/sidecar", Type: "bind", Options: []string{"bind", "ro"}},
			{Source: "/host/source", Destination: "/etc/cf-assets/source", Type: "bind", Options: []string{"bind", "rw"}},
		}}}
		fakeOCIRuntime.BundleInfoReturns("parent-bundle", parentBundle, nil)

		stockContainerizer = rundmc.New(
			fakeDepot, fakeBundleGenerator, fakeOCIRuntime,
			rundmc.NewNstarStreamer(fakeOCIRuntime, fakeNstarRunner),
			fakeProcessesStopper, fakeEventStore, fakeStateStore,
			fakePeaCreator, fakePeaUsernameResolver, 0,
			fakeRuntimeStopper, fakeCPUCgrouper,
			false, // stock runc
		)

		gvisorContainerizer = rundmc.New(
			fakeDepot, fakeBundleGenerator, fakeOCIRuntime,
			rundmc.NewNstarStreamer(fakeOCIRuntime, fakeNstarRunner),
			fakeProcessesStopper, fakeEventStore, fakeStateStore,
			fakePeaCreator, fakePeaUsernameResolver, 0,
			fakeRuntimeStopper, fakeCPUCgrouper,
			true, // gVisor execPeas
		)
	})

	Describe("image-backed subcontainer dispatch", func() {
		It("preserves the image, mounts, named user and IO for PeaCreator", func() {
			gvisorContainerizer.EnableImageBackedPeas()
			request := garden.ProcessSpec{ID: "child-id", Path: "/child-only", User: "childuser", Image: garden.ImageRef{URI: "docker:///child-image"}, BindMounts: []garden.BindMount{{SrcPath: "/child-bind", DstPath: "/mnt/token", Mode: garden.BindMountModeRO}}}
			input := garden.ProcessIO{Stdin: bytes.NewBufferString("input"), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
			process := new(gardenfakes.FakeProcess)
			fakePeaCreator.CreatePeaReturns(process, nil)
			returned, err := gvisorContainerizer.Run(logger, "parent", request, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(returned).To(BeIdenticalTo(process))
			_, forwarded, forwardedIO, parent := fakePeaCreator.CreatePeaArgsForCall(0)
			Expect(forwarded).To(Equal(request))
			Expect(forwardedIO).To(Equal(input))
			Expect(parent).To(Equal("parent"))
			Expect(fakeOCIRuntime.ExecCallCount()).To(BeZero())
			Expect(fakeOCIRuntime.BundleInfoCallCount()).To(BeZero())
			Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero(), "host Sentry PID cannot identify a child image")
		})
	})

	Describe("Image compatibility validation", func() {
		var healthcheckSpec garden.ProcessSpec

		BeforeEach(func() {
			healthcheckSpec = garden.ProcessSpec{
				Path:  "/healthcheck/bin/healthcheck",
				User:  "root",
				Image: garden.ImageRef{URI: "preloaded:healthcheck"},
			}
		})

		It("gVisor: validates the parent asset before clearing Image for exec", func() {
			fakeProcess := new(gardenfakes.FakeProcess)
			fakeProcess.IDReturns("hc-1")
			fakeOCIRuntime.ExecReturns(fakeProcess, nil)
			healthcheckSpec.Path = "/etc/cf-assets/healthcheck/healthcheck"
			healthcheckSpec.Image = garden.ImageRef{URI: "preloaded:cflinuxfs4"}
			healthcheckSpec.BindMounts = []garden.BindMount{
				{SrcPath: "/host/healthcheck", DstPath: "/etc/cf-assets/healthcheck", Mode: garden.BindMountModeRO},
			}

			_, err := gvisorContainerizer.Run(logger, "app-handle", healthcheckSpec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeOCIRuntime.BundleInfoCallCount()).To(Equal(1))
			_, parentHandle := fakeOCIRuntime.BundleInfoArgsForCall(0)
			Expect(parentHandle).To(Equal("app-handle"))
			Expect(fakeOCIRuntime.ExecCallCount()).To(Equal(1))
			_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)

			By("clearing Image only for validated parent-rootfs compatibility")
			Expect(execSpec.Image).To(Equal(garden.ImageRef{}),
				"validated exec uses the existing parent rootfs, not a new image")

			By("preserving the caller's requested parent asset path")
			Expect(execSpec.Path).To(Equal("/etc/cf-assets/healthcheck/healthcheck"))

			By("No pea is created")
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
		})

		It("stock runc: creates a real pea with the requested image", func() {
			fakeProcess := new(gardenfakes.FakeProcess)
			fakeProcess.IDReturns("hc-2")
			fakePeaCreator.CreatePeaReturns(fakeProcess, nil)

			_, err := stockContainerizer.Run(logger, "app-handle", healthcheckSpec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePeaCreator.CreatePeaCallCount()).To(Equal(1))
			_, peaSpec, _, _ := fakePeaCreator.CreatePeaArgsForCall(0)

			By("Image is preserved for the pea creator")
			Expect(peaSpec.Image.URI).To(Equal("preloaded:healthcheck"))
			Expect(fakeOCIRuntime.ExecCallCount()).To(BeZero(),
				"stock runc must NOT exec image requests — they are real peas")
		})
	})

	Describe("verified CF producer forms", func() {
		It("accepts canonical packaged sidecar rootfs markers without changing the parent asset", func() {
			fakeOCIRuntime.ExecReturns(new(gardenfakes.FakeProcess), nil)
			request := garden.ProcessSpec{
				Path: "/etc/cf-assets/healthcheck/healthcheck", Image: garden.ImageRef{URI: "/var/vcap/packages/cflinuxfs4/rootfs.tar"},
				OverrideContainerLimits: &garden.ProcessLimits{},
				BindMounts:              []garden.BindMount{{SrcPath: "/host/healthcheck", DstPath: "/etc/cf-assets/healthcheck", Mode: garden.BindMountModeRO}},
			}
			_, err := gvisorContainerizer.Run(logger, "app-handle", request, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())
			_, _, actual, _ := fakeOCIRuntime.ExecArgsForCall(0)
			request.Image = garden.ImageRef{}
			request.BindMounts = nil
			Expect(actual).To(Equal(request))
		})

		It("preserves the pinned Envoy shell wrapper with both asset mounts", func() {
			fakeOCIRuntime.ExecReturns(new(gardenfakes.FakeProcess), nil)
			request := garden.ProcessSpec{
				Path: "sh", Image: garden.ImageRef{URI: "/var/vcap/packages/cflinuxfs4/rootfs.tar"},
				Args: []string{"-c", "trap 'kill -9 0' TERM; /etc/cf-assets/envoy/envoy -c /etc/cf-assets/envoy_config/envoy.yaml --drain-time-s 0 --log-level critical& pid=$!; wait $pid"},
				BindMounts: []garden.BindMount{
					{SrcPath: "/host/envoy-binary", DstPath: "/etc/cf-assets/envoy", Mode: garden.BindMountModeRO},
					{SrcPath: "/host/envoy-config", DstPath: "/etc/cf-assets/envoy_config", Mode: garden.BindMountModeRO},
				},
			}
			_, err := gvisorContainerizer.Run(logger, "app-handle", request, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())
			_, _, actual, _ := fakeOCIRuntime.ExecArgsForCall(0)
			request.Image = garden.ImageRef{}
			request.BindMounts = nil
			Expect(actual).To(Equal(request))
		})
	})

	Describe("BindMount compatibility validation", func() {
		var proxySpec garden.ProcessSpec

		BeforeEach(func() {
			proxySpec = garden.ProcessSpec{
				Path:  "/etc/cf-assets/envoy/envoy",
				Image: garden.ImageRef{URI: "preloaded:sidecar"},
				BindMounts: []garden.BindMount{
					{SrcPath: "/host/envoy-binary", DstPath: "/etc/cf-assets/envoy", Mode: garden.BindMountModeRO},
					{SrcPath: "/host/envoy-config", DstPath: "/etc/cf-assets/envoy_config", Mode: garden.BindMountModeRO},
				},
			}
		})

		It("gVisor: clears identical BindMounts after validating their parent mounts", func() {
			fakeProcess := new(gardenfakes.FakeProcess)
			fakeOCIRuntime.ExecReturns(fakeProcess, nil)

			_, err := gvisorContainerizer.Run(logger, "app-handle", proxySpec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)
			Expect(execSpec.BindMounts).To(BeNil(),
				"validated exec reuses the identical existing parent mounts")
			Expect(execSpec.Image).To(Equal(garden.ImageRef{}))
			Expect(execSpec.Path).To(Equal("/etc/cf-assets/envoy/envoy"))
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
		})

		It("stock runc: BindMounts are forwarded to the pea creator", func() {
			fakeProcess := new(gardenfakes.FakeProcess)
			fakePeaCreator.CreatePeaReturns(fakeProcess, nil)

			_, err := stockContainerizer.Run(logger, "app-handle", proxySpec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			_, peaSpec, _, _ := fakePeaCreator.CreatePeaArgsForCall(0)
			Expect(peaSpec.BindMounts).To(HaveLen(2),
				"stock runc must forward BindMounts to the pea creator")
			Expect(peaSpec.BindMounts[0].SrcPath).To(Equal("/host/envoy-binary"))
			Expect(peaSpec.BindMounts[1].DstPath).To(Equal("/etc/cf-assets/envoy_config"))
		})
	})

	// ── Named-user resolution ────────────────────────────────────────
	// Stock runc resolves named users against the pea image rootfs via
	// PeaUsernameResolver. gVisor bypasses this (no separate rootfs) —
	// the named user must be resolvable in the parent container's rootfs
	// by the runtime's bundle-rootfs user lookup.
	Describe("Named-user resolution contract", func() {
		var namedUserSpec garden.ProcessSpec

		BeforeEach(func() {
			namedUserSpec = garden.ProcessSpec{
				Path:  "/bin/start-sidecar",
				User:  "vcap",
				Image: garden.ImageRef{URI: "preloaded:sidecar"},
			}
		})

		It("gVisor: named user is forwarded to exec without pea resolver", func() {
			fakeProcess := new(gardenfakes.FakeProcess)
			fakeOCIRuntime.ExecReturns(fakeProcess, nil)
			namedUserSpec.Path = "/etc/cf-assets/sidecar/start-sidecar"
			namedUserSpec.BindMounts = []garden.BindMount{
				{SrcPath: "/host/sidecar", DstPath: "/etc/cf-assets/sidecar", Mode: garden.BindMountModeRO},
			}

			_, err := gvisorContainerizer.Run(logger, "app-handle", namedUserSpec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero(),
				"gVisor must NOT use PeaUsernameResolver — there is no separate pea rootfs")

			_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)
			Expect(execSpec.Path).To(Equal("/etc/cf-assets/sidecar/start-sidecar"))
			Expect(execSpec.User).To(Equal("vcap"),
				"named user must be forwarded to exec for bundle-rootfs resolution by the runtime")
		})

		It("stock runc: named user is resolved through PeaUsernameResolver", func() {
			fakePeaUsernameResolver.ResolveUserReturns(1000, 1000, nil)
			fakeProcess := new(gardenfakes.FakeProcess)
			fakePeaCreator.CreatePeaReturns(fakeProcess, nil)

			_, err := stockContainerizer.Run(logger, "app-handle", namedUserSpec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(Equal(1))
			_, _, resolvedImage, resolvedUser := fakePeaUsernameResolver.ResolveUserArgsForCall(0)
			Expect(resolvedImage.URI).To(Equal("preloaded:sidecar"),
				"resolver must receive the pea image for rootfs lookup")
			Expect(resolvedUser).To(Equal("vcap"))

			_, peaSpec, _, _ := fakePeaCreator.CreatePeaArgsForCall(0)
			Expect(peaSpec.User).To(Equal("1000:1000"),
				"resolved uid:gid must replace the named user for pea creation")
		})

		Context("user formats", func() {
			It("gVisor: empty user is forwarded as-is", func() {
				spec := garden.ProcessSpec{
					Path:  "/etc/cf-assets/envoy/envoy",
					Image: garden.ImageRef{URI: "preloaded:envoy"},
				}
				fakeOCIRuntime.ExecReturns(new(gardenfakes.FakeProcess), nil)

				_, err := gvisorContainerizer.Run(logger, "app-handle", spec, garden.ProcessIO{})
				Expect(err).NotTo(HaveOccurred())

				_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)
				Expect(execSpec.User).To(BeEmpty(),
					"empty user must be forwarded as-is for runtime default handling")
			})

			It("gVisor: uid:gid user is forwarded as-is", func() {
				spec := garden.ProcessSpec{
					Path:  "/etc/cf-assets/diego-sshd/diego-sshd",
					User:  "1000:1000",
					Image: garden.ImageRef{URI: "preloaded:ssh"},
				}
				fakeOCIRuntime.ExecReturns(new(gardenfakes.FakeProcess), nil)

				_, err := gvisorContainerizer.Run(logger, "app-handle", spec, garden.ProcessIO{})
				Expect(err).NotTo(HaveOccurred())

				_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)
				Expect(execSpec.User).To(Equal("1000:1000"),
					"numeric uid:gid must be forwarded unchanged")
				Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero())
			})

			It("stock runc: uid:gid user skips resolver", func() {
				spec := garden.ProcessSpec{
					Path:  "/bin/sshd",
					User:  "1000:1000",
					Image: garden.ImageRef{URI: "preloaded:ssh"},
				}
				fakePeaCreator.CreatePeaReturns(new(gardenfakes.FakeProcess), nil)

				_, err := stockContainerizer.Run(logger, "app-handle", spec, garden.ProcessIO{})
				Expect(err).NotTo(HaveOccurred())

				Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero(),
					"uid:gid format does not need resolution")
			})
		})
	})

	// CF caller fixtures verify validated forwarding against parent mount metadata.
	Describe("CF caller patterns", func() {
		BeforeEach(func() {
			fakeOCIRuntime.ExecReturns(new(gardenfakes.FakeProcess), nil)
		})

		It("healthcheck: path, user, env, and IO preserved", func() {
			spec := garden.ProcessSpec{
				ID:                      "healthcheck-process",
				Path:                    "/etc/cf-assets/healthcheck/healthcheck",
				Args:                    []string{"-port=8080"},
				User:                    "root",
				Dir:                     "/home/vcap",
				Env:                     []string{"PORT=8080"},
				Image:                   garden.ImageRef{URI: "preloaded:cflinuxfs4"},
				OverrideContainerLimits: &garden.ProcessLimits{},
				BindMounts: []garden.BindMount{
					{SrcPath: "/host/healthcheck", DstPath: "/etc/cf-assets/healthcheck"},
				},
			}
			originalSpec := spec
			originalSpec.Args = append([]string(nil), spec.Args...)
			originalSpec.Env = append([]string(nil), spec.Env...)
			originalSpec.BindMounts = append([]garden.BindMount(nil), spec.BindMounts...)
			originalLimits := *spec.OverrideContainerLimits
			stdin := bytes.NewBufferString("healthcheck-input")
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			pio := garden.ProcessIO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

			_, err := gvisorContainerizer.Run(logger, "app-1", spec, pio)
			Expect(err).NotTo(HaveOccurred())

			_, handle, execSpec, execIO := fakeOCIRuntime.ExecArgsForCall(0)
			Expect(handle).To(Equal("app-1"))
			Expect(execSpec.Path).To(Equal("/etc/cf-assets/healthcheck/healthcheck"))
			Expect(execSpec.User).To(Equal("root"))
			Expect(execSpec.Env).To(Equal([]string{"PORT=8080"}))
			Expect(execSpec.Image).To(Equal(garden.ImageRef{}))
			Expect(execSpec.BindMounts).To(BeNil())
			expectedExecSpec := originalSpec
			expectedExecSpec.Image = garden.ImageRef{}
			expectedExecSpec.BindMounts = nil
			Expect(execSpec).To(Equal(expectedExecSpec))
			Expect(execSpec.OverrideContainerLimits).To(BeIdenticalTo(spec.OverrideContainerLimits))
			Expect(*spec.OverrideContainerLimits).To(Equal(originalLimits))
			Expect(spec).To(Equal(originalSpec))
			Expect(execIO.Stdin).To(BeIdenticalTo(stdin))
			Expect(execIO.Stdout).To(Equal(stdout))
			Expect(execIO.Stderr).To(Equal(stderr))
			Expect(fakeOCIRuntime.ExecCallCount()).To(Equal(1))
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
			Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero())
		})

		It("container proxy (envoy): path and env preserved, empty user forwarded", func() {
			spec := garden.ProcessSpec{
				Path:  "/etc/cf-assets/envoy/envoy",
				Env:   []string{"ENVOY_UID=1000", "DRAIN_TIME=900"},
				Image: garden.ImageRef{URI: "preloaded:sidecar"},
				BindMounts: []garden.BindMount{
					{SrcPath: "/host/envoy-binary", DstPath: "/etc/cf-assets/envoy"},
					{SrcPath: "/host/envoy-config", DstPath: "/etc/cf-assets/envoy_config"},
				},
			}

			_, err := gvisorContainerizer.Run(logger, "app-2", spec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)
			Expect(execSpec.Path).To(Equal("/etc/cf-assets/envoy/envoy"))
			Expect(execSpec.User).To(BeEmpty())
			Expect(execSpec.Env).To(ContainElement("ENVOY_UID=1000"))
		})

		It("SSH daemon: path and named user preserved", func() {
			spec := garden.ProcessSpec{
				Path:  "/etc/cf-assets/diego-sshd/diego-sshd",
				User:  "vcap",
				Image: garden.ImageRef{URI: "preloaded:sshd"},
				BindMounts: []garden.BindMount{
					{SrcPath: "/host/diego-sshd", DstPath: "/etc/cf-assets/diego-sshd"},
				},
			}

			_, err := gvisorContainerizer.Run(logger, "app-3", spec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)
			Expect(execSpec.Path).To(Equal("/etc/cf-assets/diego-sshd/diego-sshd"))
			Expect(execSpec.User).To(Equal("vcap"))
		})

		It("plain exec (no image): dispatched directly, not through pea path", func() {
			spec := garden.ProcessSpec{
				Path: "/bin/bash",
				User: "vcap",
				Dir:  "/home/vcap",
			}

			_, err := gvisorContainerizer.Run(logger, "app-4", spec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeOCIRuntime.ExecCallCount()).To(Equal(1))
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
			_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)
			Expect(execSpec.Path).To(Equal("/bin/bash"))
			Expect(execSpec.User).To(Equal("vcap"))
			Expect(execSpec.Dir).To(Equal("/home/vcap"))
			Expect(fakeOCIRuntime.BundleInfoCallCount()).To(BeZero())
		})

		It("reuses an identical read-write source mount without adding a mount", func() {
			spec := garden.ProcessSpec{
				Path:  "/etc/cf-assets/healthcheck/healthcheck",
				Image: garden.ImageRef{URI: "preloaded:cflinuxfs4"},
				BindMounts: []garden.BindMount{
					{SrcPath: "/host/healthcheck", DstPath: "/etc/cf-assets/healthcheck", Mode: garden.BindMountModeRO},
					{SrcPath: "/host/source", DstPath: "/etc/cf-assets/source", Mode: garden.BindMountModeRW},
				},
			}
			_, err := gvisorContainerizer.Run(logger, "app-source", spec, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeOCIRuntime.ExecCallCount()).To(Equal(1))
			_, _, execSpec, _ := fakeOCIRuntime.ExecArgsForCall(0)
			Expect(execSpec.Path).To(Equal(spec.Path))
			Expect(execSpec.BindMounts).To(BeNil())
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
		})
	})

	DescribeTable("rejects incompatible pea requests before resolving users or starting processes",
		func(mutate func(*garden.ProcessSpec, *goci.Bndl), message string) {
			spec := garden.ProcessSpec{
				Path:  "/etc/cf-assets/healthcheck/healthcheck",
				User:  "vcap",
				Image: garden.ImageRef{URI: "preloaded:cflinuxfs4"},
				BindMounts: []garden.BindMount{
					{SrcPath: "/host/healthcheck", DstPath: "/etc/cf-assets/healthcheck", Mode: garden.BindMountModeRO},
				},
			}
			mutate(&spec, &parentBundle)
			fakeOCIRuntime.BundleInfoReturns("parent-bundle", parentBundle, nil)

			process, err := gvisorContainerizer.Run(logger, "app-rejected", spec, garden.ProcessIO{})
			Expect(err).To(MatchError(ContainSubstring("unsupported pea exec")))
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(process).To(BeNil())
			Expect(fakeOCIRuntime.ExecCallCount()).To(BeZero())
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
			Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero())
		},
		Entry("a distinct image", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Image.URI = "docker:///image-only"
		}, "only preloaded CF asset"),
		Entry("an empty preloaded image name", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Image.URI = "preloaded:"
		}, "only preloaded CF asset"),
		Entry("image username credentials", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Image.Username = "registry-user"
		}, "only preloaded CF asset"),
		Entry("image password credentials", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Image.Password = "test-password"
		}, "only preloaded CF asset"),
		Entry("username credentials with an empty URI", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Image = garden.ImageRef{Username: "registry-user"}
		}, "only preloaded CF asset"),
		Entry("password credentials with an empty URI", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Image = garden.ImageRef{Password: "test-password"}
		}, "only preloaded CF asset"),
		Entry("a binary available only in the pea image", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Path = "/bin/image-only"
		}, "executable must be an existing parent CF asset"),
		Entry("an arbitrary shell", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Path, spec.Args = "sh", []string{"-c", "echo arbitrary"}
		}, "executable must be an existing parent CF asset"),
		Entry("an injected Envoy drain value", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Path, spec.Args = "sh", []string{"-c", "trap 'kill -9 0' TERM; /etc/cf-assets/envoy/envoy -c /etc/cf-assets/envoy_config/envoy.yaml --drain-time-s 0; id --log-level critical& pid=$!; wait $pid"}
		}, "executable must be an existing parent CF asset"),
		Entry("the Envoy wrapper without its config mount", func(spec *garden.ProcessSpec, parent *goci.Bndl) {
			spec.Path, spec.Args = "sh", []string{"-c", "trap 'kill -9 0' TERM; /etc/cf-assets/envoy/envoy -c /etc/cf-assets/envoy_config/envoy.yaml --drain-time-s 0 --log-level critical& pid=$!; wait $pid"}
			parent.Spec.Mounts = parent.Spec.Mounts[:2]
		}, "not covered by a parent CF asset bind mount"),
		Entry("a noncanonical packaged rootfs", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Image.URI = "/var/vcap/packages/other/../cflinuxfs4/rootfs.tar"
		}, "only preloaded CF asset"),
		Entry("an arbitrary host rootfs path", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Image.URI = "/tmp/image-only/rootfs.tar"
		}, "only preloaded CF asset"),
		Entry("a relative executable path", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Path = "etc/cf-assets/healthcheck/healthcheck"
		}, "executable must be an existing parent CF asset"),
		Entry("executable path traversal", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Path = "/etc/cf-assets/healthcheck/../healthcheck/healthcheck"
		}, "executable must be an existing parent CF asset"),
		Entry("a noncanonical executable path", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Path = "/etc/cf-assets/healthcheck//healthcheck"
		}, "executable must be an existing parent CF asset"),
		Entry("a lookalike asset prefix", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Path = "/etc/cf-assets-other/healthcheck/healthcheck"
		}, "executable must be an existing parent CF asset"),
		Entry("no parent mount covering the executable", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts = nil
		}, "not covered by a parent CF asset bind mount"),
		Entry("a non-bind parent asset mount", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts[0].Type = "tmpfs"
			parent.Spec.Mounts[0].Options = []string{"ro"}
		}, "not covered by a parent CF asset bind mount"),
		Entry("a sibling path sharing the mount prefix", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.Path = "/etc/cf-assets/healthcheck-other/healthcheck"
		}, "not covered by a parent CF asset bind mount"),
		Entry("a broad parent mount outside the CF asset directory", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts[0].Destination = "/etc"
		}, "not covered by a parent CF asset bind mount"),
		Entry("a missing requested source", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].SrcPath = ""
		}, "existing absolute host mounts"),
		Entry("a source absent from the parent mount", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].SrcPath = "/host/new-healthcheck"
		}, "not identical to the parent mount"),
		Entry("a mismatched destination", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].DstPath = "/etc/cf-assets/new-healthcheck"
		}, "not identical to the parent mount"),
		Entry("a new mount after an identical parent mount", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts = append(spec.BindMounts, garden.BindMount{
				SrcPath: "/host/new-source", DstPath: "/etc/cf-assets/new-source", Mode: garden.BindMountModeRO,
			})
		}, "not identical to the parent mount"),
		Entry("read-only requested against a read-write parent mount", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts[0].Options = []string{"bind", "rw"}
		}, "not identical to the parent mount"),
		Entry("ro then rw parent options for a read-only request", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts[0].Options = []string{"bind", "ro", "rw"}
		}, "ambiguous ro and rw"),
		Entry("rw then ro parent options for a read-only request", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts[0].Options = []string{"bind", "rw", "ro"}
		}, "ambiguous ro and rw"),
		Entry("ro then rw parent options for a read-write request", func(spec *garden.ProcessSpec, parent *goci.Bndl) {
			spec.BindMounts[0].Mode = garden.BindMountModeRW
			parent.Spec.Mounts[0].Options = []string{"bind", "ro", "rw"}
		}, "ambiguous ro and rw"),
		Entry("rw then ro parent options for a read-write request", func(spec *garden.ProcessSpec, parent *goci.Bndl) {
			spec.BindMounts[0].Mode = garden.BindMountModeRW
			parent.Spec.Mounts[0].Options = []string{"bind", "rw", "ro"}
		}, "ambiguous ro and rw"),
		Entry("read-write requested against a read-only parent mount", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].Mode = garden.BindMountModeRW
		}, "not identical to the parent mount"),
		Entry("a container-origin mount", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].Origin = garden.BindMountOriginContainer
		}, "existing absolute host mounts"),
		Entry("a relative mount source", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].SrcPath = "host/healthcheck"
		}, "existing absolute host mounts"),
		Entry("mount source traversal", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].SrcPath = "/host/other/../healthcheck"
		}, "existing absolute host mounts"),
		Entry("a relative mount destination", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].DstPath = "etc/cf-assets/healthcheck"
		}, "existing absolute host mounts"),
		Entry("mount destination traversal", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.BindMounts[0].DstPath = "/etc/cf-assets/other/../healthcheck"
		}, "existing absolute host mounts"),
		Entry("a noncanonical parent mount destination", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts[0].Destination = "/etc/cf-assets/other/../healthcheck"
		}, "noncanonical parent mount overlaps"),
		Entry("an alias replacement after a valid parent mount", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts = append(parent.Spec.Mounts, specs.Mount{
				Source: "/host/replacement", Destination: "/etc/cf-assets/healthcheck/.", Type: "bind", Options: []string{"rw"},
			})
		}, "noncanonical parent mount overlaps"),
		Entry("an alias shadowing a requested non-executable mount", func(spec *garden.ProcessSpec, parent *goci.Bndl) {
			spec.BindMounts = append(spec.BindMounts, garden.BindMount{SrcPath: "/host/envoy-config", DstPath: "/etc/cf-assets/envoy_config", Mode: garden.BindMountModeRO})
			parent.Spec.Mounts = append(parent.Spec.Mounts, specs.Mount{
				Source: "/host/replacement", Destination: "/etc/cf-assets/envoy_config/.", Type: "bind", Options: []string{"rw"},
			})
		}, "noncanonical parent mount overlaps"),
		Entry("a non-bind mount shadowing the executable", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts = append(parent.Spec.Mounts, specs.Mount{
				Destination: "/etc/cf-assets/healthcheck/healthcheck", Type: "tmpfs",
			})
		}, "not covered by a parent CF asset bind mount"),
		Entry("a later parent mount shadowing the requested source", func(_ *garden.ProcessSpec, parent *goci.Bndl) {
			parent.Spec.Mounts = append(parent.Spec.Mounts, specs.Mount{
				Source: "/host/replacement", Destination: "/etc/cf-assets/healthcheck", Type: "bind", Options: []string{"ro"},
			})
		}, "not identical to the parent mount"),
		Entry("nonzero CPU weight", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.OverrideContainerLimits = &garden.ProcessLimits{CPU: garden.CPULimits{Weight: 1}}
		}, "independent CPU or memory limits"),
		Entry("nonzero CPU shares", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.OverrideContainerLimits = &garden.ProcessLimits{CPU: garden.CPULimits{LimitInShares: 1}}
		}, "independent CPU or memory limits"),
		Entry("nonzero memory limits", func(spec *garden.ProcessSpec, _ *goci.Bndl) {
			spec.OverrideContainerLimits = &garden.ProcessLimits{Memory: garden.MemoryLimits{LimitInBytes: 1}}
		}, "independent CPU or memory limits"),
	)

	// ── Stock-runc dispatch preserved ────────────────────────────────
	// Stock runc must NOT be affected by the exec translation. All pea
	// semantics (Image, BindMounts, PeaCreator) remain fully operational.
	Describe("Stock-runc pea dispatch preserved", func() {
		It("creates a real pea for every image-bearing request", func() {
			fakePeaCreator.CreatePeaReturns(new(gardenfakes.FakeProcess), nil)

			specs := []garden.ProcessSpec{
				{Path: "/bin/healthcheck", User: "root", Image: garden.ImageRef{URI: "img1"}},
				{Path: "/bin/envoy", Image: garden.ImageRef{URI: "img2"}, BindMounts: []garden.BindMount{{SrcPath: "/a", DstPath: "/b"}}},
				{Path: "/bin/sshd", User: "vcap", Image: garden.ImageRef{URI: "img3"}},
			}

			for i, spec := range specs {
				_, err := stockContainerizer.Run(logger, "app-stock", spec, garden.ProcessIO{})
				Expect(err).NotTo(HaveOccurred())
				Expect(fakePeaCreator.CreatePeaCallCount()).To(Equal(i + 1))
			}
			Expect(fakeOCIRuntime.ExecCallCount()).To(BeZero(),
				"stock runc must NOT exec any image-bearing request")
		})
	})

	// ── Error propagation ────────────────────────────────────────────
	Describe("Error propagation", func() {
		It("gVisor: wraps a parent bundle error without losing its identity", func() {
			bundleErr := errors.New("parent bundle lookup failed")
			fakeOCIRuntime.BundleInfoReturns("", goci.Bndl{}, bundleErr)
			process, err := gvisorContainerizer.Run(logger, "app-bundle-error", garden.ProcessSpec{
				Path: "/etc/cf-assets/healthcheck/healthcheck", User: "vcap",
				Image: garden.ImageRef{URI: "preloaded:cflinuxfs4"},
			}, garden.ProcessIO{})
			Expect(err).To(MatchError(ContainSubstring("validate pea exec parent bundle")))
			Expect(errors.Is(err, bundleErr)).To(BeTrue())
			Expect(process).To(BeNil())
			Expect(fakeOCIRuntime.BundleInfoCallCount()).To(Equal(1))
			_, handle := fakeOCIRuntime.BundleInfoArgsForCall(0)
			Expect(handle).To(Equal("app-bundle-error"))
			Expect(fakeOCIRuntime.ExecCallCount()).To(BeZero())
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
			Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero())
		})

		It("gVisor: runtime exec error surfaces to the caller", func() {
			fakeOCIRuntime.ExecReturns(nil, errors.New("sandbox exec failed"))
			spec := garden.ProcessSpec{
				Path:  "/etc/cf-assets/healthcheck/healthcheck",
				Image: garden.ImageRef{URI: "preloaded:cflinuxfs4"},
			}

			_, err := gvisorContainerizer.Run(logger, "app-err", spec, garden.ProcessIO{})
			Expect(err).To(MatchError("sandbox exec failed"))
		})

		It("stock runc: pea creation error surfaces to the caller", func() {
			fakePeaCreator.CreatePeaReturns(nil, errors.New("volume fetch failed"))
			spec := garden.ProcessSpec{
				Path:  "/bin/healthcheck",
				Image: garden.ImageRef{URI: "preloaded:healthcheck"},
			}

			_, err := stockContainerizer.Run(logger, "app-err", spec, garden.ProcessIO{})
			Expect(err).To(MatchError("volume fetch failed"))
		})

		It("stock runc: username resolution error prevents pea creation", func() {
			fakePeaUsernameResolver.ResolveUserReturns(0, 0, errors.New("user not found"))
			spec := garden.ProcessSpec{
				Path:  "/bin/sidecar",
				User:  "nonexistent",
				Image: garden.ImageRef{URI: "preloaded:sidecar"},
			}

			_, err := stockContainerizer.Run(logger, "app-err", spec, garden.ProcessIO{})
			Expect(err).To(MatchError("user not found"))
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero(),
				"pea must not be created if username resolution fails")
		})

		It("bind mounts without image are rejected in both modes", func() {
			spec := garden.ProcessSpec{
				Path: "/bin/cmd",
				BindMounts: []garden.BindMount{
					{SrcPath: "/src", DstPath: "/dst"},
				},
			}

			_, err := gvisorContainerizer.Run(logger, "app-err", spec, garden.ProcessIO{})
			Expect(err).To(MatchError(ContainSubstring("bind mounts and no image")))
			Expect(fakeOCIRuntime.ExecCallCount()).To(BeZero())
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
			Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero())

			_, err = stockContainerizer.Run(logger, "app-err", spec, garden.ProcessIO{})
			Expect(err).To(MatchError(ContainSubstring("bind mounts and no image")))
			Expect(fakeOCIRuntime.ExecCallCount()).To(BeZero())
			Expect(fakePeaCreator.CreatePeaCallCount()).To(BeZero())
			Expect(fakePeaUsernameResolver.ResolveUserCallCount()).To(BeZero())
		})
	})
})
