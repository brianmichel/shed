defmodule Garden.Persistence.SandboxRecord do
  use Ecto.Schema
  import Ecto.Changeset

  schema "sandboxes" do
    field :sandbox_id, :string
    field :state, :string
    field :environment, :string
    field :template, :string
    field :metadata, :map
    field :capabilities, :map
    field :lease, :map
    field :snapshot, :map
    timestamps(type: :utc_datetime)
  end

  def changeset(schema, attrs), do: cast(schema, attrs, [:sandbox_id, :state, :environment, :template, :metadata, :capabilities, :lease, :snapshot])
  def conflict_target(_), do: [:sandbox_id]
end
