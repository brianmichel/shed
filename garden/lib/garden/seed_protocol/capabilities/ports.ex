defmodule Garden.SeedProtocol.Capabilities.Ports do
  @moduledoc """
  Service/port exposure control messages.
  """

  @behaviour Garden.SeedProtocol.Capability

  @impl true
  def name, do: :ports

  @impl true
  def message_types do
    [
      # Request to expose/open a sandbox port.
      "port.open",
      # Request to close a previously exposed port.
      "port.close",
      # Request metadata/status for exposed ports.
      "port.describe",
      # Confirms a port was opened/exposed.
      "port.opened",
      # Confirms a port was closed.
      "port.closed",
      # Reports current status of an exposed port.
      "port.status"
    ]
  end

  @impl true
  def payload_schemas, do: %{}

end
