defmodule Garden.SeedProtocol.Capabilities.Control do
  @moduledoc """
  Session drain and shutdown control messages.
  """

  @behaviour Garden.SeedProtocol.Capability

  @impl true
  def name, do: :control

  @impl true
  def message_types do
    [
      # Instructs Seed to stop accepting new work and drain.
      "garden.drain",
      # Seed acknowledgement that it is draining.
      "seed.draining",
      # Instructs Seed to shut down.
      "garden.shutdown",
      # Seed final message before disconnect/exit.
      "seed.goodbye"
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
