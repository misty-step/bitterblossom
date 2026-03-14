defmodule Conductor.Issue do
  @moduledoc "GitHub issue with readiness checks."

  @type t :: %__MODULE__{
          number: pos_integer(),
          title: binary(),
          body: binary(),
          url: binary(),
          labels: [binary()]
        }

  @enforce_keys [:number, :title, :body, :url]
  defstruct [:number, :title, :body, :url, labels: []]

  @spec from_github(map()) :: t()
  def from_github(%{"number" => n, "title" => t} = data) do
    %__MODULE__{
      number: n,
      title: t,
      body: data["body"] || "",
      url: data["url"] || "https://github.com/unknown/issues/#{n}",
      labels: Enum.map(data["labels"] || [], &label_name/1)
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

  defp has?(body, heading), do: String.contains?(body, heading)

  defp check_missing(acc, body, headings, msg) do
    if Enum.any?(headings, &has?(body, &1)), do: acc, else: [msg | acc]
  end

  defp label_name(%{"name" => n}), do: n
  defp label_name(n) when is_binary(n), do: n
end
