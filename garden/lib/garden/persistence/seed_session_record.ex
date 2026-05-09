defmodule Garden.Persistence.SeedSessionRecord do
  use Ecto.Schema
  import Ecto.Changeset

  schema "seed_sessions" do
    field :session_id, :string
    field :session_key, :string
    field :sandbox_id, :string
    field :status, :string
    field :last_seed_seq_seen, :integer
    field :last_garden_seq_sent, :integer
    field :last_garden_seq_acked, :integer
    field :messages, {:array, :map}, default: []
    field :snapshot, :map
    timestamps(type: :utc_datetime)
  end

  def changeset(schema, attrs), do: cast(schema, attrs, [:session_id, :session_key, :sandbox_id, :status, :last_seed_seq_seen, :last_garden_seq_sent, :last_garden_seq_acked, :messages, :snapshot])
  def conflict_target(_), do: [:session_id]
end
