# Builder model-eval fixture

Builder variants must run in measurement mode unless the operator explicitly
sets `dry_run = true` to false with a unique branch slug. This fixture preserves
the model-selection contract without carrying production task cards.
