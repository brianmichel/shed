defmodule Garden.SeedProtocol.Payloads do
  @moduledoc false

  import Ecto.Changeset

  @schemas %{
    "ack" => %{fields: [status: :string], required: [:status], inclusion: %{status: ["accepted", "rejected", "duplicate", "unsupported"]}},
    "error" => %{fields: [code: :string, message: :string, retryable: :boolean, details: :map, failed_message_id: :string], required: [:code, :message, :retryable]},
    "seed.hello" => %{fields: [seed_version: :string, protocol_version: :string, platform: :string, arch: :string, hostname: :string, session_key: :string], required: [:seed_version, :protocol_version, :platform, :arch, :hostname, :session_key]},
    "garden.hello" => %{fields: [protocol_version: :string, session_id: :string, sandbox_id: :string, heartbeat_interval_ms: :integer, ack_timeout_ms: :integer], required: [:protocol_version, :session_id, :sandbox_id, :heartbeat_interval_ms, :ack_timeout_ms]},
    "seed.register" => %{fields: [seed_id: :string, seed_version: :string, platform: :string, arch: :string, hostname: :string, boot_time: :string, workspace_root: :string, process_id: :integer], required: [:seed_id, :seed_version, :platform, :arch, :hostname]},
    "garden.registered" => %{fields: [session_id: :string, sandbox_id: :string, lease_expires_at: :string, max_session_duration_ms: :integer], required: [:session_id, :sandbox_id]},
    "seed.resume" => %{fields: [session_id: :string, sandbox_id: :string, last_garden_seq_seen: :integer, last_seed_seq_sent: :integer, seed_id: :string, connection_generation: :integer], required: [:session_id, :sandbox_id, :last_garden_seq_seen, :last_seed_seq_sent, :seed_id, :connection_generation]},
    "garden.resume" => %{fields: [status: :string, session_id: :string, replay_from_seed_seq: :integer, replay_from_garden_seq: :integer], required: [:status, :session_id], inclusion: %{status: ["ok", "replace_session", "rejected"]}},
    "seed.capabilities" => %{fields: [commands: :boolean, stdin: :boolean, cancel: :boolean, kill: :boolean, files: :map, pty: :boolean, metrics: :boolean, ports: :boolean, snapshots: :boolean], required: [:commands, :stdin, :cancel, :kill, :pty, :metrics, :ports, :snapshots]},
    "seed.status" => %{fields: [state: :string, hostname: :string, platform: :string, arch: :string, seed_version: :string, uptime_ms: :integer, workspace_root: :string, active_commands: :integer, network: :map], required: [:state, :seed_version], inclusion: %{state: ["booting", "ready", "degraded", "draining"]}},
    "seed.heartbeat" => %{fields: [uptime_ms: :integer, active_commands: :integer, last_garden_seq_seen: :integer, last_seed_seq_sent: :integer, load_avg: :float, memory_used_bytes: :integer, connection_generation: :integer], required: [:uptime_ms, :active_commands, :last_garden_seq_seen, :last_seed_seq_sent, :connection_generation]},
    "garden.heartbeat_ack" => %{fields: [lease_expires_at: :string, server_time: :string, status: :string], required: [:server_time, :status], inclusion: %{status: ["ok", "draining", "reauth_required"]}},
    "seed.metrics" => %{fields: [cpu_percent: :float, load_avg_1m: :float, memory_total_bytes: :integer, memory_used_bytes: :integer, disk_total_bytes: :integer, disk_used_bytes: :integer, fs_available_bytes: :integer, rx_bytes: :integer, tx_bytes: :integer, sampled_at: :string], required: [:sampled_at]},
    "seed.warning" => %{fields: [code: :string, message: :string, details: :map], required: [:code, :message]},
    "seed.activity" => %{fields: [kind: :string, command_id: :string, at: :string], required: [:kind, :at], inclusion: %{kind: ["command_output", "stdin", "file_op", "session_active"]}},
    "garden.lease_extended" => %{fields: [lease_expires_at: :string, reason: :string], required: [:lease_expires_at, :reason]},
    "garden.lease_warning" => %{fields: [lease_expires_at: :string, remaining_ms: :integer], required: [:lease_expires_at, :remaining_ms]},
    "garden.lease_expiring" => %{fields: [lease_expires_at: :string, remaining_ms: :integer, action: :string], required: [:lease_expires_at, :remaining_ms, :action]},
    "command.start" => %{fields: [command_id: :string, command: :string, cwd: :string, env: :map, stdin: :boolean, timeout_ms: :integer, metadata: :map], required: [:command_id, :command]},
    "command.accepted" => %{fields: [command_id: :string, state: :string], required: [:command_id, :state], inclusion: %{state: ["queued", "starting"]}},
    "command.started" => %{fields: [command_id: :string, pid: :integer, started_at: :string], required: [:command_id, :pid, :started_at]},
    "command.stdout" => %{fields: [command_id: :string, chunk: :string, encoding: :string, stream_seq: :integer], required: [:command_id, :chunk, :encoding, :stream_seq]},
    "command.stderr" => %{fields: [command_id: :string, chunk: :string, encoding: :string, stream_seq: :integer], required: [:command_id, :chunk, :encoding, :stream_seq]},
    "command.stdin" => %{fields: [command_id: :string, data: :string, encoding: :string], required: [:command_id, :data, :encoding]},
    "command.stdin.accepted" => %{fields: [command_id: :string, bytes: :integer], required: [:command_id, :bytes]},
    "command.cancel" => %{fields: [command_id: :string, grace_period_ms: :integer, escalation: :string], required: [:command_id, :grace_period_ms, :escalation], inclusion: %{escalation: ["kill"]}},
    "command.kill" => %{fields: [command_id: :string, signal: :string], required: [:command_id]},
    "command.exit" => %{fields: [command_id: :string, exit_code: :integer, completed_at: :string], required: [:command_id, :exit_code, :completed_at]},
    "command.failed" => %{fields: [command_id: :string, exit_code: :integer, message: :string, completed_at: :string], required: [:command_id, :message, :completed_at]},
    "command.cancelled" => %{fields: [command_id: :string, completed_at: :string], required: [:command_id, :completed_at]},
    "command.killed" => %{fields: [command_id: :string, signal: :string, completed_at: :string], required: [:command_id, :completed_at]},
    "garden.drain" => %{fields: [reason: :string, deadline: :string], required: [:reason]},
    "seed.draining" => %{fields: [active_commands: :integer, estimated_completion_ms: :integer], required: [:active_commands]},
    "garden.shutdown" => %{fields: [reason: :string, deadline: :string], required: [:reason]},
    "seed.goodbye" => %{fields: [reason: :string, final_command_count: :integer, sent_at: :string], required: [:reason, :sent_at]}
  }

  def validate_payload(type, params) when is_map(params) do
    case Map.get(@schemas, type) do
      nil -> {:ok, params}
      schema -> validate_schemaless(schema, params)
    end
  end

  def validate_payload(_type, _params) do
    {:error, cast({%{}, %{}}, %{}, []) |> add_error(:data, "must be an object")}
  end

  defp validate_schemaless(schema, params) do
    types = Map.new(schema.fields)
    fields = Keyword.keys(schema.fields)

    changeset =
      {%{}, types}
      |> cast(params, fields)
      |> validate_required(Map.get(schema, :required, []))
      |> apply_inclusions(Map.get(schema, :inclusion, %{}))

    if changeset.valid? do
      {:ok,
       changeset
       |> apply_changes()
       |> Enum.reject(fn {_k, v} -> is_nil(v) end)
       |> Map.new(fn {k, v} -> {to_string(k), v} end)}
    else
      {:error, changeset}
    end
  end

  defp apply_inclusions(changeset, inclusion) do
    Enum.reduce(inclusion, changeset, fn {field, values}, acc ->
      validate_inclusion(acc, field, values)
    end)
  end
end
