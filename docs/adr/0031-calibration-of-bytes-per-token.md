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
