defmodule Garden.SeedProtocol.Capabilities.Artifacts do
  @moduledoc """
  Artifact transfer and snapshot lifecycle messages.
  """

  @behaviour Garden.SeedProtocol.Capability

  @impl true
  def name, do: :artifacts

  @impl true
  def message_types do
    [
      # Request artifact upload flow.
      "artifact.upload",
      # Request artifact download flow.
      "artifact.download",
      # Request snapshot creation.
      "snapshot.create",
      # Signals artifact is ready.
      "artifact.ready",
      # Signals artifact processing/transfer failure.
      "artifact.failed",
      # Signals snapshot creation succeeded.
      "snapshot.created"
    ]
  end

  @impl true
  def payload_schemas, do: %{}

end
