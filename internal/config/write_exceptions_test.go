package config

import "testing"

func TestWriteExceptionsMatchIdentityAndOperationScopes(t *testing.T) {
	cfg := File{}

	exception := WriteException{Name: "gmail-send", Account: "a@example.com", Client: "work", Service: "gmail", Operation: "send"}
	if err := UpsertWriteException(&cfg, exception, false); err != nil {
		t.Fatal(err)
	}

	if !AllowsWrite(cfg.WriteExceptions, "a@example.com", "work", "gmail", "send", "") {
		t.Fatal("scoped exception did not allow matching operation")
	}

	if AllowsWrite(cfg.WriteExceptions, "other@example.com", "work", "gmail", "send", "") {
		t.Fatal("scoped exception allowed a different account")
	}

	if AllowsWrite(cfg.WriteExceptions, "a@example.com", "work", "gmail", "delete", "") {
		t.Fatal("scoped exception allowed a different operation")
	}

	if err := DeleteWriteException(&cfg, "gmail-send"); err != nil {
		t.Fatal(err)
	}

	if len(cfg.WriteExceptions) != 0 {
		t.Fatalf("exceptions = %#v, want empty", cfg.WriteExceptions)
	}
}
