defmodule Garden.Persistence.CommandRecord do
  use Ecto.Schema
  import Ecto.Changeset

  schema "commands" do
    field :command_id, :string
    field :sandbox_id, :string
    field :state, :string
    field :command, :string
    field :cwd, :string
    field :env, :map
    field :stdin, :boolean
    field :timeout_ms, :integer
    field :metadata, :map
    field :pid, :integer
    field :exit_code, :integer
    field :signal, :string
    field :started_at, :utc_datetime
    field :completed_at, :utc_datetime
    field :snapshot, :map
    timestamps(type: :utc_datetime)
  end

  def changeset(schema, attrs), do: cast(schema, attrs, [:command_id, :sandbox_id, :state, :command, :cwd, :env, :stdin, :timeout_ms, :metadata, :pid, :exit_code, :signal, :started_at, :completed_at, :snapshot])
  def conflict_target(_), do: [:command_id]
end
