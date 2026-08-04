package update

import "testing"

func TestCheckRequiresRepository(t *testing.T) {
	if _, err := Check(t.Context(), ""); err == nil {
		t.Fatal("expected an empty repository to fail")
	}
}
