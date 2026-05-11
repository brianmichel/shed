defmodule Garden.SeedProtocol.Router do
  @moduledoc """
  Centralized lookup for protocol message ownership.
  """

  alias Garden.SeedProtocol.Registry

  def capability_name(type) do
    case Registry.capability_for(type) do
      nil -> nil
      mod -> mod.name()
    end
  end
end
