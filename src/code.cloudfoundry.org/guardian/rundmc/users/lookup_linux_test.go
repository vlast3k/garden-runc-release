package users_test

import (
	"os"
	"path/filepath"
	"syscall"

	"code.cloudfoundry.org/guardian/rundmc/users"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LookupUser", func() {
	Context("when we try to get the UID, GID and HOME of a username", func() {
		var (
			rootFsPath     string
			uidOfTheBeast  = 666
			gidOfTheBeast  = 777
			homeOfTheBeast = "/home/fieryunderworld"
		)

		createPasswdFile := func() error {
			return os.WriteFile(filepath.Join(rootFsPath, "etc", "passwd"), []byte(
				`_lda:*:211:211:Local Delivery Agent:/var/empty:/usr/bin/false
_cvmsroot:*:212:212:CVMS Root:/var/empty:/usr/bin/false
_usbmuxd:*:213:213:iPhone OS Device Helper:/var/db/lockdown:/usr/bin/false
devil:*:666:777:Beelzebub:/home/fieryunderworld:/usr/bin/false
_dovecot:*:214:6:Dovecot Administrator:/var/empty:/usr/bin/false
vcap:*:1000:1000:VCAP:/home/vcap/:/bin/sh`,
			), 0777)
		}

		createGroupFile := func() error {
			return os.WriteFile(filepath.Join(rootFsPath, "etc", "group"), []byte(
				`root:x:0:
daemon:x:1:
bin:x:2:
vcap:x:1000:
another:x:4:vcap`,
			), 0777)
		}

		BeforeEach(func() {
			var err error
			rootFsPath, err = os.MkdirTemp("", "passwdtestdir")
			Expect(err).NotTo(HaveOccurred())
			Expect(os.MkdirAll(filepath.Join(rootFsPath, "etc"), 0777)).To(Succeed())
			Expect(createGroupFile()).To(Succeed())
			Expect(createPasswdFile()).To(Succeed())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(rootFsPath)).To(Succeed())
		})

		DescribeTable("user / group combinations", func(username string, expectedUid, expectedGid int, expectedHome string) {
			user, err := users.LookupUser(rootFsPath, username)
			Expect(err).ToNot(HaveOccurred())
			Expect(user.Uid).To(BeEquivalentTo(expectedUid))
			Expect(user.Gid).To(BeEquivalentTo(expectedGid))
			Expect(user.Home).To(Equal(expectedHome))
		},

			Entry("empty username", "", users.DefaultUID, users.DefaultGID, users.DefaultHome),
			Entry("username from /etc/passwd", "devil", uidOfTheBeast, gidOfTheBeast, homeOfTheBeast),
			Entry("username from /etc/passwd and groupname from /etc/group", "devil:daemon", uidOfTheBeast, 1, homeOfTheBeast),
			Entry("username from /etc/passwd and given groupid", "devil:123", uidOfTheBeast, 123, homeOfTheBeast),
			Entry("given userid and groupname from /etc/group", "666:vcap", uidOfTheBeast, 1000, homeOfTheBeast),
			Entry("given userid and groupid", "123:456", 123, 456, "/"),
			Entry("given userid that exists in /etc/passwd", "666", uidOfTheBeast, gidOfTheBeast, homeOfTheBeast),
		)

		Context("when /etc/passwd exists with no matching users", func() {
			It("returns an error", func() {
				_, err := users.LookupUser(rootFsPath, "unknownUser")
				Expect(err).To(MatchError(ContainSubstring("unable to find")))
			})
		})

		Context("when /etc/passwd exists but cannot be parsed", func() {
			BeforeEach(func() {
				senselessContents := []byte(
					`lorem ipsum dollar sit amet
					unix at the portal
					body type by letroset
					here at the epoch
					let us forget...`,
				)
				passwdPath := filepath.Join(rootFsPath, "etc", "passwd")
				Expect(os.WriteFile(passwdPath, senselessContents, 0777)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := users.LookupUser(rootFsPath, "devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find")))
			})
		})

		Context("when /etc/passwd does not exist", func() {
			BeforeEach(func() {
				Expect(os.RemoveAll(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
			})

			It("returns the default UID, GID and HOME when user 'root' is requested", func() {
				user, err := users.LookupUser(rootFsPath, "root")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(BeEquivalentTo(users.DefaultUID))
				Expect(user.Gid).To(BeEquivalentTo(users.DefaultGID))
				Expect(user.Home).To(Equal(users.DefaultHome))
			})

			It("errors when a user other than root is requested", func() {
				_, err := users.LookupUser(rootFsPath, "nobody")
				Expect(err).To(MatchError(ContainSubstring("unable to find user nobody")))
			})
		})

		Context("when /etc/group does not exist", func() {
			BeforeEach(func() {
				Expect(os.RemoveAll(filepath.Join(rootFsPath, "etc", "group"))).To(Succeed())
			})

			It("tolerates a numeric gid", func() {
				_, err := users.LookupUser(rootFsPath, "123:456")
				Expect(err).NotTo(HaveOccurred())
			})

			It("errors when a group name is requested", func() {
				_, err := users.LookupUser(rootFsPath, "123:devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find group devil")))
			})
		})

		Context("secondary groups", func() {
			It("has vcap in a secondary group", func() {
				user, err := users.LookupUser(rootFsPath, "vcap")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Gid).To(Equal(1000))
				Expect(user.Sgids).To(HaveLen(1))
				Expect(user.Sgids[0]).To(Equal(4))
			})
		})

		Context("when identity files are symlinks", func() {
			var hostDir string

			// The same absolute path holds a host decoy and, inside the rootfs,
			// the image's own file.
			writeHostAndImage := func(name, hostContents, imageContents string) string {
				hostPath := filepath.Join(hostDir, name)
				Expect(os.WriteFile(hostPath, []byte(hostContents), 0644)).To(Succeed())
				imagePath := filepath.Join(rootFsPath, hostPath)
				Expect(os.MkdirAll(filepath.Dir(imagePath), 0755)).To(Succeed())
				Expect(os.WriteFile(imagePath, []byte(imageContents), 0644)).To(Succeed())
				return hostPath
			}

			replaceWithSymlink := func(name, target string) {
				path := filepath.Join(rootFsPath, "etc", name)
				Expect(os.Remove(path)).To(Succeed())
				Expect(os.Symlink(target, path)).To(Succeed())
			}

			BeforeEach(func() {
				var err error
				hostDir, err = os.MkdirTemp("", "host-identity")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(os.RemoveAll, hostDir)
			})

			It("resolves an absolute /etc/passwd symlink inside the rootfs, not on the host", func() {
				target := writeHostAndImage("passwd",
					"childuser:x:0:0:host:/root:/bin/sh\n",
					"childuser:x:1234:4321:image:/child-home:/bin/sh\n")
				replaceWithSymlink("passwd", target)

				user, err := users.LookupUser(rootFsPath, "childuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(1234))
				Expect(user.Gid).To(Equal(4321))
				Expect(user.Home).To(Equal("/child-home"))
			})

			It("resolves an absolute /etc/group symlink inside the rootfs, not on the host", func() {
				target := writeHostAndImage("group",
					"hostroot:x:0:vcap\n",
					"imagegroup:x:4242:vcap\n")
				replaceWithSymlink("group", target)

				user, err := users.LookupUser(rootFsPath, "vcap")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Sgids).To(Equal([]int{4242}))
			})

			It("does not follow a relative symlink out of the rootfs", func() {
				hostPasswd := filepath.Join(hostDir, "passwd")
				Expect(os.WriteFile(hostPasswd, []byte("hostuser:x:0:0:host:/root:/bin/sh\n"), 0644)).To(Succeed())
				replaceWithSymlink("passwd", "../../../../../../../.."+hostPasswd)

				_, err := users.LookupUser(rootFsPath, "hostuser")
				Expect(err).To(MatchError(ContainSubstring("unable to find user hostuser")))
			})
		})

		DescribeTable("when an identity file is a FIFO", func(name string) {
			fifo := filepath.Join(rootFsPath, "etc", name)
			Expect(os.Remove(fifo)).To(Succeed())
			Expect(syscall.Mkfifo(fifo, 0644)).To(Succeed())
			// Release a reader blocked by a vulnerable implementation.
			DeferCleanup(func() {
				if f, err := os.OpenFile(fifo, os.O_RDWR, 0); err == nil {
					f.Close()
				}
			})

			result := make(chan error, 1)
			go func() {
				_, err := users.LookupUser(rootFsPath, "vcap")
				result <- err
			}()
			var err error
			Eventually(result, "5s").Should(Receive(&err))
			Expect(err).To(MatchError(ContainSubstring("not a regular file")))
		},
			Entry("passwd", "passwd"),
			Entry("group", "group"),
		)
	})
})
