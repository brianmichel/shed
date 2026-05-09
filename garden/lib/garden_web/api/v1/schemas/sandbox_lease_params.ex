defmodule GardenWeb.Api.V1.Schemas.SandboxLeaseParams do
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  embedded_schema do
    field :ttl_ms, :integer
    field :reason, :string, default: "unspecified"
  end

  def changeset(schema, params) do
    schema
    |> cast(params, [:ttl_ms, :reason])
    |> validate_required([:ttl_ms])
    |> validate_number(:ttl_ms, greater_than: 0)
  end

  def to_attrs(params) do
    %{"ttl_ms" => params.ttl_ms, "reason" => params.reason || "unspecified"}
  end
end
