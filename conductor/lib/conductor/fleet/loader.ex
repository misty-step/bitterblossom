defmodule Conductor.Fleet.Loader do
  @moduledoc """
  Parse fleet.toml into validated sprite configs with defaults applied.

  Deep module: hides all TOML parsing, validation, and default-merging
  details. Callers get a list of sprite config maps or a clear error.
  """

  alias Conductor.Workspace
  @valid_roles ~w(builder fixer polisher triage responder)
  @valid_harnesses ~w(codex claude-code)
  @type sprite_config :: %{
          name: binary(),
          role: atom(),
          org: binary(),
          repo: binary(),
          clone_url: binary() | nil,
          default_branch: binary(),
          capability_tags: [binary()],
          harness: binary(),
          model: binary(),
          reasoning_effort: binary(),
          label: binary() | nil,
          persona: binary() | nil
        }

  @doc """
  Load and validate fleet.toml from the given path.
  Returns `{:ok, config}` with `%{sprites: [...], defaults: %{...}}` or `{:error, reason}`.
  """
  @spec load(binary()) ::
          {:ok, %{sprites: [sprite_config()], defaults: map()}} | {:error, binary()}
  def load(path) do
    with {:ok, raw} <- read_toml(path),
         {:ok, defaults} <- parse_defaults(raw),
         {:ok, personas} <- parse_personas(raw),
         {:ok, sprites} <- parse_sprites(raw, defaults, personas) do
      {:ok, %{sprites: sprites, defaults: defaults}}
    end
  end

  @doc "Load fleet.toml or raise with a clear message."
  @spec load!(binary()) :: %{sprites: [sprite_config()], defaults: map()}
  def load!(path) do
    case load(path) do
      {:ok, config} -> config
      {:error, reason} -> raise "fleet.toml: #{reason}"
    end
  end

  @doc "Filter sprites by role."
  @spec by_role([sprite_config()], atom()) :: [sprite_config()]
  def by_role(sprites, role) do
    Enum.filter(sprites, &(&1.role == role))
  end

  # --- Private ---

  defp read_toml(path) do
    case File.read(path) do
      {:ok, content} ->
        case Toml.decode(content) do
          {:ok, parsed} -> {:ok, parsed}
          {:error, reason} -> {:error, "parse error: #{inspect(reason)}"}
        end

      {:error, :enoent} ->
        {:error, "#{path} not found"}

      {:error, reason} ->
        {:error, "cannot read #{path}: #{inspect(reason)}"}
    end
  end

  defp parse_defaults(raw) do
    case Map.get(raw, "defaults", %{}) do
      defaults when is_map(defaults) ->
        parse_defaults_map(defaults)

      other ->
        {:error, "[defaults] must be a TOML table, got: #{inspect(other)}"}
    end
  end

  defp parse_defaults_map(defaults) do
    with {:ok, repo} <- required_repo(defaults, "[defaults]"),
         {:ok, clone_url} <- required_clone_url(defaults, "[defaults]"),
         {:ok, default_branch} <-
           validate_default_branch(Map.get(defaults, "default_branch", "master"), "[defaults]") do
      {:ok,
       %{
         org: Map.get(defaults, "org", "misty-step"),
         repo: repo,
         clone_url: clone_url,
         default_branch: default_branch,
         harness: Map.get(defaults, "harness", "codex"),
         model: Map.get(defaults, "model", "gpt-5.4-mini"),
         reasoning_effort: Map.get(defaults, "reasoning_effort", "medium"),
         label: Map.get(defaults, "label")
       }}
    end
  end

  defp parse_personas(raw) do
    case Map.get(raw, "personas", %{}) do
      personas when is_map(personas) ->
        invalid_entries =
          for {name, value} <- personas,
              not (is_binary(name) and is_binary(value)),
              do: {name, value}

        if invalid_entries == [] do
          {:ok, personas}
        else
          {:error, "[personas] entries must be string = string pairs"}
        end

      other ->
        {:error, "[personas] must be a TOML table, got: #{inspect(other)}"}
    end
  end

  defp parse_sprites(raw, defaults, personas) do
    case Map.get(raw, "sprite") do
      nil ->
        {:error, "no [[sprite]] entries in fleet.toml"}

      sprites when is_list(sprites) ->
        results =
          Enum.with_index(sprites, 1)
          |> Enum.map(fn
            {entry, _idx} when is_map(entry) -> parse_one_sprite(entry, defaults, personas)
            {_entry, idx} -> {:error, "sprite entry ##{idx} is not a TOML table"}
          end)

        errors = for {:error, msg} <- results, do: msg

        if errors == [] do
          parsed = for({:ok, s} <- results, do: s)

          case ensure_single_responder(parsed) do
            :ok -> {:ok, parsed}
            {:error, reason} -> {:error, reason}
          end
        else
          {:error, "sprite validation errors:\n  #{Enum.join(errors, "\n  ")}"}
        end

      _ ->
        {:error, "invalid sprite declaration — expected [[sprite]] array"}
    end
  end

  defp parse_one_sprite(raw_sprite, defaults, personas) do
    name = raw_sprite["name"]
    role_str = raw_sprite["role"]

    cond do
      is_nil(name) or name == "" ->
        {:error, "sprite missing required 'name'"}

      is_nil(role_str) or role_str == "" ->
        {:error, "sprite #{name} missing required 'role'"}

      role_str not in @valid_roles ->
        {:error,
         "sprite #{name} has invalid role '#{role_str}' (valid: #{Enum.join(@valid_roles, ", ")})"}

      true ->
        harness = raw_sprite["harness"] || defaults.harness
        capability_tags = raw_sprite["capability_tags"] || []
        persona = raw_sprite["persona"]
        persona_ref = raw_sprite["persona_ref"]

        cond do
          harness not in @valid_harnesses ->
            {:error,
             "sprite #{name} has invalid harness '#{harness}' (valid: #{Enum.join(@valid_harnesses, ", ")})"}

          not valid_capability_tags?(capability_tags) ->
            {:error, "sprite #{name} capability_tags must be an array of strings"}

          not is_nil(persona) and not is_nil(persona_ref) ->
            {:error, "sprite #{name} must not set both 'persona' and 'persona_ref'"}

          not is_nil(persona_ref) and not is_binary(persona_ref) ->
            {:error, "sprite #{name} persona_ref must be a string"}

          not is_nil(persona_ref) and not Map.has_key?(personas, persona_ref) ->
            {:error, "sprite #{name} references unknown persona '#{persona_ref}'"}

          true ->
            repo = raw_sprite["repo"] || defaults.repo
            clone_url = raw_sprite["clone_url"] || defaults.clone_url

            with {:ok, validated_repo} <- validate_repo(repo, "sprite #{name}"),
                 {:ok, validated_clone_url} <- validate_clone_url(clone_url, "sprite #{name}"),
                 {:ok, validated_default_branch} <-
                   validate_default_branch(
                     raw_sprite["default_branch"] || defaults.default_branch,
                     "sprite #{name}"
                   ) do
              {:ok,
               %{
                 name: name,
                 role: String.to_atom(role_str),
                 org: raw_sprite["org"] || defaults.org,
                 repo: validated_repo,
                 clone_url: validated_clone_url,
                 default_branch: validated_default_branch,
                 capability_tags: capability_tags,
                 harness: harness,
                 model: raw_sprite["model"] || defaults.model,
                 reasoning_effort: raw_sprite["reasoning_effort"] || defaults.reasoning_effort,
                 label: raw_sprite["label"] || defaults.label,
                 persona: persona || personas[persona_ref]
               }}
            end
        end
    end
  end

  defp valid_capability_tags?(tags) when is_list(tags), do: Enum.all?(tags, &is_binary/1)
  defp valid_capability_tags?(_), do: false

  defp required_repo(map, scope) do
    case Map.get(map, "repo") do
      repo when is_binary(repo) and repo != "" ->
        validate_repo(repo, scope)

      _ ->
        {:error, "#{scope} must specify 'repo' (e.g. repo = \"service/bitterblossom\")"}
    end
  end

  defp required_clone_url(map, scope) do
    case Map.get(map, "clone_url") do
      clone_url when is_binary(clone_url) and clone_url != "" ->
        {:ok, clone_url}

      _ ->
        {:error, "#{scope} must specify 'clone_url' (the explicit transport remote)"}
    end
  end

  defp validate_repo(repo, scope) do
    case Workspace.validate_repo(repo) do
      :ok -> {:ok, repo}
      {:error, :invalid_repo} -> {:error, "#{scope} has invalid repo '#{repo}'"}
    end
  end

  defp validate_clone_url(clone_url, _scope) when is_binary(clone_url) and clone_url != "" do
    {:ok, clone_url}
  end

  defp validate_clone_url(_clone_url, scope) do
    {:error, "#{scope} must set clone_url explicitly"}
  end

  defp validate_default_branch(branch, scope) when is_binary(branch) do
    case Workspace.validate_branch(branch) do
      :ok -> {:ok, branch}
      {:error, :invalid_branch} -> {:error, "#{scope} has invalid default_branch '#{branch}'"}
    end
  end

  defp validate_default_branch(branch, scope) do
    {:error, "#{scope} has invalid default_branch '#{inspect(branch)}'"}
  end

  defp ensure_single_responder(sprites) do
    responders = Enum.filter(sprites, &(&1.role == :responder))

    case responders do
      [_single] -> :ok
      [] -> :ok
      many -> {:error, "fleet.toml v1 supports only one responder sprite, found #{length(many)}"}
    end
  end
end
