package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateWindowsFilesRejectInheritedReaders(t *testing.T) {
	root := t.TempDir()
	// A public parent reproduces installation/backup into a shared directory.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "secret")
	if err = exclusive(file, []byte("synthetic-test-secret")); err != nil {
		t.Fatal(err)
	}
	if err = save(file, []byte("replacement-synthetic-secret")); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(file + "*")
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		got, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		// The protected descriptor must grant only the current local owner.
		want := "D:P(A;;FA;;;" + user.User.Sid.String() + ")"
		// Windows renders well-known SIDs as aliases (e.g. LA for the local
		// Administrator on hosted runners). Canonicalize both descriptors.
		expected, err := windows.SecurityDescriptorFromString(want)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Replace(got.String(), "D:PAI", "D:P", 1) != expected.String() {
			t.Fatalf("unexpected private ACL: %s", got.String())
		}
	}
	dir := filepath.Join(root, ".secrets")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = privatePath(dir, true); err != nil {
		t.Fatal(err)
	}
}
