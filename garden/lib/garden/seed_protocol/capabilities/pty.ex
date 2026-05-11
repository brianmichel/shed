defmodule Garden.SeedProtocol.Capabilities.Pty do
  @moduledoc """
  PTY lifecycle and terminal I/O messages.
  """

  @behaviour Garden.SeedProtocol.Capability

  @impl true
  def name, do: :pty

  @impl true
  def message_types do
    [
      # Request creation of a PTY session.
      "pty.create",
      # Sends input bytes to an active PTY.
      "pty.input",
      # Requests terminal resize for an active PTY.
      "pty.resize",
      # Requests closure of an active PTY session.
      "pty.close",
      # Confirms PTY session creation.
      "pty.created",
      # Streams PTY output chunk.
      "pty.output",
      # Signals PTY process/session exit.
      "pty.exit",
      # PTY-specific error response.
      "pty.error"
    ]
  end

  @impl true
  def payload_schemas, do: %{}

end
