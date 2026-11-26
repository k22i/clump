# `colorArg` stays at `always`

The lazygit stanza clump installs leaves `colorArg` at lazygit's default (`always`), and `push`
strips the SGR itself.

`colorArg: never` would make clump's life easier and lazygit's panel lose its colour — the user
would pay for clump's convenience in the tool they are already using. Stripping is one regex on the
first pass over stdin, and it leaves the payload byte-identical to the `never` form.
