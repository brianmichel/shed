defmodule Garden.SeedProtocol.Message do
  @moduledoc """
  Protocol envelope validation for Garden ↔ Seed WebSocket messages.
  """

  use Ecto.Schema
  import Ecto.Changeset

  alias Garden.SeedProtocol.Payloads

  @types ~w(
    ack
    error
    seed.hello
    garden.hello
    seed.register
    garden.registered
    seed.resume
    garden.resume
    seed.capabilities
    seed.status
    seed.heartbeat
    garden.heartbeat_ack
    seed.metrics
    seed.warning
    seed.activity
    garden.lease_extended
    garden.lease_warning
    garden.lease_expiring
    command.start
    command.accepted
    command.started
    command.stdout
    command.stderr
    command.stdin
    command.stdin.accepted
    command.cancel
    command.kill
    command.exit
    command.failed
    command.cancelled
    command.killed
    file.read
    file.write
    file.edit
    file.stat
    file.search
    file.list
    file.delete
    file.mkdir
    file.result
    file.chunk
    file.error
    pty.create
    pty.input
    pty.resize
    pty.close
    pty.created
    pty.output
    pty.exit
    pty.error
    port.open
    port.close
    port.describe
    port.opened
    port.closed
    port.status
    artifact.upload
    artifact.download
    snapshot.create
    artifact.ready
    artifact.failed
    snapshot.created
    garden.drain
    seed.draining
    garden.shutdown
    seed.goodbye
  )

  @primary_key false
  embedded_schema do
    field :version, :string, default: "1"
    field :type, :string
    field :message_id, :string
    field :ack_id, :string
    field :request_id, :string
    field :session_id, :string
    field :sandbox_id, :string
    field :seq, :integer
    field :timestamp, :string
    field :expects_ack, :boolean, default: true
    field :reply_to, :string
    field :payload, :map, default: %{}
  end

  def changeset(schema, params) do
    schema
    |> cast(params, [:version, :type, :message_id, :ack_id, :request_id, :session_id, :sandbox_id, :seq, :timestamp, :expects_ack, :reply_to, :payload])
    |> validate_required([:version, :type, :message_id, :request_id, :session_id, :sandbox_id, :seq, :timestamp, :payload])
    |> validate_inclusion(:version, ["1"])
    |> validate_inclusion(:type, @types)
    |> validate_number(:seq, greater_than: 0)
    |> validate_payload()
  end

  def validate(params) do
    changeset = changeset(struct(__MODULE__), params)

    if changeset.valid? do
      {:ok, apply_changes(changeset)}
    else
      {:error, changeset}
    end
  end

  def types, do: @types

  def to_map(message) do
    %{
      version: message.version,
      type: message.type,
      message_id: message.message_id,
      ack_id: message.ack_id,
      request_id: message.request_id,
      session_id: message.session_id,
      sandbox_id: message.sandbox_id,
      seq: message.seq,
      timestamp: message.timestamp,
      expects_ack: message.expects_ack,
      reply_to: message.reply_to,
      payload: message.payload || %{}
    }
    |> Enum.reject(fn {_k, v} -> is_nil(v) end)
    |> Map.new(fn {k, v} -> {to_string(k), stringify(v)} end)
  end

  defp stringify(map) when is_map(map), do: Map.new(map, fn {k, v} -> {to_string(k), stringify(v)} end)
  defp stringify(list) when is_list(list), do: Enum.map(list, &stringify/1)
  defp stringify(value), do: value

  defp validate_payload(changeset) do
    type = get_field(changeset, :type)
    payload = get_field(changeset, :payload) || %{}

    case Payloads.validate_payload(type, payload) do
      {:ok, validated_payload} ->
        put_change(changeset, :payload, validated_payload)

      {:error, payload_changeset} ->
        add_error(changeset, :payload, "is invalid", validation: payload_errors(payload_changeset))
    end
  end

  defp payload_errors(changeset) do
    traverse_errors(changeset, fn {message, opts} ->
      Regex.replace(~r/%{(\w+)}/, message, fn _, key ->
        opts |> Keyword.get(String.to_existing_atom(key), key) |> to_string()
      end)
    end)
  end
end
