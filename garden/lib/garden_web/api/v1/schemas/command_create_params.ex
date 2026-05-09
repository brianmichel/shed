defmodule GardenWeb.Api.V1.Schemas.CommandCreateParams do
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  embedded_schema do
    field :command, :string
    field :cwd, :string, default: "/workspace"
    field :env, :map, default: %{}
    field :stdin, :boolean, default: false
    field :timeout_ms, :integer, default: 60_000
    field :metadata, :map, default: %{}
  end

  def changeset(schema, params) do
    schema
    |> cast(normalize(params), [:command, :cwd, :env, :stdin, :timeout_ms, :metadata])
    |> validate_required([:command, :cwd, :timeout_ms])
    |> validate_length(:command, min: 1)
    |> validate_number(:timeout_ms, greater_than: 0)
  end

  def to_attrs(params) do
    %{
      "command" => params.command,
      "cwd" => params.cwd,
      "env" => params.env || %{},
      "stdin" => params.stdin,
      "timeout_ms" => params.timeout_ms,
      "metadata" => params.metadata || %{}
    }
  end

  defp normalize(params) do
    params
    |> Map.put_new("cwd", "/workspace")
    |> Map.put_new("env", %{})
    |> Map.put_new("stdin", false)
    |> Map.put_new("timeout_ms", 60_000)
    |> Map.put_new("metadata", %{})
  end
end
