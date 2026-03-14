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

  @spec ready?(t()) :: :ok | {:error, [binary()]}
  def ready?(%__MODULE__{body: body}) do
    failures =
      []
      |> check_section(body, "## Product Spec", "missing `## Product Spec` section")
      |> check_section(body, "### Intent Contract", "missing `### Intent Contract` section")
      |> Enum.reverse()

    case failures do
      [] -> :ok
      list -> {:error, list}
    end
  end

  defp check_section(acc, body, heading, msg) do
    if String.contains?(body, heading), do: acc, else: [msg | acc]
  end

  defp label_name(%{"name" => n}), do: n
  defp label_name(n) when is_binary(n), do: n
end
