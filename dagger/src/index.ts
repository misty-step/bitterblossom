/**
 * Bitterblossom local CI pipeline.
 *
 * Canonical full gate: `dagger call check`
 * Fast edit loop: `dagger call fast`
 */
import {
  dag,
  CacheVolume,
  Container,
  Directory,
  argument,
  object,
  func,
} from "@dagger.io/dagger"

const ELIXIR_IMAGE = "elixir:1.18.4-otp-27"
const PYTHON_IMAGE = "python:3.13-slim"
const SHELLCHECK_IMAGE = "koalaman/shellcheck-alpine:stable"
const TRUFFLEHOG_IMAGE = "trufflesecurity/trufflehog:latest"

function pythonContainer(source: Directory): Container {
  return dag.container().from(PYTHON_IMAGE).withDirectory("/src", source).withWorkdir("/src")
}

function elixirBase(source: Directory): Container {
  return dag
    .container()
    .from(ELIXIR_IMAGE)
    .withEnvVariable("HEX_HOME", "/root/.hex")
    .withEnvVariable("MIX_HOME", "/root/.mix")
    .withEnvVariable("LANG", "C.UTF-8")
    .withEnvVariable("LC_ALL", "C.UTF-8")
    .withMountedCache("/root/.hex", dag.cacheVolume("bitterblossom-ci-hex"))
    .withMountedCache("/root/.mix", dag.cacheVolume("bitterblossom-ci-mix"))
    .withMountedCache("/root/.cache/rebar3", dag.cacheVolume("bitterblossom-ci-rebar3"))
    .withDirectory("/src", source)
    .withWorkdir("/src/conductor")
    .withExec(["mix", "local.hex", "--force"])
    .withExec(["mix", "local.rebar", "--force"])
}

function elixirEnv(source: Directory, mixEnv: string, cacheKey: string): Container {
  const depsCache: CacheVolume = dag.cacheVolume(`bitterblossom-ci-deps-${mixEnv}-${cacheKey}`)
  const buildCache: CacheVolume = dag.cacheVolume(`bitterblossom-ci-build-${mixEnv}-${cacheKey}`)

  return elixirBase(source)
    .withEnvVariable("MIX_ENV", mixEnv)
    .withMountedCache("/src/conductor/deps", depsCache)
    .withMountedCache("/src/conductor/_build", buildCache)
    .withExec(["mix", "deps.get"])
}

function errorDetail(error: unknown): string {
  if (error instanceof Error) {
    return error.message
  }

  return String(error)
}

@object()
export class BitterblossomCi {
  @func()
  async shell(
    @argument({
      defaultPath: "/",
      ignore: [
        ".git",
        ".bb",
        ".claude/worktrees",
        ".DS_Store",
        ".pytest_cache",
        ".ruff_cache",
        ".evidence",
        "conductor/.bb",
        "conductor/_build",
        "conductor/deps",
        "conductor/erl_crash.dump",
        "dagger/node_modules",
        "dagger/sdk",
      ],
    })
    source?: Directory,
  ): Promise<void> {
    await dag
      .container()
      .from(SHELLCHECK_IMAGE)
      .withDirectory("/src", source!)
      .withWorkdir("/src")
      .withExec([
        "sh",
        "-lc",
        "find scripts -type f -name '*.sh' -print0 | xargs -0 shellcheck -x -S error",
      ])
      .sync()
  }

  @func()
  async hooks(
    @argument({
      defaultPath: "/",
      ignore: [
        ".git",
        ".bb",
        ".claude/worktrees",
        ".DS_Store",
        ".pytest_cache",
        ".ruff_cache",
        ".evidence",
        "conductor/.bb",
        "conductor/_build",
        "conductor/deps",
        "conductor/erl_crash.dump",
        "dagger/node_modules",
        "dagger/sdk",
      ],
    })
    source?: Directory,
  ): Promise<void> {
    await pythonContainer(source!)
      .withExec(["pip", "install", "--quiet", "pytest==8.4.1"])
      .withExec(["python3", "-m", "pytest", "-q", "base/hooks/", "scripts/test_runtime_contract.py"])
      .sync()
  }

  @func()
  async yaml(
    @argument({
      defaultPath: "/",
      ignore: [
        ".git",
        ".bb",
        ".claude/worktrees",
        ".DS_Store",
        ".pytest_cache",
        ".ruff_cache",
        ".evidence",
        "conductor/.bb",
        "conductor/_build",
        "conductor/deps",
        "conductor/erl_crash.dump",
        "dagger/node_modules",
        "dagger/sdk",
      ],
    })
    source?: Directory,
  ): Promise<void> {
    await pythonContainer(source!)
      .withExec(["python3", "-m", "pip", "install", "--quiet", "yamllint==1.38.0"])
      .withExec(["yamllint", "-c", ".yamllint.yml", "compositions"])
      .sync()
  }

