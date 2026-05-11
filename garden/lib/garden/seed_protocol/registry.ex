defmodule Garden.SeedProtocol.Registry do
  @moduledoc """
  Central registry for protocol capability modules.

  This file is generated from `spec/protocol/messages.json`.
  """

  alias Garden.SeedProtocol.Capabilities

  @capabilities [
    Capabilities.Core,
    Capabilities.Session,
    Capabilities.Lease,
    Capabilities.Commands,
    Capabilities.Files,
    Capabilities.Pty,
    Capabilities.Ports,
    Capabilities.Artifacts,
    Capabilities.Control
  ]

  def capabilities, do: @capabilities

  def all_types do
    @capabilities
    |> Enum.flat_map(& &1.message_types())
    |> Enum.uniq()
  end

  def schema_for(type) do
    Enum.find_value(@capabilities, fn mod ->
      Map.get(mod.payload_schemas(), type)
    end)
  end

  def capability_for(type) do
    Enum.find(@capabilities, fn mod -> type in mod.message_types() end)
  end
end
