defmodule Garden.SeedProtocol.Capability do
  @moduledoc """
  Behaviour for protocol capability modules that own message types and payload schemas.

  Use `Garden.SeedProtocol.Capability` in capability modules and define directional
  message lists with `inbound_messages/0`, `outbound_messages/0`, and optionally
  `bidirectional_messages/0`.
  """

  @callback name() :: atom()
  @callback message_types() :: [String.t()]
  @callback payload_schemas() :: %{optional(String.t()) => map()}
  @callback inbound_messages() :: [String.t()]
  @callback outbound_messages() :: [String.t()]
  @callback bidirectional_messages() :: [String.t()]

  @optional_callbacks inbound_messages: 0, outbound_messages: 0, bidirectional_messages: 0

  defmacro __using__(_opts) do
    quote do
      @behaviour Garden.SeedProtocol.Capability

      @impl true
      def message_types do
        inbound_messages() ++ outbound_messages() ++ bidirectional_messages()
      end

      @impl true
      def inbound_messages, do: []

      @impl true
      def outbound_messages, do: []

      @impl true
      def bidirectional_messages, do: []

      defoverridable message_types: 0, inbound_messages: 0, outbound_messages: 0, bidirectional_messages: 0
    end
  end
end
