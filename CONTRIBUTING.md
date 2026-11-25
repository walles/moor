# Contributing to moor

If you'd like something changed in moor, please file a GitHub issue rather
than opening a pull request. The maintainer prefers to write fixes himself,
and what he needs from you is a good report. The sections below say what
makes one.

Before filing, search the existing issues, open and closed, for the same
problem.

## Reporting Bugs

What the maintainer needs from you is a way to see the problem.

A good bug report:

- Has a title describing what the user sees going wrong, not the cause or the
  fix.
- Says what you expected, what happened instead, and how to tell the two
  apart.
- Says what you were doing when you ran into it: what kind of input you were
  viewing, where it came from, and anything unusual about the situation, for
  example a file being rewritten while moor shows it, and by what and why.
  This tells the maintainer how common the situation is and which fix suits
  it.
- Has numbered steps that reproduce the problem from scratch, with exact
  commands and keystrokes. Generate input on the spot, for example with
  `seq 1 1000 | moor`, rather than relying on files only you have. If
  you can't reproduce it reliably, or only with input you can't share, say so
  and describe as precisely as you can when it happens.
- If moor crashed, includes the complete crash output. If you ran
  `./moor.sh` and a `moor-race-report.*` file showed up in your moor
  checkout, include that too.
- Says which moor version (`moor --version`) and platform you were on, and
  which terminal, including its version. If you run moor inside tmux, screen
  or similar, say which one and its version too.
- Is otherwise short. Leave out anything the steps don't need.

Before posting, run the steps yourself on the latest moor release, or on
`master` if you build from source, and check that they show the problem. If
they don't, it may already be fixed; say so in the report if you post it
anyway.

If you have an idea where the problem is, add it at the end, clearly labelled
as a hint and not a proposed fix.

## Requesting Features

Start by saying what you were trying to do and how moor got in the way, for
example "When scrolling left with a trackpad I overshoot and get line numbers
I turned off". Then, optionally, suggest a change.

What you were doing matters more than the proposed change. Knowing it lets
the maintainer judge how much the problem matters and find the best fix,
which may not be the one you had in mind.
