package mode

import "testing"

func TestRoleCatalogCoversBuiltins(t *testing.T) {
	roles := RoleCatalog()
	if len(roles) != 7 {
		t.Fatalf("roles = %d, want 7", len(roles))
	}
	for _, role := range roles {
		if role.Prompt == "" || len(role.Tools) == 0 || len(role.Modes) == 0 {
			t.Fatalf("incomplete role: %+v", role)
		}
	}
	if len(MainCatalog()) != 5 {
		t.Fatalf("main prompts = %d, want 5", len(MainCatalog()))
	}
}
