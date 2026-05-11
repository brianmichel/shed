defmodule Garden.SeedProtocol.Capabilities.Core do
  @moduledoc """
  Core protocol envelopes for acknowledgement and structured errors.
  """

  use Garden.SeedProtocol.Capability

  @impl true
  def name, do: :core

  # Messages that can flow in both directions.
  def bidirectional_messages do
    [
      # Acknowledges receipt/acceptance of a prior message (via ack_id).
      "ack",
      # Structured protocol-level failure envelope.
      "error"
    ]
  end

  @impl true
  def payload_schemas do
    %{
      "ack" => %{fields: [status: :string], required: [:status], inclusion: %{status: ["accepted", "rejected", "duplicate", "unsupported"]}},
      "error" => %{fields: [code: :string, message: :string, retryable: :boolean, details: :map, failed_message_id: :string], required: [:code, :message, :retryable]}
    }
  end

end
