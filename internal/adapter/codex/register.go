package codex

import "github.com/flo-at/sindri/internal/adapter"

func init() {
	adapter.RegisterFactory(func() adapter.Adapter {
		return New()
	})
}
