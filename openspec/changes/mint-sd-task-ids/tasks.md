# Tasks

## 1. Centralise the id scheme

- [x] 1.1 `internal/hub/task/ids.go` answers every prefix question: mint an owned id, which
      source owns an id, whether sindri owns it, and whether an id is a legacy td one. The
      prefixes are written in that file and nowhere else.
- [x] 1.2 Route every caller through it — `ownedsource.go` (mint and `owns`), `task.go`
      (two hardcoded `"td-"` comparisons that had drifted from the constant), `importtd.go`,
      and the openspec and GitHub adapters, which minted and parsed their own prefixes.

## 2. Mint sd-

- [x] 2.1 Change the mint prefix to `sd-`; `td-` stays recognised as legacy. One line, which
      is the test of whether step 1 was done right.
- [x] 2.2 Confirm no id is rewritten and every existing `td-` task still resolves.

## 3. Bring the documents into line

- [x] 3.1 The `hub` requirement and its "prefix names the owner" scenario.
- [x] 3.2 `03-gh-local`'s branch-name scenario.
- [x] 3.3 The README's task and PR examples.
