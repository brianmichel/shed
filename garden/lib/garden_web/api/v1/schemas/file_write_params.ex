defmodule GardenWeb.Api.V1.Schemas.FileWriteParams do
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  embedded_schema do
    field :path, :string
    field :content, :string
  end

  def changeset(schema, params) do
    schema
    |> cast(params, [:path, :content])
    |> validate_required([:path, :content])
    |> validate_length(:path, min: 1)
  end

  def to_attrs(params), do: %{"path" => params.path, "content" => params.content}
end
