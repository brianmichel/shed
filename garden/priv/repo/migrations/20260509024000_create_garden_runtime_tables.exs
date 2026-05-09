defmodule Garden.Repo.Migrations.CreateGardenRuntimeTables do
  use Ecto.Migration

  def change do
    create table(:sandboxes) do
      add :sandbox_id, :string, null: false
      add :state, :string
      add :environment, :string
      add :template, :string
      add :metadata, :map
      add :capabilities, :map
      add :lease, :map
      add :snapshot, :map
      timestamps(type: :utc_datetime)
    end

    create unique_index(:sandboxes, [:sandbox_id])

    create table(:commands) do
      add :command_id, :string, null: false
      add :sandbox_id, :string, null: false
      add :state, :string
      add :command, :text
      add :cwd, :string
      add :env, :map
      add :stdin, :boolean
      add :timeout_ms, :integer
      add :metadata, :map
      add :pid, :integer
      add :exit_code, :integer
      add :signal, :string
      add :started_at, :utc_datetime
      add :completed_at, :utc_datetime
      add :snapshot, :map
      timestamps(type: :utc_datetime)
    end

    create unique_index(:commands, [:command_id])
    create index(:commands, [:sandbox_id])

    create table(:sandbox_events) do
      add :event_id, :string, null: false
      add :sandbox_id, :string, null: false
      add :seq, :integer, null: false
      add :type, :string, null: false
      add :timestamp, :string
      add :data, :map
      timestamps(type: :utc_datetime)
    end

    create unique_index(:sandbox_events, [:event_id])
    create index(:sandbox_events, [:sandbox_id, :seq])

    create table(:command_events) do
      add :event_id, :string, null: false
      add :sandbox_id, :string, null: false
      add :command_id, :string, null: false
      add :seq, :integer, null: false
      add :type, :string, null: false
      add :timestamp, :string
      add :data, :map
      timestamps(type: :utc_datetime)
    end

    create unique_index(:command_events, [:event_id])
    create index(:command_events, [:sandbox_id, :command_id, :seq])

    create table(:seed_sessions) do
      add :session_id, :string, null: false
      add :session_key, :string, null: false
      add :sandbox_id, :string, null: false
      add :status, :string
      add :last_seed_seq_seen, :integer, default: 0
      add :last_garden_seq_sent, :integer, default: 0
      add :last_garden_seq_acked, :integer, default: 0
      add :messages, {:array, :map}, default: []
      add :snapshot, :map
      timestamps(type: :utc_datetime)
    end

    create unique_index(:seed_sessions, [:session_id])
    create unique_index(:seed_sessions, [:session_key])
    create index(:seed_sessions, [:sandbox_id])
  end
end
