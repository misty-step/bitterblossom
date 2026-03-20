defmodule Conductor.Issue do
  @moduledoc "GitHub issue with readiness checks."

  @type t :: %__MODULE__{
          number: pos_integer(),
          title: binary(),
          body: binary(),
          url: binary(),
          labels: [binary()],
          assignees: [binary()],
          assignee_metadata: [map()]
        }

  @enforce_keys [:number, :title, :body, :url]
  defstruct [:number, :title, :body, :url, labels: [], assignees: [], assignee_metadata: []]

  @priority_order %{p0: 0, p1: 1, p2: 2, p3: 3, unlabeled: 4}

  @spec from_github(map()) :: t()
  def from_github(%{"number" => n, "title" => t} = data) do
    assignee_metadata =
      data["assignees"]
      |> List.wrap()
      |> Enum.map(&normalize_assignee/1)
      |> Enum.reject(&(&1.name == ""))

    %__MODULE__{
      number: n,
      title: t,
      body: data["body"] || "",
      url: data["url"] || "https://github.com/unknown/issues/#{n}",
      labels: Enum.map(data["labels"] || [], &label_name/1),
      assignees: Enum.map(assignee_metadata, & &1.name),
      assignee_metadata: assignee_metadata
    }
  end

  @doc """
  Check issue readiness for conductor pickup.

  Accepts two formats:
  - Groom format: `## Problem` + `## Acceptance Criteria` (org standard)
  - Conductor format: `## Product Spec` + `### Intent Contract` (legacy)

  An issue is ready if it has EITHER format — problem + criteria is sufficient.
  """
  @spec ready?(t()) :: :ok | {:error, [binary()]}
  def ready?(%__MODULE__{body: body}) do
    cond do
      # Groom format (org standard): Problem + Acceptance Criteria
      has?(body, "## Problem") and has?(body, "## Acceptance Criteria") ->
        :ok

      # Conductor format (legacy): Product Spec + Intent Contract
      has?(body, "## Product Spec") and has?(body, "### Intent Contract") ->
        :ok

      true ->
        failures =
          []
          |> check_missing(
            body,
            ["## Problem", "## Product Spec"],
            "missing `## Problem` or `## Product Spec` section"
          )
          |> check_missing(
            body,
            ["## Acceptance Criteria", "### Intent Contract"],
            "missing `## Acceptance Criteria` or `### Intent Contract` section"
          )
          |> Enum.reverse()

        {:error, failures}
    end
  end

  @doc "Stable revision identifier for issue body change detection."
  @spec revision_id(t()) :: binary()
  def revision_id(%__MODULE__{body: body}) do
    :crypto.hash(:sha256, body || "")
  end

  @doc "Normalized issue priority from labels."
  @spec priority(t()) :: :p0 | :p1 | :p2 | :p3 | :unlabeled
  def priority(%__MODULE__{labels: labels}) do
    labels
    |> Enum.map(&priority_label/1)
    |> Enum.reject(&is_nil/1)
    |> Enum.min_by(&Map.fetch!(@priority_order, &1), fn -> :unlabeled end)
  end

  @doc "Sort key for dispatch selection: priority first, then issue number."
  @spec selection_sort_key(t()) :: {non_neg_integer(), pos_integer()}
  def selection_sort_key(%__MODULE__{} = issue) do
    {Map.fetch!(@priority_order, priority(issue)), issue.number}
  end

  @doc "True when the issue currently has any assignee."
  @spec assigned?(t()) :: boolean()
  def assigned?(%__MODULE__{assignees: assignees}), do: assignees != []

  @doc """
  Returns only human assignee names.

  Falls back to the raw `assignees` list when metadata is unavailable, which
  preserves direct struct construction in tests and legacy callers.
  """
  @spec human_assignees(t()) :: [binary()]
  def human_assignees(%__MODULE__{assignee_metadata: []} = issue), do: issue.assignees

  def human_assignees(%__MODULE__{assignee_metadata: assignee_metadata}) do
    assignee_metadata
    |> Enum.filter(&human_assignee?/1)
    |> Enum.map(& &1.name)
  end

  @doc "True when any human assignee is present on the issue."
  @spec human_assigned?(t()) :: boolean()
  def human_assigned?(%__MODULE__{} = issue), do: human_assignees(issue) != []

  defp has?(body, heading), do: String.contains?(body, heading)

  defp check_missing(acc, body, headings, msg) do
    if Enum.any?(headings, &has?(body, &1)), do: acc, else: [msg | acc]
  end

  defp label_name(%{"name" => n}), do: n
  defp label_name(n) when is_binary(n), do: n

  defp priority_label(label) when is_binary(label) do
    case String.downcase(label) do
      "p0" -> :p0
      "p1" -> :p1
      "p2" -> :p2
      "p3" -> :p3
      _ -> nil
    end
  end

  defp normalize_assignee(%{"login" => login} = assignee) when is_binary(login) do
    %{
      name: login,
      kind: assignee_kind(assignee, login)
    }
  end

  defp normalize_assignee(%{"name" => name} = assignee) when is_binary(name) do
    %{
      name: name,
      kind: assignee_kind(assignee, name)
    }
  end

  defp normalize_assignee(login) when is_binary(login) do
    %{
      name: login,
      kind: assignee_kind(%{}, login)
    }
  end

  defp normalize_assignee(_), do: %{name: "", kind: :unknown}

  defp assignee_kind(%{"type" => type}, _name) when is_binary(type) do
    case String.downcase(type) do
      "bot" -> :bot
      "app" -> :app
      "user" -> :human
      _ -> :unknown
    end
  end

  defp assignee_kind(_assignee, name) when is_binary(name) do
    if String.ends_with?(String.downcase(name), "[bot]"), do: :bot, else: :human
  end

  defp human_assignee?(%{name: "", kind: _kind}), do: false
  defp human_assignee?(%{kind: :human}), do: true
  defp human_assignee?(_assignee), do: false
end
