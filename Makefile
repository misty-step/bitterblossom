.PHONY: test test-hooks test-conductor conductor-check

test: test-hooks test-conductor

test-hooks:
	python3 -m pytest -q base/hooks/

test-conductor:
	cd conductor && mix test

conductor-check:
	cd conductor && mix conductor check-env
