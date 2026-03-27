.PHONY: test test-hooks test-conductor conductor-check

test: test-hooks test-conductor

test-hooks:
	@if command -v pytest >/dev/null 2>&1; then \
		pytest -q base/hooks/; \
	else \
		python3 -m pytest -q base/hooks/; \
	fi

test-conductor:
	cd conductor && mix deps.get && mix test

conductor-check:
	cd conductor && mix conductor check-env
