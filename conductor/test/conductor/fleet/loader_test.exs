defmodule Conductor.Fleet.LoaderTest do
  use ExUnit.Case, async: true

  alias Conductor.Fleet.Loader

  @valid_toml """
  version = "1"

  [defaults]
  org = "test-org"
  repo = "test-org/test-repo"
  harness = "codex"
  model = "gpt-5.4"
  reasoning_effort = "medium"

  [personas]
  fern = "Review carefully."

  [[sprite]]
  name = "bb-weaver"
  role = "builder"
  capability_tags = ["elixir", "ci"]
  persona = "Build things."

  [[sprite]]
  name = "bb-thorn"
  role = "fixer"

  [[sprite]]
  name = "bb-fern"
  role = "polisher"
  reasoning_effort = "high"
  persona_ref = "fern"

  [[sprite]]
  name = "bb-muse"
  role = "triage"
  """

  setup do
    dir = System.tmp_dir!()
    path = Path.join(dir, "fleet-test-#{:rand.uniform(100_000)}.toml")
    on_exit(fn -> File.rm(path) end)
    %{path: path}
  end

  describe "load/1" do
    test "parses valid fleet.toml with defaults applied", %{path: path} do
      File.write!(path, @valid_toml)
      assert {:ok, config} = Loader.load(path)

      assert length(config.sprites) == 4
      assert config.defaults.org == "test-org"
      assert config.defaults.repo == "test-org/test-repo"

      [builder, fixer, polisher, muse] = config.sprites
      assert builder.name == "bb-weaver"
      assert builder.role == :builder
      assert builder.org == "test-org"
      assert builder.harness == "codex"
      assert builder.model == "gpt-5.4"
      assert builder.reasoning_effort == "medium"
      assert builder.capability_tags == ["elixir", "ci"]
      assert builder.persona == "Build things."

      assert fixer.name == "bb-thorn"
      assert fixer.role == :fixer

      assert polisher.name == "bb-fern"
      assert polisher.role == :polisher
      assert polisher.reasoning_effort == "high"
      assert polisher.persona == "Review carefully."

      assert muse.name == "bb-muse"
      assert muse.role == :triage
    end

    test "sprite inherits defaults when not overridden", %{path: path} do
      File.write!(path, @valid_toml)
      {:ok, config} = Loader.load(path)
      [builder | _] = config.sprites

      assert builder.org == "test-org"
      assert builder.repo == "test-org/test-repo"
      assert builder.label == nil
      assert builder.capability_tags == ["elixir", "ci"]
    end

    test "label stays optional when configured explicitly", %{path: path} do
      File.write!(path, """
      version = "1"

      [defaults]
      repo = "test/repo"
      label = "hold"

      [[sprite]]
      name = "bb-weaver"
      role = "builder"
      """)

      assert {:ok, config} = Loader.load(path)
      [builder] = config.sprites
      assert config.defaults.label == "hold"
      assert builder.label == "hold"
    end

    test "returns error for missing file" do
      assert {:error, msg} = Loader.load("/nonexistent/fleet.toml")
      assert msg =~ "not found"
    end

    test "returns error for missing repo in defaults", %{path: path} do
      File.write!(path, """
      version = "1"
      [defaults]
      org = "test"

      [[sprite]]
      name = "bb-test"
      role = "builder"
      """)

      assert {:error, msg} = Loader.load(path)
      assert msg =~ "must specify 'repo'"
    end

    test "returns error for missing sprites", %{path: path} do
      File.write!(path, """
      version = "1"
      [defaults]
      org = "test"
      repo = "test/repo"
      """)

      assert {:error, msg} = Loader.load(path)
      assert msg =~ "no [[sprite]] entries"
    end

    test "returns error for unknown persona_ref", %{path: path} do
      File.write!(path, """
      version = "1"

      [defaults]
      repo = "test/repo"

      [[sprite]]
      name = "bb-fern"
      role = "polisher"
      persona_ref = "missing"
      """)

      assert {:error, msg} = Loader.load(path)
      assert msg =~ "references unknown persona 'missing'"
    end

    test "returns error when sprite sets both persona and persona_ref", %{path: path} do
      File.write!(path, """
      version = "1"

      [defaults]
      repo = "test/repo"

      [personas]
      fern = "Review carefully."

      [[sprite]]
      name = "bb-fern"
      role = "polisher"
      persona = "inline"
      persona_ref = "fern"
      """)

      assert {:error, msg} = Loader.load(path)
      assert msg =~ "must not set both 'persona' and 'persona_ref'"
    end

    test "returns error for invalid role", %{path: path} do
      File.write!(path, """
      [defaults]
      repo = "test/repo"

      [[sprite]]
      name = "bb-test"
      role = "invalid"
      """)

      assert {:error, msg} = Loader.load(path)
      assert msg =~ "invalid role"
    end

    test "returns error for missing name", %{path: path} do
      File.write!(path, """
      [defaults]
      repo = "test/repo"

      [[sprite]]
      role = "builder"
      """)

      assert {:error, msg} = Loader.load(path)
      assert msg =~ "missing required 'name'"
    end

    test "returns error for invalid TOML syntax", %{path: path} do
      File.write!(path, "this is not [valid toml")
      assert {:error, msg} = Loader.load(path)
      assert msg =~ "parse error"
    end
  end

  describe "by_role/2" do
    test "filters sprites by role", %{path: path} do
      File.write!(path, @valid_toml)
      {:ok, config} = Loader.load(path)

      builders = Loader.by_role(config.sprites, :builder)
      assert length(builders) == 1
      assert hd(builders).name == "bb-weaver"

      polishers = Loader.by_role(config.sprites, :polisher)
      assert length(polishers) == 1
      assert hd(polishers).name == "bb-fern"

      fixers = Loader.by_role(config.sprites, :fixer)
      assert length(fixers) == 1
      assert hd(fixers).name == "bb-thorn"
    end
  end
end
