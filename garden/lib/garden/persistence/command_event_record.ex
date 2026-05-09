defmodule Garden.Persistence.CommandEventRecord do
  use Ecto.Schema
  import Ecto.Changeset

  schema "command_events" do
    field :event_id, :string
    field :sandbox_id, :string
    field :command_id, :string
    field :seq, :integer
    field :type, :string
    field :timestamp, :string
    field :data, :map
    timestamps(type: :utc_datetime)
  end

  def changeset(schema, attrs), do: cast(schema, attrs, [:event_id, :sandbox_id, :command_id, :seq, :type, :timestamp, :data])
end
