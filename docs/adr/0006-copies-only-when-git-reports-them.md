# Copies are only detected when git reports them

The loaders do not pass `--find-copies`; a `copy` section is parsed when git prints one, and
otherwise the copy shows up as an added file.

`--find-copies` would make clump's diff differ from the one lazygit shows and would change the
canonical form and therefore the diff hash — breaking cache sharing between watch and one-shot
(ADR-0020).
