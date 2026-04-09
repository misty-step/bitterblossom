.PHONY: ci ci-fast test test-hooks test-conductor conductor-check ensure-mix ensure-dagger

ci: ensure-dagger
	./scripts/ci/dagger-call.sh check

ci-fast: ensure-dagger
	./scripts/ci/dagger-call.sh fast

test: ci

ensure-dagger:
	@if command -v dagger >/dev/null 2>&1; then \
		:; \
	else \
		echo "error: Dagger CLI missing: 'dagger' not found in PATH" >&2; \
		echo "hint: install Dagger before running repo-level CI" >&2; \
		exit 127; \
	fi

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
