# workflow — delta

## ADDED Requirements

### Requirement: A reviewer can read what the work was for

A reviewer SHALL be able to read the task a PR belongs to — its title, description, type, labels
and status — together with its parent, its siblings under that parent, its children, and the
comments on them. It SHALL be able to read the wider backlog on the same terms as the roles that
already do.

Without this a reviewer can answer whether a diff is good code that fits the architecture, and never
whether it does what was asked, so intent goes unchecked until the human merge. The hierarchy is
part of it: a reviewer that cannot see the neighbouring tasks cannot tell a genuine gap from a
boundary another task deliberately owns. So are the comments, which is where a plan is corrected
after the fact — a review measured on the body alone can grade work against an approach already
abandoned.

This access SHALL be read-only. A reviewer SHALL NOT gain any verb that proposes, edits, re-orders
or claims the work it judges: its independence is what makes the review worth having, and it is
reading the plan that preserves that, where writing it would not.

The reviewer SHALL be told, in the directive that assigns a review AND in the review instruction
itself, that the task and its comments are readable and which verbs read them. Access nobody
mentions is access nobody uses. The directive SHALL name the task by id AND title, as the
directive that assigns work to a worker does.

A built-in default instruction SHALL NOT be copied into a project's own state as a side effect of
being used. A default a project holds a copy of can never be improved for that project again, and
the divergence is silent — so the built-in SHALL apply wherever nothing has deliberately overridden
it, and a stored instruction matching one the tool itself wrote SHALL count as not overridden.

An interface that shows a PR to an agent SHALL name the task it belongs to, as the host's PR detail
does.

#### Scenario: The reviewer reads the task under review

- **WHEN** a reviewer reads the task a PR it was handed belongs to
- **THEN** it sees the title, description, type, labels and status, its parent and children, and
  the comments on it

#### Scenario: The reviewer reads the surrounding backlog

- **WHEN** a reviewer lists the backlog
- **THEN** it sees the whole tree, not only the task under review

#### Scenario: Reading grants nothing else

- **WHEN** a reviewer attempts to create, edit, re-order or claim a task
- **THEN** it is refused, whatever it can read

#### Scenario: The reviewer is told what it can read

- **WHEN** a review is assigned
- **THEN** the directive names the task by id and title and points at the verbs that read the task,
  its hierarchy and its comments, and the review instruction says the same

#### Scenario: An improved default instruction reaches an existing project

- **WHEN** the built-in review instruction changes and a project already holds a stored copy of an
  earlier one
- **THEN** the reviewer is given the current instruction

#### Scenario: A deliberately written instruction is kept

- **WHEN** a project's stored review instruction differs from any the tool wrote
- **THEN** that instruction is used unchanged

#### Scenario: A spec-linked task is discoverable

- **WHEN** a reviewer reads a task carrying a `spec:<name>` label
- **THEN** the label is shown, so the reviewer can verify the diff against that spec

#### Scenario: A PR shown to an agent names its task

- **WHEN** an agent displays a PR
- **THEN** the linked task is named alongside the branch and the diff
