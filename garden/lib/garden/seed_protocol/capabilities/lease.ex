defmodule Garden.SeedProtocol.Capabilities.Lease do
  @moduledoc """
  Lease and expiration signaling between Garden and Seed.
  """

  @behaviour Garden.SeedProtocol.Capability

  @impl true
  def name, do: :lease

  @impl true
  def message_types do
    [
      # Notifies Seed that the sandbox lease was extended.
      "garden.lease_extended",
      # Warns Seed that lease expiry is approaching.
      "garden.lease_warning",
      # Signals imminent lease expiry and expected action.
      "garden.lease_expiring"
    ]
  end

  @impl true
  def payload_schemas do
    %{
      "garden.lease_extended" => %{fields: [lease_expires_at: :string, reason: :string], required: [:lease_expires_at, :reason]},
      "garden.lease_warning" => %{fields: [lease_expires_at: :string, remaining_ms: :integer], required: [:lease_expires_at, :remaining_ms]},
      "garden.lease_expiring" => %{fields: [lease_expires_at: :string, remaining_ms: :integer, action: :string], required: [:lease_expires_at, :remaining_ms, :action]}
    }
  end

end
