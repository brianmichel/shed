defmodule Garden.SeedProtocol.Capabilities.Files do
  @moduledoc """
  File operation request and response messages.
  """

  use Garden.SeedProtocol.Capability

  @impl true
  def name, do: :files

  @impl true
  def inbound_messages do
    [
          # Generic successful file operation response.
          "file.result",
          # Chunked response payload for large file data.
          "file.chunk",
          # File operation error response.
          "file.error"
        ]
  end

  @impl true
  def outbound_messages do
    [
          # Request to read file content.
          "file.read",
          # Request to write file content.
          "file.write",
          # Request to apply an edit or patch operation.
          "file.edit",
          # Request file metadata or stat information.
          "file.stat",
          # Request content or path search.
          "file.search",
          # Request directory listing.
          "file.list",
          # Request file or directory deletion.
          "file.delete",
          # Request directory creation.
          "file.mkdir"
        ]
  end
  @impl true
  def payload_schemas do
    %{}
  end
end
