package db

import (
	"testing"
)

func TestLoadMasterDataCompleteness(t *testing.T) {
	md, err := LoadMasterData("../../data")
	if err != nil {
		t.Fatalf("LoadMasterData failed: %v", err)
	}
	if len(md.Weapons) != 57 {
		t.Fatalf("expected 57 weapons, got %d", len(md.Weapons))
	}
	if len(md.StartItems) != 36 {
		t.Fatalf("expected 36 starter items, got %d", len(md.StartItems))
	}
	if len(md.AreaItems) != 1373 {
		t.Fatalf("expected 1373 area items, got %d", len(md.AreaItems))
	}
	if len(md.Recipes) != 170 {
		t.Fatalf("expected 170 recipes, got %d", len(md.Recipes))
	}
	if len(md.NPCs) != 2 {
		t.Fatalf("expected 2 NPCs, got %d", len(md.NPCs))
	}
	for i, r := range md.Recipes {
		if r.Ingredient1 == "" || r.Ingredient2 == "" || r.ResultName == "" || r.ResultKind == "" {
			t.Fatalf("recipe %d has empty field: %+v", i, r)
		}
	}
}