  @func()
  async elixir(
    @argument({
      defaultPath: "/",
      ignore: [
        ".git",
        ".bb",
        ".claude/worktrees",
        ".DS_Store",
        ".pytest_cache",
        ".ruff_cache",
        ".evidence",
        "conductor/.bb",
        "conductor/_build",
        "conductor/deps",
        "conductor/erl_crash.dump",
        "dagger/node_modules",
        "dagger/sdk",
      ],
    })
    source?: Directory,
  ): Promise<void> {
    await elixirEnv(source!, "test", "quality")
      .withExec(["mix", "compile", "--warnings-as-errors"])
      .withExec(["mix", "format", "--check-formatted"])
      .withExec(["mix", "test"])
      .sync()
  }

  @func()
  async secrets(
    @argument({
      defaultPath: "/",
      ignore: [
        ".git",
        ".bb",
        ".claude/worktrees",
        ".DS_Store",
        ".pytest_cache",
        ".ruff_cache",
        ".evidence",
        "conductor/.bb",
        "conductor/_build",
        "conductor/deps",
        "conductor/erl_crash.dump",
        "dagger/node_modules",
        "dagger/sdk",
      ],
    })
    source?: Directory,
  ): Promise<void> {
    await dag
      .container()
      .from(TRUFFLEHOG_IMAGE)
      .withDirectory("/src", source!)
      .withWorkdir("/src")
      .withExec([
        "trufflehog",
        "filesystem",
        "--directory",
        "/src",
        "--results=verified",
        "--fail",
        "--no-update",
      ])
      .sync()
  }

  @func()
  async fast(
    @argument({
      defaultPath: "/",
      ignore: [
        ".git",
        ".bb",
        ".claude/worktrees",
        ".DS_Store",
        ".pytest_cache",
        ".ruff_cache",
        ".evidence",
        "conductor/.bb",
        "conductor/_build",
        "conductor/deps",
        "conductor/erl_crash.dump",
        "dagger/node_modules",
        "dagger/sdk",
      ],
    })
    source?: Directory,
  ): Promise<string> {
    const repo = source!

    await Promise.all([
      this.shell(repo),
      this.hooks(repo),
      this.yaml(repo),
      elixirEnv(repo, "test", "fast")
        .withExec(["mix", "compile", "--warnings-as-errors"])
        .withExec(["mix", "format", "--check-formatted"])
        .sync(),
    ])

    return "fast gates passed"
  }

  @func()
  async check(
    @argument({
      defaultPath: "/",
      ignore: [
        ".git",
        ".bb",
        ".claude/worktrees",
        ".DS_Store",
        ".pytest_cache",
        ".ruff_cache",
        ".evidence",
        "conductor/.bb",
        "conductor/_build",
        "conductor/deps",
        "conductor/erl_crash.dump",
        "dagger/node_modules",
        "dagger/sdk",
      ],
    })
    source?: Directory,
  ): Promise<string> {
    const repo = source!
    const results: Array<{ name: string; ok: boolean; detail: string }> = []

    const runGate = async (name: string, gate: Promise<void>): Promise<void> => {
      try {
        await gate
        results.push({ name, ok: true, detail: "OK" })
      } catch (error) {
        results.push({ name, ok: false, detail: errorDetail(error) })
      }
    }

    await Promise.all([
      runGate("shell", this.shell(repo)),
      runGate("hooks", this.hooks(repo)),
      runGate("yaml", this.yaml(repo)),
      runGate("secrets", this.secrets(repo)),
      runGate("elixir", this.elixir(repo)),
    ])

    const lines = ["Bitterblossom CI Results", "=".repeat(40)]
    let passed = 0
    let failed = 0

    for (const result of results.sort((a, b) => a.name.localeCompare(b.name))) {
      lines.push(`  ${result.ok ? "PASS" : "FAIL"}  ${result.name}`)

      if (result.ok) {
        passed += 1
        continue
      }

      failed += 1
      for (const line of result.detail.split("\n").slice(0, 10)) {
        lines.push(`         ${line}`)
      }
    }

    lines.push("=".repeat(40))
    lines.push(`${passed} passed, ${failed} failed`)

    const summary = lines.join("\n")
    if (failed > 0) {
      throw new Error(summary)
    }

    return summary
  }
}
