package users

import (
	"errors"
	"fmt"
	"io"
	"os"

	securejoin "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"github.com/moby/sys/user"
)

const (
	DefaultHome string = "/"
)

// openIdentityFile resolves path inside root, so absolute or escaping symlinks
// (including via /proc/<pid>/root) cannot reach Guardian's host filesystem.
// Non-regular files are rejected before any blocking open.
func openIdentityFile(root, path string) (io.ReadCloser, error) {
	handle, err := securejoin.OpenInRoot(root, path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	// Reopen the pinned O_PATH inode rather than its pathname.
	file, err := securejoin.Reopen(handle, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func LookupUser(rootFsPath, userName string) (*ExecUser, error) {
	passwd, err := openIdentityFile(rootFsPath, "/etc/passwd")
	if err != nil {
		return nil, err
	}
	if passwd == nil && userName == "root" {
		return &ExecUser{Uid: DefaultUID, Gid: DefaultGID, Home: DefaultHome, Sgids: []int{}}, nil
	}
	group, err := openIdentityFile(rootFsPath, "/etc/group")
	if err != nil {
		if passwd != nil {
			passwd.Close()
		}
		return nil, err
	}

	var passwdReader, groupReader io.Reader
	if passwd != nil {
		defer passwd.Close()
		passwdReader = passwd
	}
	if group != nil {
		defer group.Close()
		groupReader = group
	}

	defaultUser := &user.ExecUser{Uid: DefaultUID, Gid: DefaultGID, Home: DefaultHome}
	execUser, err := user.GetExecUser(userName, defaultUser, passwdReader, groupReader)
	if err != nil {
		return nil, err
	}

	return &ExecUser{Uid: execUser.Uid, Gid: execUser.Gid, Home: execUser.Home, Sgids: execUser.Sgids}, nil
}
