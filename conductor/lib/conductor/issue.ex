defmodule Conductor.Issue do
  @moduledoc "GitHub issue with readiness checks."

  @type t :: %__MODULE__{
          number: pos_integer(),
          title: binary(),
          body: binary(),
          url: binary(),
          labels: [binary()],
          assignees: [binary()]
        }

  @enforce_keys [:number, :title, :body, :url]
  defstruct [:number, :title, :body, :url, labels: [], assignees: []]

  @priority_order %{p0: 0, p1: 1, p2: 2, p3: 3, unlabeled: 4}

  @spec from_github(map()) :: t()
  def from_github(%{"number" => n, "title" => t} = data) do
    %__MODULE__{
      number: n,
      title: t,
      body: data["body"] || "",
      url: data["url"] || "https://github.com/unknown/issues/#{n}",
      labels: Enum.map(data["labels"] || [], &label_name/1),
      assignees:
        data["assignees"]
        |> List.wrap()
        |> Enum.map(&assignee_login/1)
        |> Enum.reject(&(&1 == ""))
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

  defp assignee_login(%{"login" => login}) when is_binary(login), do: login
  defp assignee_login(%{"name" => name}) when is_binary(name), do: name
  defp assignee_login(login) when is_binary(login), do: login
  defp assignee_login(_), do: ""
end
