package cli

import "testing"

func TestDatabaseFromComposeConfig(t *testing.T) {
	for _, db := range []string{"thoth", "thoth_restore_20261012_120000"} {
		got, err := databaseFromConfig([]byte(`{"services":{"app":{"environment":{"THOTH_DB_NAME":"` + db + `"}}}}`))
		if err != nil || got != db {
			t.Fatalf("database = %q, %v; want %q", got, err, db)
		}
	}
	for _, invalid := range []string{`{`, `{}`, `{"services":{"db":{"environment":{"POSTGRES_DB":"wrong"}}}}`} {
		if _, err := databaseFromConfig([]byte(invalid)); err == nil {
			t.Fatalf("accepted invalid active database config %s", invalid)
		}
	}
}
