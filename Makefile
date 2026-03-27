.PHONY: test test-hooks test-conductor conductor-check ensure-mix

test: test-hooks test-conductor

test-hooks:
	@if command -v pytest >/dev/null 2>&1; then \
		pytest -q base/hooks/; \
	else \
		python3 -m pytest -q base/hooks/; \
	fi

ensure-mix:
	@if command -v mix >/dev/null 2>&1; then \
		:; \
	else \
		echo "error: Elixir tooling missing: 'mix' not found in PATH" >&2; \
		echo "hint: install Elixir/OTP before running conductor targets" >&2; \
		exit 127; \
	fi

test-conductor: ensure-mix
	cd conductor && mix deps.get && mix test

conductor-check: ensure-mix
	cd conductor && mix conductor check-env
