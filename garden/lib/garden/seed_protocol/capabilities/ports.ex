defmodule Garden.SeedProtocol.Capabilities.Ports do
  @moduledoc """
  Service and port exposure control messages.
  """

  use Garden.SeedProtocol.Capability

  @impl true
  def name, do: :ports

  @impl true
  def inbound_messages do
    [
          # Confirms a port was opened or exposed.
          "port.opened",
          # Confirms a port was closed.
          "port.closed",
          # Reports current status of an exposed port.
          "port.status"
        ]
  end

  @impl true
  def outbound_messages do
    [
          # Request to expose or open a sandbox port.
          "port.open",
          # Request to close a previously exposed port.
          "port.close",
          # Request metadata or status for exposed ports.
          "port.describe"
        ]
  end
  @impl true
  def payload_schemas do
    %{}
  end
end
