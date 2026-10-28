package users

import (
	"errors"
	"io"
	"os"

	securejoin "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"github.com/moby/sys/user"
)

func fileReader(file *os.File) io.Reader {
	if file == nil {
		return nil
	}
	return file
}

// LookupImageUser reads identity files inside a prepared image root. Absolute
// image symlinks must not resolve against Guardian's host filesystem.
func LookupImageUser(root, name string) (*ExecUser, error) {
	passwd, err := securejoin.OpenInRoot(root, "/etc/passwd")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if passwd != nil {
		defer passwd.Close()
	}
	group, err := securejoin.OpenInRoot(root, "/etc/group")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if group != nil {
		defer group.Close()
	}
	if passwd == nil && name == "root" {
		return &ExecUser{Uid: DefaultUID, Gid: DefaultGID, Home: DefaultHome}, nil
	}
	// Avoid a typed nil *os.File as an io.Reader when an image omits a file.
	var passwdReader, groupReader = fileReader(passwd), fileReader(group)
	resolved, err := user.GetExecUser(name, &user.ExecUser{Uid: DefaultUID, Gid: DefaultGID, Home: DefaultHome}, passwdReader, groupReader)
	if err != nil {
		return nil, err
	}
	return &ExecUser{Uid: resolved.Uid, Gid: resolved.Gid, Home: resolved.Home, Sgids: resolved.Sgids}, nil
}
