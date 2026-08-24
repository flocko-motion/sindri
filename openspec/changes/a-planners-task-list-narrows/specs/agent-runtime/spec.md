# agent-runtime — delta

## ADDED Requirements

### Requirement: The agent's backlog listing narrows by default and refuses what it cannot do

The agent's `task list` SHALL default to the active segment of the backlog — open work, plus whatever
stopped being open recently — rather than to every task ever recorded. A backlog is mostly closed
history, and a planner reading the whole of it is reading a hundred rows of what nobody can act on.

Every listing SHALL close by stating what it showed AND the shape of the whole backlog. That is what
makes a narrow default safe: without it, fourteen rows would read as a backlog of fourteen tasks. The
wording SHALL be shared with the host CLI's own listing, so the two cannot drift into describing the
same backlog differently.

The listing SHALL accept a filter naming any segment, so nothing the default hides becomes
unreachable.

An unrecognised argument SHALL be refused, and the refusal SHALL name what is accepted. The listing
SHALL NOT be printed alongside it. Accepting an argument and ignoring it makes every filter look
identical, which reads as a filter that does not work rather than one that was never applied — the
reader then investigates the wrong thing. An argument in the position of a task id SHALL be refused
the same way rather than answered as an unknown task, which sends them looking for a task instead of
at what they typed.

A filter VALUE that is not recognised SHALL be refused by the verb. This is deliberately stricter than
the shared predicate, which admits everything on an unknown value so that a listing can never claim an
empty backlog: that leniency is a permissive answer to a question that was understood, and does not
extend to an argument the verb never understood at all.

Where the listing is indented by tree, an ancestor of a shown task SHALL remain visible and SHALL be
marked as context rather than as a match, or the listing SHALL say the tree is partial. A subtask whose
parent the filter removed would otherwise sit at a depth that means nothing, and which tree a task
hangs in is most of what the view is read for.

#### Scenario: The bare listing

- **WHEN** an agent whose reading is not bounded to its own work runs `task`, or `task list`, with no
  further arguments
- **THEN** both show the same thing: the active segment, closing by saying how many it showed and how
  many tasks are open and closed in total

#### Scenario: Asking for the whole backlog

- **WHEN** an agent runs `task list` with the filter naming every task
- **THEN** the whole backlog is listed

#### Scenario: An argument the verb does not have

- **WHEN** an agent runs `task list` with an unrecognised argument or an unrecognised filter value
- **THEN** the verb refuses, names the filters it accepts, and prints no listing

#### Scenario: A shown subtask whose parent is filtered out

- **WHEN** a filter admits a subtask but not its parent
- **THEN** the parent is listed as context, marked as such, and the subtask keeps its place beneath it
