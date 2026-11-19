//go:build windows

package fsx

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	fileRenameInfoEx          = 22 // FILE_INFO_BY_HANDLE_CLASS.FileRenameInfoEx (Windows 10 1709+)
	renameReplaceIfExists     = 0x1
	renamePosixSemantics      = 0x2
	renameIgnoreReadOnlyAttr  = 0x40
	deleteAccess              = 0x00010000
	renameInfoNameFieldOffset = 20 // Flags(4) + padding(4) + RootDirectory(8) + FileNameLength(4)
)

// replace renames tmp over dst. It first asks for POSIX rename semantics, which lets the
// replacement happen while other processes still have dst open (they keep reading the old
// content). The classic rename fails with "Access is denied" in that situation. If the POSIX
// form is not available (older Windows, some filesystems) it falls back to the classic one.
func replace(tmp, dst string) error {
	if err := posixRename(tmp, dst); err == nil {
		return nil
	}
	return os.Rename(tmp, dst)
}

func posixRename(tmp, dst string) error {
	src, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(src, deleteAccess|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)

	name, err := windows.UTF16FromString(dst) // includes the terminating NUL
	if err != nil {
		return err
	}
	nameBytes := (len(name) - 1) * 2
	buf := make([]byte, renameInfoNameFieldOffset+len(name)*2)
	*(*uint32)(unsafe.Pointer(&buf[0])) = renameReplaceIfExists | renamePosixSemantics | renameIgnoreReadOnlyAttr
	*(*uint32)(unsafe.Pointer(&buf[16])) = uint32(nameBytes)
	copy(buf[renameInfoNameFieldOffset:], unsafe.Slice((*byte)(unsafe.Pointer(&name[0])), len(name)*2))
	return windows.SetFileInformationByHandle(h, fileRenameInfoEx, &buf[0], uint32(len(buf)))
}
