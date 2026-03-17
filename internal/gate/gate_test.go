package gate

import "testing"

func TestGateValidate_ValidGeneratedKey(t *testing.T) {
	g, err := New(0)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	key := g.Key()
	ok, err := g.Validate(key)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !ok {
		t.Fatal("expected generated key to be valid")
	}
	if g.UseCount() != 1 {
		t.Fatalf("expected useCount = 1, got %d", g.UseCount())
	}
}

func TestGateValidate_InvalidKey(t *testing.T) {
	g, err := New(0)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ok, err := g.Validate("definitely-wrong-key")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if ok {
		t.Fatal("expected invalid key to be rejected")
	}
	if g.UseCount() != 0 {
		t.Fatalf("expected useCount = 0 after invalid key, got %d", g.UseCount())
	}
}

func TestGateValidate_EmptyKey(t *testing.T) {
	g, err := New(0)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ok, err := g.Validate("")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if ok {
		t.Fatal("expected empty key to be rejected")
	}
	if g.UseCount() != 0 {
		t.Fatalf("expected useCount = 0 after empty key, got %d", g.UseCount())
	}
}

func TestGateValidate_RotatesAfterMaxUses(t *testing.T) {
	g, err := New(2)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	oldKey := g.Key()

	ok, err := g.Validate(oldKey)
	if err != nil {
		t.Fatalf("first Validate() error = %v", err)
	}
	if !ok {
		t.Fatal("expected first validation to succeed")
	}
	if g.UseCount() != 1 {
		t.Fatalf("expected useCount = 1 after first use, got %d", g.UseCount())
	}

	ok, err = g.Validate(oldKey)
	if err != nil {
		t.Fatalf("second Validate() error = %v", err)
	}
	if !ok {
		t.Fatal("expected second validation to succeed")
	}

	newKey := g.Key()
	if newKey == oldKey {
		t.Fatal("expected gate key to rotate after reaching maxUses")
	}
	if g.UseCount() != 0 {
		t.Fatalf("expected useCount reset to 0 after rotation, got %d", g.UseCount())
	}
}
