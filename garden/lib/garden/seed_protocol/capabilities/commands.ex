defmodule Garden.SeedProtocol.Capabilities.Commands do
  @moduledoc """
  Command execution lifecycle and control messages.
  """

  use Garden.SeedProtocol.Capability

  @impl true
  def name, do: :commands

  @impl true
  def inbound_messages do
    [
          # Seed accepted the command request.
          "command.accepted",
          # Command process started.
          "command.started",
          # Stdout output chunk from running command.
          "command.stdout",
          # Stderr output chunk from running command.
          "command.stderr",
          # Seed confirms stdin bytes were accepted.
          "command.stdin.accepted",
          # Command exited normally with exit code.
          "command.exit",
          # Command failed during start or execution.
          "command.failed",
          # Command finished due to cancellation.
          "command.cancelled",
          # Command was force-killed.
          "command.killed"
        ]
  end

  @impl true
  def outbound_messages do
    [
          # Garden requests command start.
          "command.start",
          # Garden sends stdin data to the command.
          "command.stdin",
          # Garden requests graceful cancellation.
          "command.cancel",
          # Garden requests immediate termination.
          "command.kill"
        ]
  end
  @impl true
  def payload_schemas do
    %{
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
      "command.killed" => %{fields: [command_id: :string, signal: :string, completed_at: :string], required: [:command_id, :completed_at]}
    }
  end
end
