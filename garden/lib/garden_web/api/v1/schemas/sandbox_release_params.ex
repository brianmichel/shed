defmodule GardenWeb.Api.V1.Schemas.SandboxReleaseParams do
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  embedded_schema do
    field :reason, :string, default: "unspecified"
  end

  def changeset(schema, params) do
    cast(schema, params, [:reason])
  end

  def to_attrs(params) do
    %{"reason" => params.reason || "unspecified"}
  end
end
