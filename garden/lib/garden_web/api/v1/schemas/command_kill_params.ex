defmodule GardenWeb.Api.V1.Schemas.CommandKillParams do
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  embedded_schema do
  end

  def changeset(schema, params) do
    cast(schema, params, [])
  end

  def to_attrs(_params), do: %{}
end
