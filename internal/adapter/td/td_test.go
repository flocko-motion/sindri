package td

import "testing"

func TestParseAndSort(t *testing.T) {
	data := []byte(`[
		{"id":"td-1","status":"closed","updated_at":"2026-05-27T10:00:00Z"},
		{"id":"td-2","status":"open","priority":"P0"},
		{"id":"td-3","status":"in_progress","updated_at":"2026-05-28T10:00:00Z"},
		{"id":"td-4","status":"in_review","updated_at":"2026-05-28T12:00:00Z"}
	]`)
	got, err := parseAndSort(data)
	if err != nil {
		t.Fatal(err)
	}
	// Expected order: open first, then active (recent first), then closed.
	wantOrder := []string{"td-2", "td-4", "td-3", "td-1"}
	for i, w := range wantOrder {
		if got[i].ID != w {
			t.Errorf("position %d: got %s want %s", i, got[i].ID, w)
		}
	}
}
