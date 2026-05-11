defmodule Garden.SeedProtocol.Capabilities.Pty do
  @moduledoc """
  PTY lifecycle and terminal IO messages.
  """

  use Garden.SeedProtocol.Capability

  @impl true
  def name, do: :pty

  @impl true
  def inbound_messages do
    [
          # Confirms PTY session creation.
          "pty.created",
          # Streams PTY output chunk.
          "pty.output",
          # Signals PTY process or session exit.
          "pty.exit",
          # PTY-specific error response.
          "pty.error"
        ]
  end

  @impl true
  def outbound_messages do
    [
          # Request creation of a PTY session.
          "pty.create",
          # Send input bytes to an active PTY.
          "pty.input",
          # Request terminal resize for an active PTY.
          "pty.resize",
          # Request closure of an active PTY session.
          "pty.close"
        ]
  end
  @impl true
  def payload_schemas do
    %{}
  end
end
