defmodule Conductor.Fleet.Loader do
  @moduledoc """
  Parse fleet.toml into validated sprite configs with defaults applied.

  Deep module: hides all TOML parsing, validation, and default-merging
  details. Callers get a list of sprite config maps or a clear error.
  """

  @valid_roles ~w(builder fixer polisher triage responder)
  @valid_harnesses ~w(codex claude-code)
  @type sprite_config :: %{
          name: binary(),
          role: atom(),
          org: binary(),
          repo: binary(),
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
    repo = Map.get(defaults, "repo")

    if is_nil(repo) or repo == "" do
      {:error, "[defaults] must specify 'repo' (e.g. repo = \"org/repo\")"}
    else
      {:ok,
       %{
         org: Map.get(defaults, "org", "misty-step"),
         repo: repo,
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
          {:ok, for({:ok, s} <- results, do: s)}
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
            {:ok,
             %{
               name: name,
               role: String.to_atom(role_str),
               org: raw_sprite["org"] || defaults.org,
               repo: raw_sprite["repo"] || defaults.repo,
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

  defp valid_capability_tags?(tags) when is_list(tags), do: Enum.all?(tags, &is_binary/1)
  defp valid_capability_tags?(_), do: false
end
