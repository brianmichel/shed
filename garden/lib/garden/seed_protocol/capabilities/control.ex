defmodule Garden.SeedProtocol.Capabilities.Control do
  @moduledoc """
  Session drain and shutdown control messages.
  """

  use Garden.SeedProtocol.Capability

  @impl true
  def name, do: :control

  # Seed -> Garden messages.
  def inbound_messages do
    [
      # Seed acknowledgement that it is draining.
      "seed.draining",
      # Seed final message before disconnect/exit.
      "seed.goodbye"
    ]
  end

  # Garden -> Seed messages.
  def outbound_messages do
    [
      # Instructs Seed to stop accepting new work and drain.
      "garden.drain",
      # Instructs Seed to shut down.
      "garden.shutdown"
    ]
  end

  @impl true
  def payload_schemas do
    %{
      "garden.drain" => %{fields: [reason: :string, deadline: :string], required: [:reason]},
      "seed.draining" => %{fields: [active_commands: :integer, estimated_completion_ms: :integer], required: [:active_commands]},
      "garden.shutdown" => %{fields: [reason: :string, deadline: :string], required: [:reason]},
      "seed.goodbye" => %{fields: [reason: :string, final_command_count: :integer, sent_at: :string], required: [:reason, :sent_at]}
    }
  end

end
