# Outside Contributors

Unless you are working for the maintainer, don't change moor's code. File a
GitHub issue instead, as described in CONTRIBUTING.md.

The rest of this file is for whoever changes moor's code.

# Validation

Use `./test.sh` to run all tests. In addition to just running all tests, that
script will do linting, some cross compiling and more.

# Fixing Bugs

1. Create a new branch with a sensible name, e.g. `fix-crash-on-search-backwards`.
2. Reproduce the bug with a failing test. Commit this test once it reproduces
   the bug properly.
3. Fix the bug until your new test passes.

# Manual Testing

If the user wants to test manually, ask them to run `./moor.sh` rather than
building a binary yourself. `moor.sh` builds and runs the current sources with
race detection enabled.

# Race Conditions

`./moor.sh` and `./test.sh` enable the race detector, and any race it detects
lands in a `moor-race-report.*` file in the repo root.

If you see one of those files, work through RACES.md before finishing whatever
else you are doing. Race fixes are exempt from the "reproduce with a failing
test" rule above, RACES.md says what to do instead.

# PR Best Practices

Always run `./test.sh` locally before making any PRs.

# Releases

Release messages go into annotated tags. Please look at the ten most recent
annotated tags for style guidance. The basis for all those messages are user
visible changes since last release.

# Windows Testing

Keyboard handling and terminal modes in `cmd.exe` can only be verified on real
Windows. WINDOWS-VM.md walks through the throwaway VirtualBox VM for that, and
`scripts/windows-vm-run.sh 'some command'` runs a command in that guest and
prints its output back here.

The guest is on a **US keyboard layout** — keep it, or injected keystrokes get
their punctuation silently re-mapped. WINDOWS-VM.md also has the scancodes for
keys that aren't characters, which is what driving moor in the guest takes.
