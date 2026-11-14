## MODIFIED Requirements

### Requirement: Commit message
`commit` SHALL take the message from `-m` (repeatable, parts joined by a blank line) or `-F <file>` (`-F -` reads stdin); with both, `-F` SHALL win. Without either it SHALL exit with code 2 and SHALL NOT open an editor. It SHALL clean the message as `git stripspace` does before the `commit-msg` hook, and again after that hook when it ran, SHALL exit with code 1 when the result is empty, and SHALL NOT apply `commit.cleanup` or `commit.template`.

#### Scenario: No message
- **WHEN** the user runs `commit` without `-m` or `-F`
- **THEN** the command exits with code 2 and no commit is created

#### Scenario: Both -m and -F
- **WHEN** the user runs `commit -m "a" -F msg.txt`
- **THEN** the commit message is the content of `msg.txt`

#### Scenario: Message cleanup
- **WHEN** the user runs `commit -m "  \n\nsubject  \n\n\n\nbody\t\n\n"`
- **THEN** the commit message is `subject\n\nbody\n`, the message that `git stripspace` gives for that text
