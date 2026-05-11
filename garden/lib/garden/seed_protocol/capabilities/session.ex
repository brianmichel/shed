defmodule Garden.SeedProtocol.Capabilities.Session do
  @moduledoc """
  Session lifecycle, handshake, liveness, and runtime status messages.
  """

  @behaviour Garden.SeedProtocol.Capability

  @impl true
  def name, do: :session

  @impl true
  def message_types do
    [
      # Seed initiates protocol handshake and identifies runtime.
      "seed.hello",
      # Garden handshake response with session settings.
      "garden.hello",
      # Seed registers runtime metadata for the session.
      "seed.register",
      # Garden confirms registration/session binding.
      "garden.registered",
      # Seed requests resume after reconnect.
      "seed.resume",
      # Garden responds with resume outcome/replay hints.
      "garden.resume",
      # Seed advertises feature support/capabilities.
      "seed.capabilities",
      # Seed publishes runtime status snapshot.
      "seed.status",
      # Seed heartbeat for liveness + sequence watermarks.
      "seed.heartbeat",
      # Garden acknowledges heartbeat and returns lease context.
      "garden.heartbeat_ack",
      # Seed publishes resource/network metrics sample.
      "seed.metrics",
      # Seed emits a non-fatal warning.
      "seed.warning",
      # Seed reports meaningful activity for lease heuristics.
      "seed.activity"
    ]
  end

  @impl true
  def payload_schemas do
    %{
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
      "seed.activity" => %{fields: [kind: :string, command_id: :string, at: :string], required: [:kind, :at], inclusion: %{kind: ["command_output", "stdin", "file_op", "session_active"]}}
    }
  end

end
