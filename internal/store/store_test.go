package store

import "testing"

func TestRepositoryIdentity(t *testing.T) {
	for _, v := range []string{"https://github.com/Owner/Repo.git", "git@github.com:Owner/Repo.git", "ssh://git@github.com/Owner/Repo"} {
		got, e := CanonicalRepository(v)
		if e != nil || got != "github.com/owner/repo" {
			t.Fatal(v, got, e)
		}
	}
	for _, v := range []string{"project", "/home/x/project", "https://example.com/x?token=secret", "local:folder"} {
		if _, e := CanonicalRepository(v); e == nil {
			t.Fatal("accepted ambiguous identity", v)
		}
	}
}
