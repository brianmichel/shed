defmodule Garden.SeedProtocol.Capabilities.Artifacts do
  @moduledoc """
  Artifact transfer and snapshot lifecycle messages.
  """

  use Garden.SeedProtocol.Capability

  @impl true
  def name, do: :artifacts

  @impl true
  def inbound_messages do
    [
          # Signals artifact is ready.
          "artifact.ready",
          # Signals artifact processing or transfer failure.
          "artifact.failed",
          # Signals snapshot creation succeeded.
          "snapshot.created"
        ]
  end

  @impl true
  def outbound_messages do
    [
          # Request artifact upload flow.
          "artifact.upload",
          # Request artifact download flow.
          "artifact.download",
          # Request snapshot creation.
          "snapshot.create"
        ]
  end
  @impl true
  def payload_schemas do
    %{}
  end
end
