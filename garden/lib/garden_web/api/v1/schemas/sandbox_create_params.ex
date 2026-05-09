defmodule GardenWeb.Api.V1.Schemas.SandboxCreateParams do
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  embedded_schema do
    field :environment, :string, default: "linux"
    field :template, :string, default: "default"
    field :ttl_ms, :integer, default: 30 * 60 * 1000
    field :metadata, :map, default: %{}
  end

  def changeset(schema, params) do
    schema
    |> cast(normalize(params), [:environment, :template, :ttl_ms, :metadata])
    |> validate_required([:environment, :template, :ttl_ms])
    |> validate_inclusion(:environment, ["linux", "macos"])
    |> validate_number(:ttl_ms, greater_than: 0)
  end

  def to_attrs(params) do
    %{
      "environment" => params.environment,
      "template" => params.template,
      "metadata" => params.metadata || %{},
      "lease" => %{"ttl_ms" => params.ttl_ms}
    }
  end

  defp normalize(params) do
    params
    |> Map.put_new("environment", "linux")
    |> Map.put_new("template", "default")
    |> Map.put_new("metadata", %{})
    |> Map.put_new("ttl_ms", get_in(params, ["lease", "ttl_ms"]) || 30 * 60 * 1000)
  end
end
