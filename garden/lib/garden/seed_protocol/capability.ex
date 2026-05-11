defmodule Garden.SeedProtocol.Capability do
  @moduledoc """
  Behaviour for protocol capability modules that own message types and payload schemas.
  """

  @callback name() :: atom()
  @callback message_types() :: [String.t()]
  @callback payload_schemas() :: %{optional(String.t()) => map()}
end
