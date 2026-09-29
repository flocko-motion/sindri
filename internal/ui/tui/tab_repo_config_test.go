package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
)

// configFormOn opens the repo-config form over d against a hub that records whatever config the
// form saves, and hands back the model plus a reader for the recorded write.
func configFormOn(t *testing.T, d api.RepoDetail) (model, func() api.Config) {
	t.Helper()
	var wrote api.Config
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repo/config" {
			if err := json.NewDecoder(r.Body).Decode(&wrote); err != nil {
				t.Errorf("decode written config: %v", err)
			}
		}
		w.Write([]byte("{}"))
	}))
	t.Cleanup(srv.Close)

	m := newModel(nil, nil, "/r/sindri")
	m.w, m.h = 100, 40
	m.cl = client.DialTCP(strings.TrimPrefix(srv.URL, "http://"), "t")
	m.openRepoConfigForm(d)
	return m, func() api.Config { return wrote }
}

// fieldNamed finds the form field carrying that label, failing the test when the form has none.
func fieldNamed(t *testing.T, m model, label string) field {
	t.Helper()
	for _, f := range m.form.fields {
		if tf, ok := f.(*textField); ok && tf.name == label {
			return tf
		}
	}
	t.Fatalf("the form has no %q field", label)
	return nil
}

// TestRepoConfigFormEditsTheReference: `reference:` decides the branch every agent works against
// and the form deliberately preserved it without showing it, so the one screen for editing this
// repo's config could not reach the key that matters most to what agents do.
func TestRepoConfigFormEditsTheReference(t *testing.T) {
	m, written := configFormOn(t, api.RepoDetail{Name: "sindri", Config: api.Config{Verify: "make check", Reference: "trunk"}})

	ref := fieldNamed(t, m, "reference")
	if ref.value() != "trunk" {
		t.Errorf("the field should be prefilled from the config, got %q", ref.value())
	}

	ref.focus()
	for _, r := range "-2" {
		ref.update(keyMsg(string(r)))
	}
	cmd := m.form.update(keyMsg("ctrl+s"))
	if cmd == nil {
		t.Fatal("ctrl+s produced no submit")
	}
	cmd()

	if got := written(); got.Reference != "trunk-2" {
		t.Errorf("the save should carry the edited reference, hub got %q", got.Reference)
	}
}

// TestRepoConfigFormKeepsTheKeysItDoesNotShow: a save rewrites the whole file, so the fields it
// omits have to ride along on the config as loaded rather than be rebuilt as zeroes.
func TestRepoConfigFormKeepsTheKeysItDoesNotShow(t *testing.T) {
	m, written := configFormOn(t, api.RepoDetail{
		Name: "sindri", Config: api.Config{Verify: "make check", Reading: []string{"ARCHITECTURE.md"}}})

	if cmd := m.form.update(keyMsg("ctrl+s")); cmd != nil {
		cmd()
	}
	got := written()
	if len(got.Reading) != 1 {
		t.Errorf("`reading` must survive a save that never showed it, got %v", got.Reading)
	}
	// An empty reference is a working setting — agents follow the main checkout — so an untouched
	// blank field must save as blank rather than being treated as a value to invent.
	if got.Reference != "" {
		t.Errorf("an empty reference must stay empty, got %q", got.Reference)
	}
}
