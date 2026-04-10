defmodule Conductor.Canary.ServiceCatalog do
  @moduledoc """
  Load and validate the typed Canary service catalog.

  This catalog is the repo-owned authority for `service -> repo + rollout
  permissions`. It stays intentionally small and strict so Tansy has a truthful
  and reviewable control surface.
  """

  alias Conductor.Workspace

  @allowed_keys ~w(
    name
    aliases
    repo
    clone_url
    default_branch
    test_cmd
    deploy_cmd
    rollback_cmd
    auto_merge
    auto_deploy
    stabilization_window_s
  )

  @type service :: %{
          name: binary(),
          aliases: [binary()],
          repo: binary(),
          clone_url: binary(),
          default_branch: binary(),
          test_cmd: [binary()],
          deploy_cmd: [binary()] | nil,
          rollback_cmd: [binary()] | nil,
          auto_merge: boolean(),
          auto_deploy: boolean(),
          stabilization_window_s: pos_integer()
        }

  @spec load(binary()) :: {:ok, [service()]} | {:error, binary()}
  def load(path) do
    with {:ok, raw} <- read_toml(path),
         {:ok, services} <- parse_services(raw) do
      {:ok, services}
    end
  end

  @spec load!() :: [service()]
  def load! do
    load!(Conductor.Config.canary_services_path())
  end

  @spec load!(binary()) :: [service()]
  def load!(path) do
    case load(path) do
      {:ok, services} -> services
      {:error, reason} -> raise "canary-services.toml: #{reason}"
    end
  end

  @spec fetch([service()], binary()) :: {:ok, service()} | {:error, :not_found}
  def fetch(services, name) when is_list(services) and is_binary(name) do
    case Enum.find(services, &service_matches?(&1, name)) do
      nil -> {:error, :not_found}
      service -> {:ok, service}
    end
  end

  @spec resolve([service()], binary()) :: {:ok, service()} | {:error, :not_found}
  def resolve(services, name), do: fetch(services, name)

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

  defp parse_services(raw) do
    case Map.get(raw, "service") do
      nil ->
        {:error, "no [[service]] entries in canary-services.toml"}

      services when is_list(services) ->
        with results <- Enum.with_index(services, 1) |> Enum.map(&parse_one_service/1),
             [] <- Enum.filter(results, &match?({:error, _}, &1)),
             parsed <- Enum.map(results, fn {:ok, service} -> service end),
             :ok <- ensure_unique_service_identifiers(parsed) do
          {:ok, parsed}
        else
          [{:error, _} | _] = errors ->
            {:error,
             "service validation errors:\n  " <>
               Enum.map_join(errors, "\n  ", fn {:error, msg} -> msg end)}

          {:error, reason} ->
            {:error, reason}
        end

      other ->
        {:error, "[[service]] entries must be a TOML array, got: #{inspect(other)}"}
    end
  end

  defp ensure_unique_service_identifiers(services) do
    duplicate =
      services
      |> Enum.flat_map(fn service -> [service.name | service.aliases] end)
      |> Enum.frequencies()
      |> Enum.find(fn {_name, count} -> count > 1 end)

    case duplicate do
      nil -> :ok
      {name, _} -> {:error, "duplicate service identifier '#{name}'"}
    end
  end

  defp parse_one_service({entry, idx}) when is_map(entry) do
    with :ok <- validate_allowed_keys(entry, idx),
         {:ok, name} <- validate_required_string(entry, "name", idx),
         {:ok, aliases} <- validate_aliases(entry, idx, name),
         {:ok, repo} <- validate_repo(entry, idx),
         {:ok, clone_url} <- validate_required_string(entry, "clone_url", idx),
         {:ok, branch} <- validate_default_branch(entry, idx),
         {:ok, test_cmd} <- validate_argv(entry, "test_cmd", idx, required: true),
         {:ok, deploy_cmd} <- validate_argv(entry, "deploy_cmd", idx),
         {:ok, rollback_cmd} <- validate_argv(entry, "rollback_cmd", idx),
         {:ok, auto_merge} <- validate_boolean(entry, "auto_merge", false, idx),
         {:ok, auto_deploy} <- validate_boolean(entry, "auto_deploy", false, idx),
         {:ok, stabilization_window_s} <-
           validate_positive_integer(entry, "stabilization_window_s", 600, idx),
         :ok <- validate_auto_deploy_requirements(auto_deploy, deploy_cmd, rollback_cmd, idx) do
      {:ok,
       %{
         name: name,
         aliases: aliases,
         repo: repo,
         clone_url: clone_url,
         default_branch: branch,
         test_cmd: test_cmd,
         deploy_cmd: deploy_cmd,
         rollback_cmd: rollback_cmd,
         auto_merge: auto_merge,
         auto_deploy: auto_deploy,
         stabilization_window_s: stabilization_window_s
       }}
    end
  end

  defp parse_one_service({_entry, idx}), do: {:error, "service entry ##{idx} is not a TOML table"}

  defp validate_allowed_keys(entry, idx) do
    unknown = Map.keys(entry) |> Enum.reject(&(&1 in @allowed_keys))

    case unknown do
      [] -> :ok
      keys -> {:error, "service entry ##{idx} has unknown keys: #{Enum.join(keys, ", ")}"}
    end
  end

  defp validate_required_string(entry, key, idx) do
    case Map.get(entry, key) do
      value when is_binary(value) and value != "" -> {:ok, value}
      _ -> {:error, "service entry ##{idx} missing required '#{key}'"}
    end
  end

  defp validate_aliases(entry, idx, name) do
    case Map.get(entry, "aliases", []) do
      aliases when is_list(aliases) ->
        cond do
          aliases == [] ->
            {:ok, []}

          not Enum.all?(aliases, &(is_binary(&1) and &1 != "")) ->
            {:error, "service entry ##{idx} aliases must be a non-empty array of strings"}

          length(Enum.uniq(aliases)) != length(aliases) ->
            {:error, "service entry ##{idx} aliases must be unique"}

          name in aliases ->
            {:error, "service entry ##{idx} aliases must not repeat '#{name}'"}

          true ->
            {:ok, aliases}
        end

      _ ->
        {:error, "service entry ##{idx} aliases must be a non-empty array of strings"}
    end
  end

  defp validate_repo(entry, idx) do
    with {:ok, repo} <- validate_required_string(entry, "repo", idx),
         :ok <- Workspace.validate_repo(repo) do
      {:ok, repo}
    else
      {:error, :invalid_repo} ->
        {:error, "service entry ##{idx} has invalid repo '#{Map.get(entry, "repo")}'"}

      {:error, reason} ->
        {:error, reason}
    end
  end

  defp validate_default_branch(entry, idx) do
    with {:ok, branch} <- validate_required_string(entry, "default_branch", idx),
         :ok <- Workspace.validate_branch(branch) do
      {:ok, branch}
    else
      {:error, :invalid_branch} ->
        {:error,
         "service entry ##{idx} has invalid default_branch '#{Map.get(entry, "default_branch")}'"}

      {:error, reason} ->
        {:error, reason}
    end
  end

  defp validate_argv(entry, key, idx, opts \\ []) do
    case Map.get(entry, key) do
      nil ->
        if Keyword.get(opts, :required, false) do
          {:error, "service entry ##{idx} missing required '#{key}'"}
        else
          {:ok, nil}
        end

      value when is_list(value) ->
        if value != [] and Enum.all?(value, &(is_binary(&1) and &1 != "")) do
          {:ok, value}
        else
          {:error, "service entry ##{idx} #{key} must be a non-empty array of strings"}
        end

      _ ->
        {:error, "service entry ##{idx} #{key} must be a non-empty array of strings"}
    end
  end

  defp validate_boolean(entry, key, default, idx) do
    case Map.get(entry, key, default) do
      value when is_boolean(value) -> {:ok, value}
      _ -> {:error, "service entry ##{idx} #{key} must be a boolean"}
    end
  end

  defp validate_positive_integer(entry, key, default, idx) do
    case Map.get(entry, key, default) do
      value when is_integer(value) and value > 0 -> {:ok, value}
      _ -> {:error, "service entry ##{idx} #{key} must be a positive integer"}
    end
  end

  defp validate_auto_deploy_requirements(false, _deploy_cmd, _rollback_cmd, _idx), do: :ok

  defp validate_auto_deploy_requirements(true, nil, _rollback_cmd, idx) do
    {:error, "service entry ##{idx} enables auto_deploy without deploy_cmd"}
  end

  defp validate_auto_deploy_requirements(true, _deploy_cmd, nil, idx) do
    {:error, "service entry ##{idx} enables auto_deploy without rollback_cmd"}
  end

  defp validate_auto_deploy_requirements(true, _deploy_cmd, _rollback_cmd, _idx), do: :ok

  defp service_matches?(service, name) do
    service.name == name or name in service.aliases
  end
end
