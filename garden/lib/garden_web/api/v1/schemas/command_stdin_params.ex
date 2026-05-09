defmodule GardenWeb.Api.V1.Schemas.CommandStdinParams do
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  embedded_schema do
    field :data, :string
    field :encoding, :string, default: "utf-8"
  end

  def changeset(schema, params) do
    schema
    |> cast(normalize(params), [:data, :encoding])
    |> validate_required([:data, :encoding])
    |> validate_inclusion(:encoding, ["utf-8"])
  end

  def to_attrs(params) do
    %{"data" => params.data, "encoding" => params.encoding}
  end

  defp normalize(params), do: Map.put_new(params, "encoding", "utf-8")
end
