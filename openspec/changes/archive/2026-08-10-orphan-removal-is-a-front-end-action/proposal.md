# Removing an orphan is a front-end action, not a suggested shell command

## Why

Two specs say the user is handed a shell command to run themselves:

- `hub` — the hub "SHALL surface them as a warning and propose a shell command the user
  can run to remove them".
- `view-workers` — the workers view "SHALL show the proposed shell command to remove it".

The TUI has not worked that way for some time: the Agents tab removes an orphan directly,
behind a confirmation (`openRemoveOrphanChoice` → `client.RemoveOrphan` → `POST
/orphan/remove`). So one of the two has to give — either the TUI action is out of spec and
should be deleted, or the requirement is describing an implementation that has been
replaced.

The requirement is the stale one, and the reason is in its own next clause: what it
actually protects is **"The hub SHALL NOT silently kill orphans"** — nothing dies without
the user asking. A confirmed removal the user initiates is not a silent kill; it is the
user asking, in the front end they already have open. Printing `podman rm -f` for them to
copy served that same goal by making the removal manual, but it also leaks the container
engine into the interface — the name of the tool, not the port — which
`name-the-port-not-the-tool` is elsewhere removing, and it means the front end knows the
runtime's CLI syntax rather than calling the hub.

The CLI, meanwhile, still only prints the suggestion, so this is also the parity gap
td-e8392c names: `RemoveOrphan` is a client method the TUI can reach and the CLI cannot.

## What changes

- The hub keeps the guarantee that matters — it never kills an orphan on its own — and
  reports the orphan so a front end can offer removal. It no longer prescribes a shell
  command as the mechanism.
- The workers view flags an orphan distinctly from an agent row, as before, and offers a
  confirmed removal rather than displaying engine syntax.
- Both front ends can remove an orphan, which is what "any behaviour offered by one
  front-end must be reachable from the other" already required.

## Impact

- Specs: `hub`, `view-workers`.
- Code: the CLI gains the removal (`sindri agent delete <orphan>`); the TUI already had it.
  No hub change — `POST /orphan/remove` exists and is what the TUI already calls.
