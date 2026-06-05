package main

import (
	"fmt"
	"time"

	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/board"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/worker"
)

func main() {
	root := "/r/sindri"
	timeit := func(name string, f func()) {
		t := time.Now()
		f()
		fmt.Printf("%-20s %v\n", name, time.Since(t))
	}
	timeit("td.Tasks(open)", func() { td.Tasks(root, issue.FilterOpen) })
	timeit("td.Tasks(all)", func() { td.Tasks(root, issue.FilterAll) })
	timeit("spec.Changes", func() { spec.Changes(root) })
	timeit("worker.List", func() { worker.List(root) })
	timeit("store.ListFor", func() { store.ListFor(root) })
	timeit("board.List (all)", func() { board.List(root, issue.FilterAll) })
}
