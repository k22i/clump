# Calibrating bytes per token

`Store.Calibrate` folds one observation into the stored bytes-per-token for a provider:

```
obs = promptBytes / (inputTokens - overheadTokens)     ignored when the denominator < 100
                                                       or obs outside [0.5, 10]
stored = 0.7*old + 0.3*obs
```

The overhead subtraction is why `Config.OverheadTokens` exists (~650 for the claude CLI, measured
643): the call's fixed input is not proportional to prompt size, so it would bias the observation.

Fake providers and raw command output are skipped: they report estimates rather than counts, so
folding them in would calibrate the estimator against itself. The clamps keep one odd usage number
from poisoning the stored value.

A fold that cannot be written (a read-only or full cache directory) does not fail the grouping that
produced it, but it is not silent either: the first failure per process becomes a notice on the
result (`token calibration stopped updating, estimates keep the previous ratio: …`), which the
footer shows. It is reported once, not per call, because the condition is persistent and a notice
per call would bury every other notice. A cached result keeps the notice it was stored with.
