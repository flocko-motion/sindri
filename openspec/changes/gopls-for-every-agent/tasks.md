# Tasks

## 1. Declare it always

- [x] 1.1 Drop both guards; `mcpServers()` no longer takes a workspace, since nothing is decided
      from it.
- [x] 1.2 Replace the test that asserted a non-Go workspace gets nothing — it encoded the rule this
      reverses, and its successor says which rule changed and why.

## 2. The counterpart obligation: the shim explains itself

- [x] 2.1 Locate the module: `go.work` or `go.mod` at the root, else the shallowest `go.mod` within
      a bounded depth. Breadth-first, so the outermost module wins.
- [x] 2.2 Skip `.git`, `vendor`, `node_modules`, `testdata` and dotted directories — thousands of
      directories, none of them the module being served.
- [x] 2.3 Refuse out loud: name the directory and the depth searched, and say nothing was examined,
      so the answer is never mistaken for a fact about the code.
- [x] 2.4 Serve gopls FROM the located module, not from the workspace root.

## 3. Discoverability

- [x] 3.1 Answer the question rather than assume: hiding it was deliberate and is no longer right,
      now that every agent depends on it. Listed, with a Short that says who spawns it.

## 4. Pin it

- [x] 4.1 Every pod gets the server — no module at the root, and no workspace at all.
- [x] 4.2 Each layout the root `stat` rejected: subdirectory module, `go.work` over several modules,
      mixed-language tree, two levels down.
- [x] 4.3 The outermost module wins; the search does not wander into dependencies.
- [x] 4.4 A tree with no module is refused in words, and an empty workspace says nothing was
      examined.
- [x] 4.5 Mutation-checked compilably: restoring the `go.mod` guard, removing the descent, and
      descending into skipped directories each fail their own tests.
