package cli

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Install the private DACL atomically at creation: an inherited reader must not
// be able to open an empty backup and retain a handle while we write its contents.
func privateCreate(path string) (*os.File, error) {
	sd, err := privateDescriptor(false)
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attr := windows.SecurityAttributes{SecurityDescriptor: sd}
	attr.Length = uint32(unsafe.Sizeof(attr))
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, &attr, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

// Go file modes do not set Windows DACLs. Protect secrets before writing bytes,
// even when the installation or backup directory inherits broad read access.
func privatePath(path string, directory bool) error {
	sd, err := privateDescriptor(directory)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
}

func privateDescriptor(directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	flags := ""
	if directory {
		flags = "OICI"
	}
	return windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;" + user.User.Sid.String() + ")")
}
